package platform

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// CollectDocker collects Docker container metrics via the docker CLI.
// It is only supported on Linux; calling it on other platforms returns an
// "unsupported" capability and an empty container list.
//
// Discovery uses `docker ps` and per-container stats are gathered with
// `docker stats --no-stream` in a single invocation.
//
// When Docker is absent or the daemon is inaccessible, the "containers"
// capability is set to "unavailable" and no error is returned — the caller
// can inspect snap.Caps["containers"] to determine the reason.
func CollectDocker(ctx context.Context, r Runner, family Family, snap *Snapshot) {
	if family != Linux {
		snap.setUnsupported("containers")
		return
	}

	// Verify Docker is available and the daemon is reachable.
	if _, err := r.Run(ctx, "docker version --format '{{.Server.Version}}' 2>/dev/null"); err != nil {
		if snap.Caps == nil {
			snap.Caps = map[string]string{}
		}
		snap.Caps["containers"] = string(CapUnavailable)
		if snap.Errors == nil {
			snap.Errors = map[string]string{}
		}
		snap.Errors["containers"] = "docker daemon not reachable: " + err.Error()
		return
	}

	// Discover running containers: ID, name, image, status.
	psOut, err := r.Run(ctx, "docker ps --format '{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Status}}' 2>/dev/null")
	if err != nil {
		snap.setErr("containers", fmt.Sprintf("docker ps: %v", err))
		return
	}

	containers, err := parseDockerPS(psOut)
	if err != nil {
		snap.setErr("containers", fmt.Sprintf("parse docker ps: %v", err))
		return
	}

	if len(containers) == 0 {
		snap.setOK("containers")
		return
	}

	// Gather resource stats for all running containers in one call.
	statsOut, err := r.Run(ctx, "docker stats --no-stream --format '{{.ID}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}\t{{.BlockIO}}' 2>/dev/null")
	if err != nil {
		snap.setErr("containers", fmt.Sprintf("docker stats: %v", err))
		return
	}

	statsMap, err := parseDockerStats(statsOut)
	if err != nil {
		snap.setErr("containers", fmt.Sprintf("parse docker stats: %v", err))
		return
	}

	// Merge stats into container list.
	for i := range containers {
		if st, ok := statsMap[containers[i].ID]; ok {
			containers[i].CPUPercent = st.CPUPercent
			containers[i].MemUsedBytes = st.MemUsedBytes
			containers[i].MemLimitBytes = st.MemLimitBytes
			containers[i].MemPercent = st.MemPercent
			containers[i].NetRxBytes = st.NetRxBytes
			containers[i].NetTxBytes = st.NetTxBytes
			containers[i].BlockReadBytes = st.BlockReadBytes
			containers[i].BlockWriteBytes = st.BlockWriteBytes
		}
	}

	snap.Containers = containers
	snap.setOK("containers")
}

// parseDockerPS parses `docker ps --format '{{.ID}}\t{{.Names}}\t{{.Image}}\t{{.Status}}'` output.
func parseDockerPS(output string) ([]ContainerInfo, error) {
	var containers []ContainerInfo
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 4)
		if len(fields) < 4 {
			continue
		}
		containers = append(containers, ContainerInfo{
			ID:     strings.TrimSpace(fields[0]),
			Name:   strings.TrimSpace(fields[1]),
			Image:  strings.TrimSpace(fields[2]),
			Status: strings.TrimSpace(fields[3]),
		})
	}
	return containers, nil
}

// dockerStatEntry holds parsed resource data for one container.
type dockerStatEntry struct {
	CPUPercent      float64
	MemUsedBytes    int64
	MemLimitBytes   int64
	MemPercent      float64
	NetRxBytes      int64
	NetTxBytes      int64
	BlockReadBytes  int64
	BlockWriteBytes int64
}

// parseDockerStats parses `docker stats --no-stream --format '{{.ID}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}\t{{.BlockIO}}'` output.
// Returns a map of short container ID → stats.
func parseDockerStats(output string) (map[string]dockerStatEntry, error) {
	result := make(map[string]dockerStatEntry)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 5)
		if len(fields) < 5 {
			continue
		}
		id := strings.TrimSpace(fields[0])

		// CPUPerc: "5.23%"
		cpuStr := strings.TrimSuffix(strings.TrimSpace(fields[1]), "%")
		cpu, _ := strconv.ParseFloat(cpuStr, 64)

		// MemUsage: "123MiB / 1GiB" or "123MiB / 1.00GiB"
		memUsed, memLimit := parseDockerMemUsage(strings.TrimSpace(fields[2]))

		var memPct float64
		if memLimit > 0 {
			memPct = 100.0 * float64(memUsed) / float64(memLimit)
		}

		// NetIO: "1.23kB / 456B"
		netRx, netTx := parseDockerIO(strings.TrimSpace(fields[3]))

		// BlockIO: "1.23MB / 456kB"
		blkRead, blkWrite := parseDockerIO(strings.TrimSpace(fields[4]))

		result[id] = dockerStatEntry{
			CPUPercent:      cpu,
			MemUsedBytes:    memUsed,
			MemLimitBytes:   memLimit,
			MemPercent:      memPct,
			NetRxBytes:      netRx,
			NetTxBytes:      netTx,
			BlockReadBytes:  blkRead,
			BlockWriteBytes: blkWrite,
		}
	}
	return result, nil
}

// parseDockerMemUsage parses docker MemUsage strings like "123MiB / 1GiB".
// Returns (used, limit) in bytes.
func parseDockerMemUsage(s string) (int64, int64) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	used := parseDockerSize(strings.TrimSpace(parts[0]))
	limit := parseDockerSize(strings.TrimSpace(parts[1]))
	return used, limit
}

// parseDockerIO parses docker NetIO / BlockIO strings like "1.23kB / 456B".
// Returns (rx/read, tx/write) in bytes.
func parseDockerIO(s string) (int64, int64) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	rx := parseDockerSize(strings.TrimSpace(parts[0]))
	tx := parseDockerSize(strings.TrimSpace(parts[1]))
	return rx, tx
}

// parseDockerSize parses Docker human-readable size strings.
// Handles: B, kB, MB, GB, TB, PB and their binary equivalents: KiB, MiB, GiB, TiB, PiB.
// Returns bytes as int64; returns 0 on parse error.
func parseDockerSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "--" {
		return 0
	}

	// Find where the numeric part ends.
	i := 0
	for i < len(s) && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	if i == 0 {
		return 0
	}
	numStr := s[:i]
	suffix := strings.TrimSpace(s[i:])

	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0
	}

	switch strings.ToLower(suffix) {
	case "b", "":
		return int64(num)
	case "kb":
		return int64(num * 1e3)
	case "mb":
		return int64(num * 1e6)
	case "gb":
		return int64(num * 1e9)
	case "tb":
		return int64(num * 1e12)
	case "pb":
		return int64(num * 1e15)
	case "kib":
		return int64(num * 1024)
	case "mib":
		return int64(num * 1024 * 1024)
	case "gib":
		return int64(num * 1024 * 1024 * 1024)
	case "tib":
		return int64(num * 1024 * 1024 * 1024 * 1024)
	case "pib":
		return int64(num * 1024 * 1024 * 1024 * 1024 * 1024)
	default:
		return int64(num)
	}
}
