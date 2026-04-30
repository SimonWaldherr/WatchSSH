package platform

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type darwinCollector struct{}

func (c *darwinCollector) Collect(ctx context.Context, r Runner) (*Snapshot, error) {
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

	// 4. Memory: hw.memsize + vm_stat
	physOut, err := r.Run(ctx, "sysctl -n hw.memsize")
	if err != nil {
		s.setErr("memory", err.Error())
	} else {
		physMem, err := strconv.ParseInt(strings.TrimSpace(physOut), 10, 64)
		if err != nil {
			s.setErr("memory", fmt.Sprintf("hw.memsize parse: %v", err))
		} else {
			vmstatOut, err := r.Run(ctx, "vm_stat")
			if err != nil {
				s.setErr("memory", err.Error())
			} else {
				mem, err := parseDarwinVMStat(vmstatOut, physMem)
				if err != nil {
					s.setErr("memory", err.Error())
				} else {
					s.Memory = mem
					s.setOK("memory")
				}
			}
		}
	}

	// 5. Swap via vm.swapusage
	swapOut, err := r.Run(ctx, "sysctl -n vm.swapusage")
	if err != nil {
		s.setUnsupported("swap")
	} else {
		sw, err := parseDarwinSwapUsage(swapOut)
		if err != nil {
			s.setErr("swap", err.Error())
		} else if sw == nil {
			s.setUnsupported("swap")
		} else {
			s.Swap = sw
			s.setOK("swap")
		}
	}

	// 6. CPU via `top -l 2 -n 0 -s 1` — take second sample's CPU line
	topOut, err := r.Run(ctx, "top -l 2 -n 0 -s 1")
	if err != nil {
		s.setErr("cpu", err.Error())
	} else {
		cpu, err := parseDarwinTopCPU(topOut)
		if err != nil {
			s.setErr("cpu", err.Error())
		} else {
			s.CPU = cpu
			s.setOK("cpu")
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

	// 8. Network via netstat -ibn
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
	psOut, err := r.Run(ctx, "ps -eo pid,user,%cpu,%mem,rss,stat,comm 2>/dev/null | sort -k3 -rn | head -10")
	if err != nil {
		s.setErr("processes", err.Error())
	} else {
		procs, err := parsePSEO(psOut)
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

// parseDarwinVMStat parses `vm_stat` output and returns MemoryStats.
// Page sizes and counts are extracted from the output.
func parseDarwinVMStat(output string, physMemBytes int64) (*MemoryStats, error) {
	lines := strings.Split(output, "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("parseDarwinVMStat: empty output")
	}

	// Extract page size from first line: "Mach Virtual Memory Statistics: (page size of 4096 bytes)"
	pageSize := int64(4096) // default
	firstLine := lines[0]
	if idx := strings.Index(firstLine, "page size of "); idx >= 0 {
		rest := firstLine[idx+len("page size of "):]
		rest = strings.Fields(rest)[0]
		if v, err := strconv.ParseInt(rest, 10, 64); err == nil {
			pageSize = v
		}
	}

	pages := make(map[string]int64)
	for _, line := range lines[1:] {
		// Format: "Pages free:                               97376."
		colonIdx := strings.LastIndex(line, ":")
		if colonIdx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:colonIdx])
		valStr := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line[colonIdx+1:]), "."))
		v, err := strconv.ParseInt(valStr, 10, 64)
		if err != nil {
			continue
		}
		pages[key] = v
	}

	free := pages["Pages free"]
	active := pages["Pages active"]
	inactive := pages["Pages inactive"]
	speculative := pages["Pages speculative"]
	wired := pages["Pages wired down"]

	used := (active + wired) * pageSize
	available := (free + inactive + speculative) * pageSize
	freeBytes := free * pageSize
	total := physMemBytes

	var usagePct float64
	if total > 0 {
		usagePct = 100.0 * float64(used) / float64(total)
	}

	return &MemoryStats{
		TotalBytes:     total,
		UsedBytes:      used,
		FreeBytes:      freeBytes,
		AvailableBytes: available,
		UsagePercent:   usagePct,
	}, nil
}

// parseDarwinSwapUsage parses `sysctl -n vm.swapusage` output.
// Example: "total = 1024.00M  used = 34.50M  free = 989.50M  (encrypted)"
func parseDarwinSwapUsage(output string) (*SwapStats, error) {
	s := strings.TrimSpace(output)
	if s == "" {
		return nil, nil
	}

	parseSize := func(val string) (int64, error) {
		val = strings.TrimSpace(val)
		if val == "" {
			return 0, fmt.Errorf("empty value")
		}
		suffix := val[len(val)-1]
		numStr := val[:len(val)-1]
		num, err := strconv.ParseFloat(numStr, 64)
		if err != nil {
			// Try without suffix (bytes)
			v, err2 := strconv.ParseFloat(val, 64)
			if err2 != nil {
				return 0, err
			}
			return int64(v), nil
		}
		switch suffix {
		case 'B', 'b':
			return int64(num), nil
		case 'K', 'k':
			return int64(num * 1024), nil
		case 'M', 'm':
			return int64(num * 1024 * 1024), nil
		case 'G', 'g':
			return int64(num * 1024 * 1024 * 1024), nil
		default:
			return 0, fmt.Errorf("unknown suffix %c", suffix)
		}
	}

	vals := make(map[string]int64)
	// Split on two or more spaces or on "  " to handle key = value pairs
	for _, part := range strings.Split(s, "  ") {
		part = strings.TrimSpace(part)
		if part == "" || strings.HasPrefix(part, "(") {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])
		v, err := parseSize(val)
		if err != nil {
			continue
		}
		vals[key] = v
	}

	total := vals["total"]
	used := vals["used"]
	free := vals["free"]

	if total == 0 && used == 0 && free == 0 {
		return nil, nil
	}

	var pct float64
	if total > 0 {
		pct = 100.0 * float64(used) / float64(total)
	}

	return &SwapStats{
		TotalBytes: total,
		UsedBytes:  used,
		FreeBytes:  free,
		Percent:    pct,
	}, nil
}

// parseDarwinTopCPU scans all "CPU usage:" lines and takes the LAST one
// (which corresponds to the second sample from `top -l 2`).
func parseDarwinTopCPU(output string) (*CPUStats, error) {
	var lastLine string
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "CPU usage:") {
			lastLine = line
		}
	}
	if lastLine == "" {
		return nil, fmt.Errorf("parseDarwinTopCPU: no CPU usage line found")
	}

	// Format: "CPU usage: 5.91% user, 6.54% sys, 87.53% idle"
	var userPct, sysPct, idlePct float64
	parts := strings.Split(lastLine, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		// Remove "CPU usage: " prefix from first part
		part = strings.TrimPrefix(part, "CPU usage:")
		part = strings.TrimSpace(part)

		fields := strings.Fields(part)
		if len(fields) < 2 {
			continue
		}
		pctStr := strings.TrimSuffix(fields[0], "%")
		v, err := strconv.ParseFloat(pctStr, 64)
		if err != nil {
			continue
		}
		switch fields[1] {
		case "user":
			userPct = v
		case "sys":
			sysPct = v
		case "idle":
			idlePct = v
		}
	}

	return &CPUStats{
		UsagePercent:  userPct + sysPct,
		UserPercent:   userPct,
		SystemPercent: sysPct,
		IdlePercent:   idlePct,
	}, nil
}

// parseDarwinNetstat parses `netstat -ibn` output.
// Only includes interfaces with <Link#N> in the Network column.
func parseDarwinNetstat(output string) ([]NetworkStats, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return nil, nil
	}

	var nets []NetworkStats
	for _, line := range lines[1:] { // skip header
		fields := strings.Fields(line)
		// Fields: Name Mtu Network Address Ipkts Ierrs Ibytes Opkts Oerrs Obytes Coll
		if len(fields) < 10 {
			continue
		}
		// Only process hardware link entries
		if !strings.HasPrefix(fields[2], "<Link#") {
			continue
		}
		ibytes, err := strconv.ParseInt(fields[6], 10, 64)
		if err != nil {
			continue
		}
		obytes, err := strconv.ParseInt(fields[9], 10, 64)
		if err != nil {
			continue
		}
		ipkts, _ := strconv.ParseInt(fields[4], 10, 64)
		opkts, _ := strconv.ParseInt(fields[7], 10, 64)

		nets = append(nets, NetworkStats{
			Interface:   fields[0],
			BytesRecv:   ibytes,
			BytesSent:   obytes,
			PacketsRecv: ipkts,
			PacketsSent: opkts,
		})
	}
	return nets, nil
}

// parsePSEO parses `ps -eo pid,user,%cpu,%mem,rss,stat,comm` output (macOS style, has header).
// RSS is in KB on macOS — convert to bytes.
func parsePSEO(output string) ([]ProcessInfo, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return nil, nil
	}

	var procs []ProcessInfo
	for _, line := range lines[1:] { // skip header
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
			RSSBytes:   rss * 1024, // RSS in KB on macOS
			State:      fields[5],
			Command:    command,
		})
	}
	return procs, nil
}
