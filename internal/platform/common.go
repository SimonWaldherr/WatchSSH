package platform

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// parseSystemInfo parses `uname -srm` output and hostname into a SystemInfo.
// uname -srm fields: OS Kernel Arch
func parseSystemInfo(unameOut, hostnameOut string) SystemInfo {
	fields := strings.Fields(strings.TrimSpace(unameOut))
	si := SystemInfo{Hostname: strings.TrimSpace(hostnameOut)}
	if len(fields) >= 1 {
		si.OS = fields[0]
	}
	if len(fields) >= 2 {
		si.Kernel = fields[1]
	}
	if len(fields) >= 3 {
		si.Arch = fields[2]
	}
	return si
}

// parseSysctlBootTime parses the BSD sysctl kern.boottime format:
//
//	{ sec = 1234567890, usec = 123456 }
//
// and returns elapsed seconds since boot.
func parseSysctlBootTime(output string) (float64, error) {
	s := strings.TrimSpace(output)
	// Strip surrounding braces if present
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	s = strings.TrimSpace(s)

	var bootSec int64
	for _, part := range strings.Split(s, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])
		if key == "sec" {
			v, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("parseSysctlBootTime: bad sec %q: %w", val, err)
			}
			bootSec = v
		}
	}
	if bootSec == 0 {
		return 0, fmt.Errorf("parseSysctlBootTime: sec not found in %q", output)
	}
	uptime := float64(time.Now().Unix() - bootSec)
	if uptime < 0 {
		uptime = 0
	}
	return uptime, nil
}

// parseSysctlLoadAvg parses the BSD sysctl vm.loadavg format:
//
//	{ 0.52 0.48 0.41 }
func parseSysctlLoadAvg(output string) (*LoadAvg, error) {
	s := strings.TrimSpace(output)
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	s = strings.TrimSpace(s)

	fields := strings.Fields(s)
	if len(fields) < 3 {
		return nil, fmt.Errorf("parseSysctlLoadAvg: expected 3 fields, got %d in %q", len(fields), output)
	}

	parse := func(f string) (float64, error) {
		return strconv.ParseFloat(f, 64)
	}
	l1, err := parse(fields[0])
	if err != nil {
		return nil, fmt.Errorf("parseSysctlLoadAvg: %w", err)
	}
	l5, err := parse(fields[1])
	if err != nil {
		return nil, fmt.Errorf("parseSysctlLoadAvg: %w", err)
	}
	l15, err := parse(fields[2])
	if err != nil {
		return nil, fmt.Errorf("parseSysctlLoadAvg: %w", err)
	}
	return &LoadAvg{Load1: l1, Load5: l5, Load15: l15}, nil
}

// parseDFOutput parses POSIX `df -kP` output (1024-byte blocks).
// Example:
//
//	Filesystem     1024-blocks      Used Available Capacity Mounted on
//	/dev/sda1           102400     44040     58360      44% /
func parseDFOutput(output string) ([]DiskStats, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("parseDFOutput: no data rows")
	}

	var disks []DiskStats
	for _, line := range lines[1:] { // skip header
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
			TotalBytes:   total * 1024,
			UsedBytes:    used * 1024,
			FreeBytes:    avail * 1024,
			UsagePercent: pct,
		})
	}
	return disks, nil
}

// parsePSAux parses BSD/Linux `ps aux` output.
// Header: USER PID %CPU %MEM VSZ RSS TTY STAT START TIME COMMAND
// Takes up to 10 entries (skips header).
func parsePSAux(output string) ([]ProcessInfo, error) {
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
		if len(fields) < 11 {
			continue
		}
		pid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		cpu, _ := strconv.ParseFloat(fields[2], 64)
		mem, _ := strconv.ParseFloat(fields[3], 64)
		rss, _ := strconv.ParseInt(fields[5], 10, 64)
		command := strings.Join(fields[10:], " ")

		procs = append(procs, ProcessInfo{
			PID:        pid,
			User:       fields[0],
			CPUPercent: cpu,
			MemPercent: mem,
			RSSBytes:   rss * 1024, // ps aux RSS is in KB
			State:      fields[7],
			Command:    command,
		})
	}
	return procs, nil
}
