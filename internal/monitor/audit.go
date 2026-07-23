package monitor

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
	"github.com/SimonWaldherr/WatchSSH/internal/platform"
	sshclient "github.com/SimonWaldherr/WatchSSH/internal/ssh"
)

// AuditTarget runs the same bounded, read-only audit on demand. It is used by
// the web interface and never persists credentials or changes a remote host.
func AuditTarget(ctx context.Context, srv config.Server, cfg *config.Config, timeout time.Duration) (*AuditResult, error) {
	if srv.Local {
		r := &localRunner{}
		return collectAudit(ctx, r, string(platform.Detect(ctx, r)), srv.Audit.MaxEntries), nil
	}
	client, err := sshclient.New(ctx, srv, cfg, timeout)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	return collectAudit(ctx, client, string(platform.Detect(ctx, client)), srv.Audit.MaxEntries), nil
}

// collectAudit uses read-only OS interfaces and is deliberately Linux-focused.
// Other platforms report an explicit capability rather than guessing commands.
func collectAudit(ctx context.Context, r runner, platform string, limit int) *AuditResult {
	result := &AuditResult{Capabilities: map[string]string{}}
	if limit <= 0 {
		limit = 200
	}
	if platform != "Linux" {
		result.Capabilities["users"] = "unsupported"
		result.Capabilities["packages"] = "unsupported"
		return result
	}
	users, err := r.Run(ctx, "getent passwd 2>/dev/null || cat /etc/passwd 2>/dev/null")
	if err != nil {
		result.Capabilities["users"] = "unavailable"
	} else {
		result.Users, result.UsersCut = parseAuditUsers(users, limit)
		result.Capabilities["users"] = "ok"
	}
	tool, err := r.Run(ctx, "if command -v dpkg-query >/dev/null 2>&1; then printf dpkg-query; elif command -v rpm >/dev/null 2>&1; then printf rpm; elif command -v apk >/dev/null 2>&1; then printf apk; else exit 127; fi")
	tool = strings.TrimSpace(tool)
	if err != nil {
		result.Capabilities["packages"] = "unsupported"
	} else {
		var command string
		switch tool {
		case "dpkg-query":
			command = "dpkg-query -W -f='${binary:Package}\\n' 2>/dev/null"
		case "rpm":
			command = "rpm -qa 2>/dev/null"
		case "apk":
			command = "apk info 2>/dev/null"
		default:
			result.Capabilities["packages"] = "unsupported"
			return result
		}
		packages, packageErr := r.Run(ctx, command)
		if packageErr != nil {
			result.Capabilities["packages"] = "unavailable"
			return result
		}
		result.Packages, result.PackagesCut = parseAuditEntries(packages, limit)
		result.PackageTool = tool
		result.Capabilities["packages"] = "ok"
	}
	return result
}

func parseAuditUsers(output string, limit int) ([]AuditUser, bool) {
	users := make([]AuditUser, 0)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 3 || strings.TrimSpace(fields[0]) == "" {
			continue
		}
		uid, err := strconv.Atoi(fields[2])
		if err != nil {
			continue
		}
		users = append(users, AuditUser{Name: fields[0], UID: uid})
	}
	sort.Slice(users, func(i, j int) bool { return users[i].Name < users[j].Name })
	if len(users) > limit {
		return users[:limit], true
	}
	return users, false
}

func parseAuditEntries(output string, limit int) ([]string, bool) {
	entries := make([]string, 0)
	for _, entry := range strings.Fields(output) {
		if entry != "" {
			entries = append(entries, entry)
		}
	}
	sort.Strings(entries)
	if len(entries) > limit {
		return entries[:limit], true
	}
	return entries, false
}
