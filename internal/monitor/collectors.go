package monitor

import (
	"fmt"
	"strconv"
	"strings"
)

// parseMemInfo parses the content of /proc/meminfo and returns MemoryStats and SwapStats.
// SwapStats is nil if no swap is configured (SwapTotal == 0).
func parseMemInfo(output string) (MemoryStats, *SwapStats, error) {
	var s MemoryStats
	vals := make(map[string]int64)

	for _, line := range strings.Split(output, "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSuffix(parts[0], ":")
		val, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		vals[key] = val // values are in kB
	}

	s.TotalBytes = vals["MemTotal"] * 1024
	s.FreeBytes = vals["MemFree"] * 1024
	s.AvailableBytes = vals["MemAvailable"] * 1024
	// Used = total - free - buffers - cached (excludes reclaimable cache)
	used := vals["MemTotal"] - vals["MemFree"] - vals["Buffers"] - vals["Cached"]
	if used < 0 {
		used = 0
	}
	s.UsedBytes = used * 1024
	if s.TotalBytes > 0 {
		s.UsagePercent = float64(s.UsedBytes) / float64(s.TotalBytes) * 100
	}

	var swap *SwapStats
	swapTotal := vals["SwapTotal"]
	swapFree := vals["SwapFree"]
	if swapTotal > 0 {
		swapUsed := swapTotal - swapFree
		var pct float64
		if swapTotal > 0 {
			pct = float64(swapUsed) / float64(swapTotal) * 100
		}
		swap = &SwapStats{
			TotalBytes: swapTotal * 1024,
			UsedBytes:  swapUsed * 1024,
			FreeBytes:  swapFree * 1024,
			Percent:    pct,
		}
	}

	return s, swap, nil
}

// parseDiskInfo parses POSIX-format df output (df -B1 -P …).
//
// Example line:
//
// /dev/sda1    104857600000 45097156608 54452572160  46% /
func parseDiskInfo(output string) ([]DiskStats, error) {
	var disks []DiskStats
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return disks, nil
	}
	for _, line := range lines[1:] { // skip header
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		total, _ := strconv.ParseInt(fields[1], 10, 64)
		used, _ := strconv.ParseInt(fields[2], 10, 64)
		free, _ := strconv.ParseInt(fields[3], 10, 64)
		pctStr := strings.TrimSuffix(fields[4], "%")
		pct, _ := strconv.ParseFloat(pctStr, 64)

		disks = append(disks, DiskStats{
			Device:       fields[0],
			TotalBytes:   total,
			UsedBytes:    used,
			FreeBytes:    free,
			UsagePercent: pct,
			MountPoint:   fields[5],
		})
	}
	return disks, nil
}

// parseNetDev parses /proc/net/dev and returns per-interface counters.
func parseNetDev(output string) ([]NetworkStats, error) {
	var stats []NetworkStats
	lines := strings.Split(output, "\n")
	if len(lines) < 3 {
		return stats, nil
	}
	for _, line := range lines[2:] { // skip two header lines
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		iface := strings.TrimSpace(line[:idx])
		fields := strings.Fields(line[idx+1:])
		if len(fields) < 10 {
			continue
		}
		bytesRecv, _ := strconv.ParseInt(fields[0], 10, 64)
		packetsRecv, _ := strconv.ParseInt(fields[1], 10, 64)
		bytesSent, _ := strconv.ParseInt(fields[8], 10, 64)
		packetsSent, _ := strconv.ParseInt(fields[9], 10, 64)

		stats = append(stats, NetworkStats{
			Interface:   iface,
			BytesRecv:   bytesRecv,
			BytesSent:   bytesSent,
			PacketsRecv: packetsRecv,
			PacketsSent: packetsSent,
		})
	}
	return stats, nil
}

// parseLoadAvg parses /proc/loadavg (e.g. "0.52 0.58 0.59 1/432 12345").
func parseLoadAvg(output string) (LoadStats, error) {
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) < 3 {
		return LoadStats{}, fmt.Errorf("unexpected loadavg format: %q", output)
	}
	l1, err1 := strconv.ParseFloat(fields[0], 64)
	l5, err2 := strconv.ParseFloat(fields[1], 64)
	l15, err3 := strconv.ParseFloat(fields[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return LoadStats{}, fmt.Errorf("parsing loadavg values from %q", output)
	}
	return LoadStats{Load1: l1, Load5: l5, Load15: l15}, nil
}

// parseUptime parses /proc/uptime (e.g. "12345.67 9876.54") and returns
// the uptime in seconds.
func parseUptime(output string) (float64, error) {
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) < 1 {
		return 0, fmt.Errorf("empty uptime output")
	}
	return strconv.ParseFloat(fields[0], 64)
}

// cpuSample holds raw jiffie values from one /proc/stat cpu line.
type cpuSample struct {
	user    int64
	nice    int64
	system  int64
	idle    int64
	iowait  int64
	irq     int64
	softirq int64
	total   int64
}

// parseProcStat extracts the aggregate "cpu" line from /proc/stat output.
func parseProcStat(output string) (cpuSample, error) {
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return cpuSample{}, fmt.Errorf("too few fields in /proc/stat cpu line")
		}
		var s cpuSample
		vals := make([]int64, len(fields)-1)
		for i, f := range fields[1:] {
			v, err := strconv.ParseInt(f, 10, 64)
			if err != nil {
				return cpuSample{}, fmt.Errorf("parsing /proc/stat field %q: %w", f, err)
			}
			vals[i] = v
		}
		s.user = vals[0]
		s.nice = vals[1]
		s.system = vals[2]
		s.idle = vals[3]
		if len(vals) > 4 {
			s.iowait = vals[4]
		}
		if len(vals) > 5 {
			s.irq = vals[5]
		}
		if len(vals) > 6 {
			s.softirq = vals[6]
		}
		s.total = s.user + s.nice + s.system + s.idle + s.iowait + s.irq + s.softirq
		return s, nil
	}
	return cpuSample{}, fmt.Errorf("cpu line not found in /proc/stat output")
}

// calcCPUStats computes CPU utilisation from two consecutive /proc/stat readings.
func calcCPUStats(s1, s2 cpuSample) CPUStats {
	total := s2.total - s1.total
	if total == 0 {
		return CPUStats{}
	}
	pct := func(delta int64) float64 {
		return float64(delta) / float64(total) * 100
	}
	idle := pct(s2.idle - s1.idle)
	iowait := pct(s2.iowait - s1.iowait)
	user := pct(s2.user - s1.user)
	nice := pct(s2.nice - s1.nice)
	sys := pct(s2.system - s1.system)
	return CPUStats{
		UsagePercent:  100 - idle - iowait,
		UserPercent:   user + nice,
		SystemPercent: sys,
		IdlePercent:   idle,
		IOWaitPercent: iowait,
	}
}

// parseProcesses parses "ps aux" output (11+ space-separated columns).
//
// Expected header:
//
// USER PID %CPU %MEM VSZ RSS TTY STAT START TIME COMMAND…
func parseProcesses(output string) ([]ProcessInfo, error) {
	var procs []ProcessInfo
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return procs, nil
	}
	for _, line := range lines[1:] { // skip header
		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}
		pid, _ := strconv.Atoi(fields[1])
		cpu, _ := strconv.ParseFloat(fields[2], 64)
		mem, _ := strconv.ParseFloat(fields[3], 64)
		rss, _ := strconv.ParseInt(fields[5], 10, 64)
		procs = append(procs, ProcessInfo{
			User:       fields[0],
			PID:        pid,
			CPUPercent: cpu,
			MemPercent: mem,
			RSSBytes:   rss * 1024, // ps aux RSS is in KB
			State:      fields[7],
			Command:    strings.Join(fields[10:], " "),
		})
	}
	return procs, nil
}

// parseSystemInfo builds a SystemInfo from the output of "uname -snrm" and "hostname".
func parseSystemInfo(unameOut, hostnameOut string) SystemInfo {
	fields := strings.Fields(strings.TrimSpace(unameOut))
	info := SystemInfo{
		Hostname: strings.TrimSpace(hostnameOut),
	}
	if len(fields) >= 1 {
		info.OS = fields[0]
	}
	if len(fields) >= 3 {
		info.Kernel = fields[2]
	}
	if len(fields) >= 4 {
		info.Arch = fields[3]
	}
	return info
}
