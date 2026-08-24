package monitor

import (
	"context"
	"strings"
)

// standardToolNames deliberately mixes POSIX/core utilities with optional
// operational tools. WatchSSH treats every entry as optional; this inventory
// is observational and never installs packages or changes a target.
var standardToolNames = []string{
	"awk", "basename", "cat", "cut", "date", "df", "dirname", "du", "find", "grep",
	"head", "hostname", "id", "printf", "ps", "sed", "sort", "stat", "tail", "test",
	"timeout", "tr", "uname", "uptime", "wc", "who", "xargs",
	"curl", "docker", "free", "ip", "iostat", "journalctl", "lsof", "mount", "nc", "netstat",
	"openssl", "pgrep", "pidof", "ping", "service", "ss", "systemctl", "vmstat",
}

// discoverStandardTools uses the POSIX shell builtin command rather than
// `which`, which is not guaranteed to be installed. Failure returns an empty
// result because tool inventory must never fail the main collection.
func discoverStandardTools(ctx context.Context, r runner) map[string]bool {
	const command = "for t in awk basename cat cut date df dirname du find grep head hostname id printf ps sed sort stat tail test timeout tr uname uptime wc who xargs curl docker free ip iostat journalctl lsof mount nc netstat openssl pgrep pidof ping service ss systemctl vmstat; do command -v \"$t\" >/dev/null 2>&1 && printf '%s\\n' \"$t\"; done"
	out, err := r.Run(ctx, command)
	if err != nil {
		return nil
	}
	available := make(map[string]bool, len(standardToolNames))
	for _, line := range strings.Fields(out) {
		for _, tool := range standardToolNames {
			if line == tool {
				available[tool] = true
				break
			}
		}
	}
	return available
}
