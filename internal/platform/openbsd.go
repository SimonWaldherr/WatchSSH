package platform

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type openbsdCollector struct{}

// obsdCPUSample holds raw kern.cp_time counters for OpenBSD.
type obsdCPUSample struct {
	user, nice, sys, spin, intr, idle int64
	total                             int64
}

func (c *openbsdCollector) Collect(ctx context.Context, r Runner) (*Snapshot, error) {
	s := &Snapshot{}

	// 1. SystemInfo
	unameOut, _ := r.Run(ctx, "uname -srm")
	hostOut, _ := r.Run(ctx, "hostname")
	s.SystemInfo = parseSystemInfo(unameOut, hostOut)

	// 2. Uptime — OpenBSD kern.boottime may be plain integer or BSD {sec=...} format
	btOut, err := r.Run(ctx, "sysctl -n kern.boottime")
	if err != nil {
		s.setErr("uptime", err.Error())
	} else {
		u, err := parseOpenBSDBootTime(btOut)
		if err != nil {
			s.setErr("uptime", err.Error())
		} else {
			s.UptimeSecs = &u
			s.setOK("uptime")
		}
	}

	// 3. Load via vm.loadavg (same { x y z } format)
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

	// 4. Memory: uvmexp page counts
	uvmOut, err := r.Run(ctx, "sysctl -n vm.uvmexp.npages vm.uvmexp.free vm.uvmexp.active hw.pagesize")
	if err != nil {
		s.setErr("memory", err.Error())
	} else {
		physOut, _ := r.Run(ctx, "sysctl -n hw.physmem")
		physMem, _ := strconv.ParseInt(strings.TrimSpace(physOut), 10, 64)
		mem, err := parseOpenBSDVmstat(uvmOut, physMem)
		if err != nil {
			s.setErr("memory", err.Error())
		} else {
			s.Memory = mem
			s.setOK("memory")
		}
	}

	// 5. Swap via swapctl -s -k
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

	// 6. CPU: two kern.cp_time samples 1s apart
	cp1Out, err := r.Run(ctx, "sysctl kern.cp_time")
	if err != nil {
		s.setErr("cpu", err.Error())
	} else {
		sample1, err := parseOpenBSDCPTime(cp1Out)
		if err != nil {
			s.setErr("cpu", err.Error())
		} else {
			select {
			case <-ctx.Done():
				s.setErr("cpu", ctx.Err().Error())
			case <-time.After(time.Second):
				cp2Out, err := r.Run(ctx, "sysctl kern.cp_time")
				if err != nil {
					s.setErr("cpu", err.Error())
				} else {
					sample2, err := parseOpenBSDCPTime(cp2Out)
					if err != nil {
						s.setErr("cpu", err.Error())
					} else {
						s.CPU = calcOpenBSDCPU(sample1, sample2)
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

// parseOpenBSDBootTime handles both plain-integer and BSD {sec=...} formats.
// OpenBSD kern.boottime may be a plain seconds-since-epoch integer.
func parseOpenBSDBootTime(output string) (float64, error) {
	s := strings.TrimSpace(output)
	// Try plain integer first
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		uptime := float64(time.Now().Unix() - v)
		if uptime < 0 {
			uptime = 0
		}
		return uptime, nil
	}
	// Fall back to BSD { sec = ... } format
	return parseSysctlBootTime(s)
}

// parseOpenBSDVmstat parses sysctl uvmexp output (4 lines: npages, free, active, pagesize).
func parseOpenBSDVmstat(output string, physMemBytes int64) (*MemoryStats, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 4 {
		return nil, fmt.Errorf("parseOpenBSDVmstat: expected 4 lines, got %d", len(lines))
	}
	parseInt := func(s string) (int64, error) {
		return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	}
	npages, err := parseInt(lines[0])
	if err != nil {
		return nil, fmt.Errorf("parseOpenBSDVmstat: npages: %w", err)
	}
	free, err := parseInt(lines[1])
	if err != nil {
		return nil, fmt.Errorf("parseOpenBSDVmstat: free: %w", err)
	}
	// active is lines[2] but not needed for basic calculation
	pageSize, err := parseInt(lines[3])
	if err != nil {
		return nil, fmt.Errorf("parseOpenBSDVmstat: pagesize: %w", err)
	}

	total := npages * pageSize
	freeBytes := free * pageSize
	used := (npages - free) * pageSize

	// Use physmem as total if available and larger (more accurate)
	if physMemBytes > total {
		total = physMemBytes
	}

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

// parseOpenBSDSwapctl parses `swapctl -s -k` output.
// Format: "total: 1048576 1K-blocks allocated, 512 used, 1048064 available"
func parseOpenBSDSwapctl(output string) (*SwapStats, error) {
	s := strings.TrimSpace(output)
	if s == "" || strings.Contains(strings.ToLower(s), "no swap") {
		return nil, nil
	}

	// Extract numbers: total, used, available (in 1K blocks)
	var totalKB, usedKB, availKB int64
	// Parse "total: X 1K-blocks allocated, Y used, Z available"
	// Remove "total:" prefix
	s = strings.TrimPrefix(s, "total:")
	s = strings.TrimSpace(s)

	parts := strings.Split(s, ",")
	if len(parts) < 3 {
		return nil, fmt.Errorf("parseOpenBSDSwapctl: unexpected format %q", output)
	}

	// First part: "X 1K-blocks allocated"
	firstFields := strings.Fields(strings.TrimSpace(parts[0]))
	if len(firstFields) >= 1 {
		v, err := strconv.ParseInt(firstFields[0], 10, 64)
		if err == nil {
			totalKB = v
		}
	}

	// Second part: " Y used"
	secondFields := strings.Fields(strings.TrimSpace(parts[1]))
	if len(secondFields) >= 1 {
		v, err := strconv.ParseInt(secondFields[0], 10, 64)
		if err == nil {
			usedKB = v
		}
	}

	// Third part: " Z available"
	thirdFields := strings.Fields(strings.TrimSpace(parts[2]))
	if len(thirdFields) >= 1 {
		v, err := strconv.ParseInt(thirdFields[0], 10, 64)
		if err == nil {
			availKB = v
		}
	}

	if totalKB == 0 {
		return nil, nil
	}

	var pct float64
	if totalKB > 0 {
		pct = 100.0 * float64(usedKB) / float64(totalKB)
	}

	return &SwapStats{
		TotalBytes: totalKB * 1024,
		UsedBytes:  usedKB * 1024,
		FreeBytes:  availKB * 1024,
		Percent:    pct,
	}, nil
}

// parseOpenBSDCPTime parses OpenBSD kern.cp_time format:
// kern.cp_time=user=217735,nice=0,sys=167138,spin=0,intr=14268,idle=6855742
func parseOpenBSDCPTime(output string) (obsdCPUSample, error) {
	s := strings.TrimSpace(output)
	// Strip "kern.cp_time=" prefix
	if idx := strings.Index(s, "="); idx >= 0 {
		s = s[idx+1:]
	}

	vals := make(map[string]int64)
	for _, part := range strings.Split(s, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		v, err := strconv.ParseInt(strings.TrimSpace(kv[1]), 10, 64)
		if err != nil {
			continue
		}
		vals[strings.TrimSpace(kv[0])] = v
	}

	sample := obsdCPUSample{
		user: vals["user"],
		nice: vals["nice"],
		sys:  vals["sys"],
		spin: vals["spin"],
		intr: vals["intr"],
		idle: vals["idle"],
	}
	sample.total = sample.user + sample.nice + sample.sys + sample.spin + sample.intr + sample.idle
	if sample.total == 0 {
		return obsdCPUSample{}, fmt.Errorf("parseOpenBSDCPTime: no values parsed from %q", output)
	}
	return sample, nil
}

// calcOpenBSDCPU computes CPU percentages from two obsd samples.
func calcOpenBSDCPU(s1, s2 obsdCPUSample) *CPUStats {
	delta := s2.total - s1.total
	if delta <= 0 {
		return &CPUStats{}
	}
	pct := func(a, b int64) float64 {
		return 100.0 * float64(a-b) / float64(delta)
	}
	user := pct(s2.user+s2.nice, s1.user+s1.nice)
	sys := pct(s2.sys+s2.spin+s2.intr, s1.sys+s1.spin+s1.intr)
	idle := pct(s2.idle, s1.idle)
	return &CPUStats{
		UsagePercent:  user + sys,
		UserPercent:   user,
		SystemPercent: sys,
		IdlePercent:   idle,
	}
}
