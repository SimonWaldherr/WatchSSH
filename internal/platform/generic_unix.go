package platform

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type genericUnixCollector struct {
	family Family
}

func (c *genericUnixCollector) Collect(ctx context.Context, r Runner) (*Snapshot, error) {
	s := &Snapshot{}

	unameOut, _ := r.Run(ctx, "uname -srm")
	hostOut, _ := r.Run(ctx, "hostname")
	s.SystemInfo = parseSystemInfo(unameOut, hostOut)
	if s.SystemInfo.OS == "" {
		s.SystemInfo.OS = string(c.family)
	}

	coresOut, err := r.Run(ctx, "getconf _NPROCESSORS_ONLN 2>/dev/null || psrinfo -p 2>/dev/null")
	if err != nil {
		s.setErr("cpu_cores", err.Error())
	} else if cores, err := parseCPUCores(coresOut); err != nil {
		s.setErr("cpu_cores", err.Error())
	} else {
		s.SystemInfo.CPUCores = cores
		s.setOK("cpu_cores")
	}

	procUptimeOut, err := r.Run(ctx, "cat /proc/uptime 2>/dev/null")
	if err == nil {
		if uptime, err := parseLinuxUptime(procUptimeOut); err == nil {
			s.UptimeSecs = &uptime
			s.setOK("uptime")
		}
	}
	if s.UptimeSecs == nil {
		s.setUnsupported("uptime")
	}

	uptimeOut, err := r.Run(ctx, "uptime 2>/dev/null")
	if err != nil {
		s.setErr("load", err.Error())
	} else if load, err := parseUptimeLoadAvg(uptimeOut); err != nil {
		s.setErr("load", err.Error())
	} else {
		s.Load = load
		s.setOK("load")
	}

	memOut, err := r.Run(ctx, "prtconf 2>/dev/null | grep 'Memory size' 2>/dev/null")
	if err == nil && strings.TrimSpace(memOut) != "" {
		if mem, err := parsePrtconfMemory(memOut); err != nil {
			s.setErr("memory", err.Error())
		} else {
			s.Memory = mem
			s.setOK("memory")
		}
	} else {
		s.setUnsupported("memory")
	}

	swapOut, err := r.Run(ctx, "swap -s 2>/dev/null")
	if err == nil && strings.TrimSpace(swapOut) != "" {
		if swap, err := parseSolarisSwapS(swapOut); err != nil {
			s.setErr("swap", err.Error())
		} else if swap == nil {
			s.setUnsupported("swap")
		} else {
			s.Swap = swap
			s.setOK("swap")
		}
	} else {
		s.setUnsupported("swap")
	}

	s.setUnsupported("cpu")

	dfOut, err := r.Run(ctx, "df -kP 2>/dev/null")
	if err != nil {
		s.setErr("disks", err.Error())
	} else if disks, err := parseDFOutput(dfOut); err != nil {
		s.setErr("disks", err.Error())
	} else {
		s.Disks = disks
		s.setOK("disks")
	}

	inodeOut, err := r.Run(ctx, "df -iP 2>/dev/null || df -i 2>/dev/null")
	if err != nil {
		s.setErr("disk_inodes", err.Error())
	} else if inodes, err := parseDFInodeOutput(inodeOut); err != nil {
		s.setErr("disk_inodes", err.Error())
	} else {
		s.Disks = mergeDiskInodes(s.Disks, inodes)
		s.setOK("disk_inodes")
	}

	netOut, err := r.Run(ctx, "netstat -ibn 2>/dev/null || netstat -i -n 2>/dev/null")
	if err != nil {
		s.setErr("network", err.Error())
	} else if nets, err := parseDarwinNetstat(netOut); err != nil {
		s.setErr("network", err.Error())
	} else {
		s.Network = nets
		s.setOK("network")
	}

	psOut, err := r.Run(ctx, "ps aux 2>/dev/null | head -11")
	if err != nil {
		s.setErr("processes", err.Error())
	} else if procs, err := parsePSAux(psOut); err != nil {
		s.setErr("processes", err.Error())
	} else {
		s.Processes = procs
		s.setOK("processes")
	}

	s.setUnsupported("file_descriptors")
	collectStandardUnixMetrics(ctx, r, s)
	return s, nil
}

func parseUptimeLoadAvg(output string) (*LoadAvg, error) {
	idx := strings.LastIndex(strings.ToLower(output), "load average")
	if idx < 0 {
		idx = strings.LastIndex(strings.ToLower(output), "load averages")
	}
	if idx < 0 {
		return nil, fmt.Errorf("parseUptimeLoadAvg: load average not found in %q", output)
	}
	tail := output[idx:]
	re := regexp.MustCompile(`[-+]?\d+(?:\.\d+)?`)
	matches := re.FindAllString(tail, 3)
	if len(matches) < 3 {
		return nil, fmt.Errorf("parseUptimeLoadAvg: expected 3 values in %q", output)
	}
	l1, _ := strconv.ParseFloat(matches[0], 64)
	l5, _ := strconv.ParseFloat(matches[1], 64)
	l15, _ := strconv.ParseFloat(matches[2], 64)
	return &LoadAvg{Load1: l1, Load5: l5, Load15: l15}, nil
}

func parsePrtconfMemory(output string) (*MemoryStats, error) {
	re := regexp.MustCompile(`(?i)memory\s+size:\s+([0-9.]+)\s*([a-z]+)?`)
	m := re.FindStringSubmatch(output)
	if len(m) < 2 {
		return nil, fmt.Errorf("parsePrtconfMemory: no memory size in %q", output)
	}
	value, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return nil, fmt.Errorf("parsePrtconfMemory: %w", err)
	}
	unit := strings.ToLower(m[2])
	mult := float64(1024 * 1024)
	switch unit {
	case "kb", "k", "kilobytes":
		mult = 1024
	case "gb", "g", "gigabytes":
		mult = 1024 * 1024 * 1024
	case "mb", "m", "megabytes", "":
		mult = 1024 * 1024
	}
	total := int64(value * mult)
	return &MemoryStats{TotalBytes: total}, nil
}

func parseSolarisSwapS(output string) (*SwapStats, error) {
	re := regexp.MustCompile(`(?i)([0-9]+)k\s+used,\s+([0-9]+)k\s+available`)
	m := re.FindStringSubmatch(output)
	if len(m) != 3 {
		return nil, fmt.Errorf("parseSolarisSwapS: unexpected format %q", output)
	}
	usedKB, _ := strconv.ParseInt(m[1], 10, 64)
	freeKB, _ := strconv.ParseInt(m[2], 10, 64)
	totalKB := usedKB + freeKB
	if totalKB == 0 {
		return nil, nil
	}
	used := usedKB * 1024
	free := freeKB * 1024
	total := totalKB * 1024
	return &SwapStats{
		TotalBytes: total,
		UsedBytes:  used,
		FreeBytes:  free,
		Percent:    float64(used) / float64(total) * 100,
	}, nil
}
