package platform

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type freebsdCollector struct{}

// freebsdCPUSample holds raw kern.cp_time counters for FreeBSD.
type freebsdCPUSample struct {
	user, nice, sys, intr, idle int64
	total                       int64
}

func (c *freebsdCollector) Collect(ctx context.Context, r Runner) (*Snapshot, error) {
	s := &Snapshot{}

	// 1. SystemInfo
	unameOut, _ := r.Run(ctx, "uname -srm")
	hostOut, _ := r.Run(ctx, "hostname")
	s.SystemInfo = parseSystemInfo(unameOut, hostOut)

	// 2. Uptime via kern.boottime
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

	// 4. Memory via multiple sysctl values in one call
	memOut, err := r.Run(ctx, "sysctl -n hw.physmem hw.pagesize vm.stats.vm.v_free_count vm.stats.vm.v_inactive_count vm.stats.vm.v_active_count vm.stats.vm.v_wire_count")
	if err != nil {
		s.setErr("memory", err.Error())
	} else {
		mem, err := parseFreeBSDSysctlMem(memOut)
		if err != nil {
			s.setErr("memory", err.Error())
		} else {
			s.Memory = mem
			s.setOK("memory")
		}
	}

	// 5. Swap via swapinfo -k
	swapOut, err := r.Run(ctx, "swapinfo -k 2>/dev/null")
	if err != nil {
		s.setUnsupported("swap")
	} else {
		sw, err := parseFreeBSDSwapinfo(swapOut)
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

	// 8. Network — same netstat -ibn format as Darwin
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

	return s, nil
}

// parseFreeBSDSysctlMem parses sysctl output with one value per line:
// hw.physmem, hw.pagesize, v_free_count, v_inactive_count, v_active_count, v_wire_count
func parseFreeBSDSysctlMem(output string) (*MemoryStats, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 6 {
		return nil, fmt.Errorf("parseFreeBSDSysctlMem: expected 6 lines, got %d", len(lines))
	}
	parseInt := func(s string) (int64, error) {
		return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	}
	physmem, err := parseInt(lines[0])
	if err != nil {
		return nil, fmt.Errorf("parseFreeBSDSysctlMem: physmem: %w", err)
	}
	pageSize, err := parseInt(lines[1])
	if err != nil {
		return nil, fmt.Errorf("parseFreeBSDSysctlMem: pagesize: %w", err)
	}
	freeCount, err := parseInt(lines[2])
	if err != nil {
		return nil, fmt.Errorf("parseFreeBSDSysctlMem: v_free_count: %w", err)
	}
	inactiveCount, err := parseInt(lines[3])
	if err != nil {
		return nil, fmt.Errorf("parseFreeBSDSysctlMem: v_inactive_count: %w", err)
	}
	activeCount, err := parseInt(lines[4])
	if err != nil {
		return nil, fmt.Errorf("parseFreeBSDSysctlMem: v_active_count: %w", err)
	}
	wireCount, err := parseInt(lines[5])
	if err != nil {
		return nil, fmt.Errorf("parseFreeBSDSysctlMem: v_wire_count: %w", err)
	}

	total := physmem
	available := (freeCount + inactiveCount) * pageSize
	used := (activeCount + wireCount) * pageSize
	free := freeCount * pageSize

	var usagePct float64
	if total > 0 {
		usagePct = 100.0 * float64(used) / float64(total)
	}

	return &MemoryStats{
		TotalBytes:     total,
		UsedBytes:      used,
		FreeBytes:      free,
		AvailableBytes: available,
		UsagePercent:   usagePct,
	}, nil
}

// parseFreeBSDSwapinfo parses `swapinfo -k` output.
// Returns nil (not an error) if no swap devices are configured.
func parseFreeBSDSwapinfo(output string) (*SwapStats, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		// No swap configured
		return nil, nil
	}

	var totalKB, usedKB int64
	for _, line := range lines[1:] { // skip header
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		t, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		u, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			continue
		}
		totalKB += t
		usedKB += u
	}

	if totalKB == 0 {
		return nil, nil
	}

	freeKB := totalKB - usedKB
	var pct float64
	if totalKB > 0 {
		pct = 100.0 * float64(usedKB) / float64(totalKB)
	}

	return &SwapStats{
		TotalBytes: totalKB * 1024,
		UsedBytes:  usedKB * 1024,
		FreeBytes:  freeKB * 1024,
		Percent:    pct,
	}, nil
}

// parseFreeBSDCPTime parses `sysctl -n kern.cp_time` output.
// Format: "218340 0 167239 0 6855742"  (user nice sys intr idle)
func parseFreeBSDCPTime(output string) (freebsdCPUSample, error) {
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) < 5 {
		return freebsdCPUSample{}, fmt.Errorf("parseFreeBSDCPTime: expected 5 fields, got %d", len(fields))
	}
	parseInt := func(s string) int64 {
		v, _ := strconv.ParseInt(s, 10, 64)
		return v
	}
	s := freebsdCPUSample{
		user: parseInt(fields[0]),
		nice: parseInt(fields[1]),
		sys:  parseInt(fields[2]),
		intr: parseInt(fields[3]),
		idle: parseInt(fields[4]),
	}
	s.total = s.user + s.nice + s.sys + s.intr + s.idle
	return s, nil
}

// calcFreeBSDCPU computes CPU percentages from two kern.cp_time samples.
// FreeBSD does not expose iowait separately.
func calcFreeBSDCPU(s1, s2 freebsdCPUSample) *CPUStats {
	delta := s2.total - s1.total
	if delta <= 0 {
		return &CPUStats{}
	}
	pct := func(a, b int64) float64 {
		return 100.0 * float64(a-b) / float64(delta)
	}
	user := pct(s2.user+s2.nice, s1.user+s1.nice)
	sys := pct(s2.sys+s2.intr, s1.sys+s1.intr)
	idle := pct(s2.idle, s1.idle)
	return &CPUStats{
		UsagePercent:  user + sys,
		UserPercent:   user,
		SystemPercent: sys,
		IdlePercent:   idle,
	}
}
