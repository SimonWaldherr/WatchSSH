package monitor

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
)

// shellSingleQuote escapes s for safe interpolation as one single-quoted
// POSIX shell argument, so operator-supplied unit names, patterns, and
// priorities can never break out of the intended command.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func runToolProbe(ctx context.Context, r runner, timeout int, command string) (string, error) {
	if timeout <= 0 {
		return r.Run(ctx, command)
	}
	probeCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	return r.Run(probeCtx, command)
}

// runServiceChecks queries systemd unit state with `systemctl is-active`.
// It is read-only: WatchSSH never starts, stops, or restarts a unit.
func runServiceChecks(ctx context.Context, r runner, checks []config.ServiceCheck) []ServiceResult {
	results := make([]ServiceResult, 0, len(checks))
	for _, sc := range checks {
		serviceName := strings.TrimSuffix(sc.Unit, ".service")
		command := fmt.Sprintf(
			"if command -v systemctl >/dev/null 2>&1; then systemctl is-active %s 2>&1; elif command -v service >/dev/null 2>&1; then if service %s status >/dev/null 2>&1; then printf active; else printf inactive; fi; else printf unsupported; fi",
			shellSingleQuote(sc.Unit), shellSingleQuote(serviceName),
		)
		out, err := runToolProbe(ctx, r, sc.Timeout, command)
		state := strings.TrimSpace(out)
		result := ServiceResult{Name: sc.Name, Unit: sc.Unit, State: state}
		switch {
		case state == "unsupported":
			result.Error = "systemctl is not available on the target"
		case state == "" && err != nil:
			result.Error = err.Error()
		default:
			result.OK = state == "active"
			if !result.OK {
				result.Error = fmt.Sprintf("unit %q is %s", sc.Unit, state)
			}
		}
		results = append(results, result)
	}
	return results
}

// runProcessChecks counts processes whose command line matches Pattern using
// `pgrep -c -f`.
func runProcessChecks(ctx context.Context, r runner, checks []config.ProcessCheck) []ProcessCheckResult {
	results := make([]ProcessCheckResult, 0, len(checks))
	for _, pc := range checks {
		minCount := pc.MinCount
		if minCount <= 0 {
			minCount = 1
		}
		command := fmt.Sprintf(
			"if command -v pgrep >/dev/null 2>&1; then pgrep -c -f %s 2>/dev/null; true; elif command -v ps >/dev/null 2>&1 && command -v grep >/dev/null 2>&1; then ps ax -o command= 2>/dev/null | grep -E -c -e %s || true; else printf unsupported; fi",
			shellSingleQuote(pc.Pattern), shellSingleQuote(pc.Pattern),
		)
		out, err := runToolProbe(ctx, r, pc.Timeout, command)
		trimmed := strings.TrimSpace(out)
		result := ProcessCheckResult{Name: pc.Name, Pattern: pc.Pattern, MinCount: minCount}
		if trimmed == "unsupported" {
			result.Error = "pgrep is not available on the target"
			results = append(results, result)
			continue
		}
		count, parseErr := strconv.Atoi(trimmed)
		if parseErr != nil {
			if err != nil {
				result.Error = err.Error()
			} else {
				result.Error = fmt.Sprintf("could not parse pgrep output %q", trimmed)
			}
			results = append(results, result)
			continue
		}
		result.Count = count
		result.OK = count >= minCount
		if !result.OK {
			result.Error = fmt.Sprintf("%d process(es) matched %q, want >= %d", count, pc.Pattern, minCount)
		}
		results = append(results, result)
	}
	return results
}

// runListeningChecks confirms a local socket is in LISTEN state on the
// target using `ss`. Unlike a target-side TCP dial, this reflects the bind
// state of the service itself, independent of firewalling between hosts.
func runListeningChecks(ctx context.Context, r runner, checks []config.ListeningCheck) []ListeningResult {
	results := make([]ListeningResult, 0, len(checks))
	for _, lc := range checks {
		protocol := lc.Protocol
		if protocol == "" {
			protocol = "tcp"
		}
		flag := "t"
		if protocol == "udp" {
			flag = "u"
		}
		command := fmt.Sprintf(
			"if command -v ss >/dev/null 2>&1; then ss -H -l%sn 2>/dev/null | awk '{print $4}' | grep -qE ':%d$' && printf listening || printf not_listening; elif command -v netstat >/dev/null 2>&1 && command -v awk >/dev/null 2>&1; then netstat -an 2>/dev/null | awk -v proto=%s -v port=%d '$1 ~ (\"^\" proto) && $0 ~ (\"[:.]\" port \"([^0-9]|$)\") { if (proto == \"udp\" || $0 ~ /LISTEN/) found=1 } END { if (found) printf \"listening\"; else printf \"not_listening\" }'; else printf unsupported; fi",
			flag, lc.Port, shellSingleQuote(protocol), lc.Port,
		)
		out, _ := runToolProbe(ctx, r, lc.Timeout, command)
		state := strings.TrimSpace(out)
		result := ListeningResult{Name: lc.Name, Port: lc.Port, Protocol: protocol}
		switch state {
		case "listening":
			result.OK = true
		case "not_listening":
			result.Error = fmt.Sprintf("no %s listener on port %d", protocol, lc.Port)
		default:
			result.Error = "neither ss nor netstat is available on the target"
		}
		results = append(results, result)
	}
	return results
}

// runJournalChecks counts recent systemd journal entries at or above
// Priority using `journalctl`. The since-timestamp is computed locally
// (rather than passed as a journalctl-relative expression) so behavior does
// not depend on the target's journalctl version.
func runJournalChecks(ctx context.Context, r runner, checks []config.JournalCheck) []JournalResult {
	results := make([]JournalResult, 0, len(checks))
	for _, jc := range checks {
		priority := jc.Priority
		if priority == "" {
			priority = "err"
		}
		sinceMinutes := jc.SinceMinutes
		if sinceMinutes <= 0 {
			sinceMinutes = 10
		}
		since := time.Now().Add(-time.Duration(sinceMinutes) * time.Minute).Format("2006-01-02 15:04:05")
		unitFlag := ""
		if strings.TrimSpace(jc.Unit) != "" {
			unitFlag = "-u " + shellSingleQuote(jc.Unit) + " "
		}
		command := fmt.Sprintf(
			"if command -v journalctl >/dev/null 2>&1; then journalctl -q --no-pager -p %s %s--since %s 2>/dev/null | wc -l; else printf unsupported; fi",
			shellSingleQuote(priority), unitFlag, shellSingleQuote(since),
		)
		out, err := runToolProbe(ctx, r, jc.Timeout, command)
		trimmed := strings.TrimSpace(out)
		result := JournalResult{Name: jc.Name, Unit: jc.Unit, Priority: priority, MaxCount: jc.MaxCount}
		if trimmed == "unsupported" {
			result.Error = "journalctl is not available on the target"
			results = append(results, result)
			continue
		}
		count, parseErr := strconv.Atoi(trimmed)
		if parseErr != nil {
			if err != nil {
				result.Error = err.Error()
			} else {
				result.Error = fmt.Sprintf("could not parse journalctl output %q", trimmed)
			}
			results = append(results, result)
			continue
		}
		result.Count = count
		result.OK = count <= jc.MaxCount
		if !result.OK {
			result.Error = fmt.Sprintf("%d entries at priority %s or higher in the last %d minutes, want <= %d", count, priority, sinceMinutes, jc.MaxCount)
		}
		results = append(results, result)
	}
	return results
}

// runFileChecks reads metadata with the shell test builtin and stat. It uses
// both GNU and BSD stat syntax so the same check works on Linux and BSD/macOS.
func runFileChecks(ctx context.Context, r runner, checks []config.FileCheck) []FileCheckResult {
	results := make([]FileCheckResult, 0, len(checks))
	for _, fc := range checks {
		path := shellSingleQuote(fc.Path)
		command := fmt.Sprintf("if [ ! -e %s ]; then printf missing; elif [ ! -r %s ]; then printf unreadable; elif ! command -v stat >/dev/null 2>&1; then printf unsupported; else if data=$(stat -c '%%s %%Y' %s 2>/dev/null); then printf '%%s' \"$data\"; elif data=$(stat -f '%%z %%m' %s 2>/dev/null); then printf '%%s' \"$data\"; else printf unreadable; fi; fi", path, path, path, path)
		out, err := runToolProbe(ctx, r, fc.Timeout, command)
		result := FileCheckResult{Name: fc.Name, Path: fc.Path}
		fields := strings.Fields(out)
		if len(fields) != 2 {
			result.Error = toolProbeError(strings.TrimSpace(out), err, "stat")
			results = append(results, result)
			continue
		}
		size, sizeErr := strconv.ParseInt(fields[0], 10, 64)
		modified, modifiedErr := strconv.ParseInt(fields[1], 10, 64)
		if sizeErr != nil || modifiedErr != nil {
			result.Error = fmt.Sprintf("could not parse stat output %q", strings.TrimSpace(out))
			results = append(results, result)
			continue
		}
		result.SizeBytes = size
		result.AgeSeconds = time.Now().Unix() - modified
		if result.AgeSeconds < 0 {
			result.AgeSeconds = 0
		}
		var violations []string
		if fc.MaxAgeSeconds > 0 && result.AgeSeconds > int64(fc.MaxAgeSeconds) {
			violations = append(violations, fmt.Sprintf("age %ds exceeds %ds", result.AgeSeconds, fc.MaxAgeSeconds))
		}
		if fc.MinSizeBytes > 0 && size < fc.MinSizeBytes {
			violations = append(violations, fmt.Sprintf("size %d is below %d bytes", size, fc.MinSizeBytes))
		}
		if fc.MaxSizeBytes > 0 && size > fc.MaxSizeBytes {
			violations = append(violations, fmt.Sprintf("size %d exceeds %d bytes", size, fc.MaxSizeBytes))
		}
		result.OK = len(violations) == 0
		result.Error = strings.Join(violations, "; ")
		results = append(results, result)
	}
	return results
}

// runDirectoryChecks uses du for a directory's allocated size. When a maximum
// file count is configured, find stops after one entry beyond that limit.
func runDirectoryChecks(ctx context.Context, r runner, checks []config.DirectoryCheck) []DirectoryResult {
	results := make([]DirectoryResult, 0, len(checks))
	for _, dc := range checks {
		path := shellSingleQuote(dc.Path)
		usageCommand := fmt.Sprintf("if [ ! -d %s ]; then printf missing; elif [ ! -r %s ]; then printf unreadable; elif command -v du >/dev/null 2>&1 && command -v awk >/dev/null 2>&1; then du -sk %s 2>/dev/null | awk 'NR == 1 {printf \"%%.0f\", $1 * 1024}'; else printf unsupported; fi", path, path, path)
		out, err := runToolProbe(ctx, r, dc.Timeout, usageCommand)
		result := DirectoryResult{Name: dc.Name, Path: dc.Path, MaxFileCount: dc.MaxFileCount}
		used, parseErr := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
		if parseErr != nil {
			result.Error = toolProbeError(strings.TrimSpace(out), err, "du")
			results = append(results, result)
			continue
		}
		result.UsedBytes = used
		if dc.MaxFileCount > 0 {
			countCommand := fmt.Sprintf("if [ ! -d %s ]; then printf missing; elif [ ! -r %s ]; then printf unreadable; elif command -v find >/dev/null 2>&1 && command -v awk >/dev/null 2>&1; then find %s -type f -print 2>/dev/null | awk -v limit=%d 'NR > limit {print limit + 1; exceeded=1; exit} END {if (!exceeded) print NR}'; else printf unsupported; fi", path, path, path, dc.MaxFileCount)
			countOut, countErr := runToolProbe(ctx, r, dc.Timeout, countCommand)
			count, countParseErr := strconv.Atoi(strings.TrimSpace(countOut))
			if countParseErr != nil {
				result.Error = toolProbeError(strings.TrimSpace(countOut), countErr, "find")
				results = append(results, result)
				continue
			}
			result.FileCount = count
			result.FileCountCapped = count > dc.MaxFileCount
		}
		var violations []string
		if dc.MaxUsageBytes > 0 && result.UsedBytes > dc.MaxUsageBytes {
			violations = append(violations, fmt.Sprintf("usage %d exceeds %d bytes", result.UsedBytes, dc.MaxUsageBytes))
		}
		if dc.MaxFileCount > 0 && result.FileCount > dc.MaxFileCount {
			violations = append(violations, fmt.Sprintf("file count exceeds %d", dc.MaxFileCount))
		}
		result.OK = len(violations) == 0
		result.Error = strings.Join(violations, "; ")
		results = append(results, result)
	}
	return results
}

// runLogChecks counts only matches in a bounded log tail; raw log data is
// never returned to the monitor or persisted in its result.
func runLogChecks(ctx context.Context, r runner, checks []config.LogCheck) []LogCheckResult {
	results := make([]LogCheckResult, 0, len(checks))
	for _, lc := range checks {
		path := shellSingleQuote(lc.Path)
		command := fmt.Sprintf("if [ ! -f %s ]; then printf missing; elif [ ! -r %s ]; then printf unreadable; elif command -v tail >/dev/null 2>&1 && command -v grep >/dev/null 2>&1; then tail -n %d %s 2>/dev/null | grep -E -c -e %s || true; else printf unsupported; fi", path, path, lc.Lines, path, shellSingleQuote(lc.Pattern))
		out, err := runToolProbe(ctx, r, lc.Timeout, command)
		result := LogCheckResult{Name: lc.Name, Path: lc.Path, Pattern: lc.Pattern, Lines: lc.Lines, MaxCount: lc.MaxCount}
		count, parseErr := strconv.Atoi(strings.TrimSpace(out))
		if parseErr != nil {
			result.Error = toolProbeError(strings.TrimSpace(out), err, "tail/grep")
			results = append(results, result)
			continue
		}
		result.Count = count
		result.OK = count <= lc.MaxCount
		if !result.OK {
			result.Error = fmt.Sprintf("%d matching line(s), want <= %d", count, lc.MaxCount)
		}
		results = append(results, result)
	}
	return results
}

func toolProbeError(output string, err error, tool string) string {
	switch output {
	case "missing":
		return "path does not exist on the target"
	case "unreadable":
		return "path or metadata is not readable by the SSH user"
	case "unsupported":
		return tool + " is not available on the target"
	}
	if err != nil {
		return err.Error()
	}
	if output == "" {
		return tool + " produced no usable output"
	}
	return fmt.Sprintf("could not parse %s output %q", tool, output)
}
