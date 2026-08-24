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

// runServiceChecks queries systemd unit state with `systemctl is-active`.
// It is read-only: WatchSSH never starts, stops, or restarts a unit.
func runServiceChecks(ctx context.Context, r runner, checks []config.ServiceCheck) []ServiceResult {
	results := make([]ServiceResult, 0, len(checks))
	for _, sc := range checks {
		command := fmt.Sprintf(
			"if command -v systemctl >/dev/null 2>&1; then systemctl is-active %s 2>&1; else printf unsupported; fi",
			shellSingleQuote(sc.Unit),
		)
		out, err := r.Run(ctx, command)
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
			"if command -v pgrep >/dev/null 2>&1; then pgrep -c -f %s 2>/dev/null; true; else printf unsupported; fi",
			shellSingleQuote(pc.Pattern),
		)
		out, err := r.Run(ctx, command)
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
			"if command -v ss >/dev/null 2>&1; then ss -H -l%sn 2>/dev/null | awk '{print $4}' | grep -qE ':%d$' && printf listening || printf not_listening; else printf unsupported; fi",
			flag, lc.Port,
		)
		out, _ := r.Run(ctx, command)
		state := strings.TrimSpace(out)
		result := ListeningResult{Name: lc.Name, Port: lc.Port, Protocol: protocol}
		switch state {
		case "listening":
			result.OK = true
		case "not_listening":
			result.Error = fmt.Sprintf("no %s listener on port %d", protocol, lc.Port)
		default:
			result.Error = "ss is not available on the target"
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
		out, err := r.Run(ctx, command)
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
