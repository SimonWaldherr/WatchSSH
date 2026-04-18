package platform

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type linuxCollector struct{}

// linuxCPUSample holds raw /proc/stat CPU counters.
type linuxCPUSample struct {
	user, nice, system, idle, iowait, irq, softirq int64
	total                                          int64
}

func (c *linuxCollector) Collect(ctx context.Context, r Runner) (*Snapshot, error) {
	s := &Snapshot{}

	// 1. SystemInfo
	unameOut, err := r.Run(ctx, "uname -srm")
	if err != nil {
		unameOut = ""
	}
	hostOut, err := r.Run(ctx, "hostname")
	if err != nil {
		hostOut = ""
	}
	s.SystemInfo = parseSystemInfo(unameOut, hostOut)

	// 2. Uptime from /proc/uptime
	uptimeOut, err := r.Run(ctx, "cat /proc/uptime")
	if err != nil {
		s.setErr("uptime", err.Error())
	} else {
		u, err := parseLinuxUptime(uptimeOut)
		if err != nil {
			s.setErr("uptime", err.Error())
		} else {
			s.UptimeSecs = &u
			s.setOK("uptime")
		}
	}

	// 3. Load from /proc/loadavg
	loadOut, err := r.Run(ctx, "cat /proc/loadavg")
	if err != nil {
		s.setErr("load", err.Error())
	} else {
		la, err := parseLinuxLoadAvg(loadOut)
		if err != nil {
			s.setErr("load", err.Error())
		} else {
			s.Load = la
			s.setOK("load")
		}
	}

	// 4. Memory + Swap from /proc/meminfo
	memOut, err := r.Run(ctx, "cat /proc/meminfo")
	if err != nil {
		s.setErr("memory", err.Error())
		s.setErr("swap", err.Error())
	} else {
		mem, swap, err := parseLinuxMemInfo(memOut)
		if err != nil {
			s.setErr("memory", err.Error())
		} else {
			s.Memory = mem
			s.setOK("memory")
			if swap != nil {
				s.Swap = swap
				s.setOK("swap")
			} else {
				s.setUnsupported("swap")
			}
		}
	}

	// 5. CPU: two /proc/stat samples 1s apart
	stat1Out, err := r.Run(ctx, "cat /proc/stat")
	if err != nil {
		s.setErr("cpu", err.Error())
	} else {
		sample1, err := parseLinuxProcStat(stat1Out)
		if err != nil {
			s.setErr("cpu", err.Error())
		} else {
			// Wait 1s while respecting context cancellation
			select {
			case <-ctx.Done():
				s.setErr("cpu", ctx.Err().Error())
			case <-time.After(time.Second):
				stat2Out, err := r.Run(ctx, "cat /proc/stat")
				if err != nil {
					s.setErr("cpu", err.Error())
				} else {
					sample2, err := parseLinuxProcStat(stat2Out)
					if err != nil {
						s.setErr("cpu", err.Error())
					} else {
						s.CPU = calcLinuxCPU(sample1, sample2)
						s.setOK("cpu")
					}
				}
			}
		}
	}

	// 6. Disks via df -B1 -P
	dfOut, err := r.Run(ctx, "df -B1 -P -x tmpfs -x devtmpfs -x squashfs 2>/dev/null")
	if err != nil {
		s.setErr("disks", err.Error())
	} else {
		disks, err := parseLinuxDf(dfOut)
		if err != nil {
			s.setErr("disks", err.Error())
		} else {
			s.Disks = disks
			s.setOK("disks")
		}
	}

	// 7. Network from /proc/net/dev
	netOut, err := r.Run(ctx, "cat /proc/net/dev")
	if err != nil {
		s.setErr("network", err.Error())
	} else {
		nets, err := parseLinuxNetDev(netOut)
		if err != nil {
			s.setErr("network", err.Error())
		} else {
			s.Network = nets
			s.setOK("network")
		}
	}

	// 8. Processes
	psOut, err := r.Run(ctx, "ps -eo pid,user,%cpu,%mem,rss,stat,comm --no-headers --sort=-%cpu 2>/dev/null | head -10")
	if err != nil {
		s.setErr("processes", err.Error())
	} else {
		procs, err := parseLinuxPS(psOut)
		if err != nil {
			s.setErr("processes", err.Error())
		} else {
			s.Processes = procs
			s.setOK("processes")
		}
	}

	return s, nil
}

// parseLinuxUptime parses /proc/uptime. First field is uptime in seconds.
func parseLinuxUptime(output string) (float64, error) {
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) < 1 {
		return 0, fmt.Errorf("parseLinuxUptime: empty output")
	}
	return strconv.ParseFloat(fields[0], 64)
}

// parseLinuxLoadAvg parses /proc/loadavg.
// Format: 0.52 0.48 0.41 1/234 5678
func parseLinuxLoadAvg(output string) (*LoadAvg, error) {
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) < 3 {
		return nil, fmt.Errorf("parseLinuxLoadAvg: expected >=3 fields, got %d", len(fields))
	}
	l1, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return nil, fmt.Errorf("parseLinuxLoadAvg: %w", err)
	}
	l5, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return nil, fmt.Errorf("parseLinuxLoadAvg: %w", err)
	}
	l15, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return nil, fmt.Errorf("parseLinuxLoadAvg: %w", err)
	}
	return &LoadAvg{Load1: l1, Load5: l5, Load15: l15}, nil
}

// parseLinuxMemInfo parses /proc/meminfo and extracts memory and swap stats.
func parseLinuxMemInfo(output string) (*MemoryStats, *SwapStats, error) {
	vals := make(map[string]int64)
	for _, line := range strings.Split(output, "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSuffix(parts[0], ":")
		v, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		vals[key] = v // values are in kB
	}

	total, ok := vals["MemTotal"]
	if !ok {
		return nil, nil, fmt.Errorf("parseLinuxMemInfo: MemTotal not found")
	}
	free := vals["MemFree"]
	available := vals["MemAvailable"]
	buffers := vals["Buffers"]
	cached := vals["Cached"]
	// used = total - free - buffers - cached
	used := total - free - buffers - cached
	if used < 0 {
		used = 0
	}

	var usagePct float64
	if total > 0 {
		usagePct = 100.0 * float64(used) / float64(total)
	}

	mem := &MemoryStats{
		TotalBytes:     total * 1024,
		UsedBytes:      used * 1024,
		FreeBytes:      free * 1024,
		AvailableBytes: available * 1024,
		UsagePercent:   usagePct,
	}

	var swap *SwapStats
	swapTotal, hasSwapTotal := vals["SwapTotal"]
	swapFree := vals["SwapFree"]
	if hasSwapTotal && swapTotal > 0 {
		swapUsed := swapTotal - swapFree
		var swapPct float64
		if swapTotal > 0 {
			swapPct = 100.0 * float64(swapUsed) / float64(swapTotal)
		}
		swap = &SwapStats{
			TotalBytes: swapTotal * 1024,
			UsedBytes:  swapUsed * 1024,
			FreeBytes:  swapFree * 1024,
			Percent:    swapPct,
		}
	}

	return mem, swap, nil
}

// parseLinuxProcStat parses the first 'cpu' line from /proc/stat.
func parseLinuxProcStat(output string) (linuxCPUSample, error) {
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		// fields[0]="cpu" fields[1..]=user nice system idle iowait irq softirq ...
		if len(fields) < 8 {
			return linuxCPUSample{}, fmt.Errorf("parseLinuxProcStat: too few fields")
		}
		parseInt := func(s string) int64 {
			v, _ := strconv.ParseInt(s, 10, 64)
			return v
		}
		s := linuxCPUSample{
			user:    parseInt(fields[1]),
			nice:    parseInt(fields[2]),
			system:  parseInt(fields[3]),
			idle:    parseInt(fields[4]),
			iowait:  parseInt(fields[5]),
			irq:     parseInt(fields[6]),
			softirq: parseInt(fields[7]),
		}
		s.total = s.user + s.nice + s.system + s.idle + s.iowait + s.irq + s.softirq
		return s, nil
	}
	return linuxCPUSample{}, fmt.Errorf("parseLinuxProcStat: cpu line not found")
}

// calcLinuxCPU computes CPU percentages from two /proc/stat samples.
func calcLinuxCPU(s1, s2 linuxCPUSample) *CPUStats {
	deltaTotal := s2.total - s1.total
	if deltaTotal <= 0 {
		return &CPUStats{}
	}
	pct := func(a, b int64) float64 {
		return 100.0 * float64(a-b) / float64(deltaTotal)
	}
	user := pct(s2.user+s2.nice, s1.user+s1.nice)
	sys := pct(s2.system+s2.irq+s2.softirq, s1.system+s1.irq+s1.softirq)
	idle := pct(s2.idle, s1.idle)
	iowait := pct(s2.iowait, s1.iowait)
	return &CPUStats{
		UsagePercent:  user + sys,
		UserPercent:   user,
		SystemPercent: sys,
		IdlePercent:   idle,
		IOWaitPercent: iowait,
	}
}

// parseLinuxDf parses `df -B1 -P` output where sizes are in bytes.
func parseLinuxDf(output string) ([]DiskStats, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return nil, nil
	}
	var disks []DiskStats
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		total, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		used, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			continue
		}
		avail, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil {
			continue
		}
		pctStr := strings.TrimSuffix(fields[4], "%")
		pct, _ := strconv.ParseFloat(pctStr, 64)

		disks = append(disks, DiskStats{
			Device:       fields[0],
			MountPoint:   fields[5],
			TotalBytes:   total,
			UsedBytes:    used,
			FreeBytes:    avail,
			UsagePercent: pct,
		})
	}
	return disks, nil
}

// parseLinuxNetDev parses /proc/net/dev.
// Format (after 2 header lines):
//
//	Interface: bytes packets errs drop ... | bytes packets errs drop ...
func parseLinuxNetDev(output string) ([]NetworkStats, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 3 {
		return nil, nil
	}
	var nets []NetworkStats
	for _, line := range lines[2:] { // skip two header lines
		// Split on colon to separate interface name from stats
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		iface := strings.TrimSpace(parts[0])
		fields := strings.Fields(parts[1])
		// rx: bytes packets errs drop fifo frame compressed multicast
		// tx: bytes packets errs drop fifo colls carrier compressed
		if len(fields) < 16 {
			continue
		}
		rxBytes, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			continue
		}
		rxPkts, _ := strconv.ParseInt(fields[1], 10, 64)
		txBytes, err := strconv.ParseInt(fields[8], 10, 64)
		if err != nil {
			continue
		}
		txPkts, _ := strconv.ParseInt(fields[9], 10, 64)

		nets = append(nets, NetworkStats{
			Interface:   iface,
			BytesRecv:   rxBytes,
			BytesSent:   txBytes,
			PacketsRecv: rxPkts,
			PacketsSent: txPkts,
		})
	}
	return nets, nil
}

// parseLinuxPS parses `ps -eo pid,user,%cpu,%mem,rss,stat,comm --no-headers` output.
func parseLinuxPS(output string) ([]ProcessInfo, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var procs []ProcessInfo
	for _, line := range lines {
		if len(procs) >= 10 {
			break
		}
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		cpu, _ := strconv.ParseFloat(fields[2], 64)
		mem, _ := strconv.ParseFloat(fields[3], 64)
		rss, _ := strconv.ParseInt(fields[4], 10, 64)
		command := strings.Join(fields[6:], " ")

		procs = append(procs, ProcessInfo{
			PID:        pid,
			User:       fields[1],
			CPUPercent: cpu,
			MemPercent: mem,
			RSSBytes:   rss * 1024, // rss from ps is in KB
			State:      fields[5],
			Command:    command,
		})
	}
	return procs, nil
}
