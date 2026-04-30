package platform

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type netbsdCollector struct{}

func (c *netbsdCollector) Collect(ctx context.Context, r Runner) (*Snapshot, error) {
	s := &Snapshot{}

	// 1. SystemInfo
	unameOut, _ := r.Run(ctx, "uname -srm")
	hostOut, _ := r.Run(ctx, "hostname")
	s.SystemInfo = parseSystemInfo(unameOut, hostOut)

	// 2. Uptime — NetBSD kern.boottime uses same { sec = ..., usec = ... } format as FreeBSD
	btOut, err := r.Run(ctx, "sysctl -n kern.boottime")
	if err != nil {
		s.setErr("uptime", err.Error())
	} else {
		u, err := parseSysctlBootTime(btOut)
		if err != nil {
			s.setErr("uptime", err.Error())
		} else {
			s.UptimeSecs = &u
			s.setOK("uptime")
		}
	}

	// 3. Load via vm.loadavg
	laOut, err := r.Run(ctx, "sysctl -n vm.loadavg")
	if err != nil {
		s.setErr("load", err.Error())
	} else {
		la, err := parseSysctlLoadAvg(laOut)
		if err != nil {
			s.setErr("load", err.Error())
		} else {
			s.Load = la
			s.setOK("load")
		}
	}

	// 4. Memory via vm.uvmexp2 page counts (NetBSD uses uvmexp2)
	uvmOut, err := r.Run(ctx, "sysctl -n vm.uvmexp2.npages vm.uvmexp2.free hw.pagesize")
	if err != nil {
		// Fall back to hw.physmem only
		physOut, err2 := r.Run(ctx, "sysctl -n hw.physmem")
		if err2 != nil {
			s.setErr("memory", err.Error())
		} else {
			physMem, err2 := strconv.ParseInt(strings.TrimSpace(physOut), 10, 64)
			if err2 != nil {
				s.setErr("memory", err2.Error())
			} else {
				s.Memory = &MemoryStats{TotalBytes: physMem}
				s.setOK("memory")
			}
		}
	} else {
		mem, err := parseNetBSDSysctlMem(uvmOut)
		if err != nil {
			s.setErr("memory", err.Error())
		} else {
			s.Memory = mem
			s.setOK("memory")
		}
	}

	// 5. Swap via swapctl -s -k (same format as OpenBSD)
	swapOut, err := r.Run(ctx, "swapctl -s -k 2>/dev/null")
	if err != nil {
		s.setUnsupported("swap")
	} else {
		sw, err := parseOpenBSDSwapctl(swapOut)
		if err != nil {
			s.setErr("swap", err.Error())
		} else if sw == nil {
			s.setUnsupported("swap")
		} else {
			s.Swap = sw
			s.setOK("swap")
		}
	}

	// 6. CPU: two kern.cp_time samples 1s apart (same space-separated format as FreeBSD)
	cp1Out, err := r.Run(ctx, "sysctl -n kern.cp_time")
	if err != nil {
		s.setErr("cpu", err.Error())
	} else {
		sample1, err := parseFreeBSDCPTime(cp1Out)
		if err != nil {
			s.setErr("cpu", err.Error())
		} else {
			select {
			case <-ctx.Done():
				s.setErr("cpu", ctx.Err().Error())
			case <-time.After(time.Second):
				cp2Out, err := r.Run(ctx, "sysctl -n kern.cp_time")
				if err != nil {
					s.setErr("cpu", err.Error())
				} else {
					sample2, err := parseFreeBSDCPTime(cp2Out)
					if err != nil {
						s.setErr("cpu", err.Error())
					} else {
						s.CPU = calcFreeBSDCPU(sample1, sample2)
						s.setOK("cpu")
					}
				}
			}
		}
	}

	// 7. Disks via df -kP
	dfOut, err := r.Run(ctx, "df -kP 2>/dev/null")
	if err != nil {
		s.setErr("disks", err.Error())
	} else {
		disks, err := parseDFOutput(dfOut)
		if err != nil {
			s.setErr("disks", err.Error())
		} else {
			s.Disks = disks
			s.setOK("disks")
		}
	}

	// 8. Network — same netstat -ibn format
	netOut, err := r.Run(ctx, "netstat -ibn 2>/dev/null")
	if err != nil {
		s.setErr("network", err.Error())
	} else {
		nets, err := parseDarwinNetstat(netOut)
		if err != nil {
			s.setErr("network", err.Error())
		} else {
			s.Network = nets
			s.setOK("network")
		}
	}

	// 9. Processes
	psOut, err := r.Run(ctx, "ps aux 2>/dev/null | head -11")
	if err != nil {
		s.setErr("processes", err.Error())
	} else {
		procs, err := parsePSAux(psOut)
		if err != nil {
			s.setErr("processes", err.Error())
		} else {
			s.Processes = procs
			s.setOK("processes")
		}
	}

	// 10. Optional modules from standard Unix tools.
	collectStandardUnixMetrics(ctx, r, s)

	return s, nil
}

// parseNetBSDSysctlMem parses 3 lines: npages, free, pagesize from vm.uvmexp2.
func parseNetBSDSysctlMem(output string) (*MemoryStats, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 3 {
		return nil, fmt.Errorf("parseNetBSDSysctlMem: expected 3 lines, got %d", len(lines))
	}
	parseInt := func(s string) (int64, error) {
		return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	}
	npages, err := parseInt(lines[0])
	if err != nil {
		return nil, fmt.Errorf("parseNetBSDSysctlMem: npages: %w", err)
	}
	free, err := parseInt(lines[1])
	if err != nil {
		return nil, fmt.Errorf("parseNetBSDSysctlMem: free: %w", err)
	}
	pageSize, err := parseInt(lines[2])
	if err != nil {
		return nil, fmt.Errorf("parseNetBSDSysctlMem: pagesize: %w", err)
	}

	total := npages * pageSize
	freeBytes := free * pageSize
	used := (npages - free) * pageSize

	var usagePct float64
	if total > 0 {
		usagePct = 100.0 * float64(used) / float64(total)
	}

	return &MemoryStats{
		TotalBytes:     total,
		UsedBytes:      used,
		FreeBytes:      freeBytes,
		AvailableBytes: freeBytes,
		UsagePercent:   usagePct,
	}, nil
}
