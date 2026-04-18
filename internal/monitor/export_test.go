// export_test.go exposes package-internal functions to external test packages.
// This file is only compiled during testing.
package monitor

// ParseMemInfo wraps parseMemInfo for black-box testing.
// Returns MemoryStats and optionally SwapStats (nil if no swap configured).
func ParseMemInfo(output string) (MemoryStats, *SwapStats, error) {
	return parseMemInfo(output)
}

// ParseDiskInfo wraps parseDiskInfo for black-box testing.
var ParseDiskInfo = parseDiskInfo

// ParseNetDev wraps parseNetDev for black-box testing.
var ParseNetDev = parseNetDev

// ParseLoadAvg wraps parseLoadAvg for black-box testing.
var ParseLoadAvg = parseLoadAvg

// ParseUptime wraps parseUptime for black-box testing.
var ParseUptime = parseUptime

// ParseProcesses wraps parseProcesses for black-box testing.
var ParseProcesses = parseProcesses

// ParseSystemInfo wraps parseSystemInfo for black-box testing.
var ParseSystemInfo = parseSystemInfo

// CalcCPUFromStatOutputs parses two /proc/stat outputs and returns CPU stats.
func CalcCPUFromStatOutputs(stat1, stat2 string) (CPUStats, error) {
	s1, err := parseProcStat(stat1)
	if err != nil {
		return CPUStats{}, err
	}
	s2, err := parseProcStat(stat2)
	if err != nil {
		return CPUStats{}, err
	}
	return calcCPUStats(s1, s2), nil
}
