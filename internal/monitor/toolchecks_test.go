package monitor

import (
	"context"
	"strings"
	"testing"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
)

// stubRunner returns output based on the command it receives, so a single
// test can simulate several distinct target-side tool responses.
type stubRunner struct {
	fn func(cmd string) (string, error)
}

func (s stubRunner) Run(_ context.Context, cmd string) (string, error) {
	return s.fn(cmd)
}

func TestShellSingleQuote(t *testing.T) {
	cases := map[string]string{
		"nginx.service": `'nginx.service'`,
		"it's":          `'it'\''s'`,
		"":              `''`,
		"a b":           `'a b'`,
	}
	for in, want := range cases {
		if got := shellSingleQuote(in); got != want {
			t.Errorf("shellSingleQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRunServiceChecks(t *testing.T) {
	r := stubRunner{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "'nginx.service'"):
			return "active\n", nil
		case strings.Contains(cmd, "'broken.service'"):
			return "failed\n", nil
		case strings.Contains(cmd, "'missing-tool.service'"):
			return "unsupported", nil
		}
		return "", nil
	}}
	checks := []config.ServiceCheck{
		{Name: "web", Unit: "nginx.service"},
		{Name: "broken", Unit: "broken.service"},
		{Name: "no-tool", Unit: "missing-tool.service"},
	}
	results := runServiceChecks(context.Background(), r, checks)
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	if !results[0].OK || results[0].State != "active" {
		t.Errorf("active unit result = %+v", results[0])
	}
	if results[1].OK || results[1].State != "failed" || results[1].Error == "" {
		t.Errorf("failed unit result = %+v", results[1])
	}
	if results[2].OK || results[2].Error == "" {
		t.Errorf("unsupported result = %+v", results[2])
	}
}

func TestRunProcessChecks(t *testing.T) {
	r := stubRunner{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "'nginx: worker'"):
			return "3\n", nil
		case strings.Contains(cmd, "'nonexistent'"):
			return "0\n", nil
		case strings.Contains(cmd, "'no-pgrep'"):
			return "unsupported", nil
		}
		return "", nil
	}}
	checks := []config.ProcessCheck{
		{Name: "workers", Pattern: "nginx: worker", MinCount: 2},
		{Name: "gone", Pattern: "nonexistent", MinCount: 1},
		{Name: "no-tool", Pattern: "no-pgrep", MinCount: 1},
	}
	results := runProcessChecks(context.Background(), r, checks)
	if !results[0].OK || results[0].Count != 3 {
		t.Errorf("workers result = %+v", results[0])
	}
	if results[1].OK || results[1].Count != 0 {
		t.Errorf("gone result = %+v", results[1])
	}
	if results[2].OK || results[2].Error == "" {
		t.Errorf("unsupported result = %+v", results[2])
	}
}

func TestRunProcessChecksUsesPIDOfBeforePortableFallbacks(t *testing.T) {
	r := stubRunner{fn: func(cmd string) (string, error) {
		if !strings.Contains(cmd, "pidof 'apache2'") {
			t.Fatalf("pidof command = %q", cmd)
		}
		return "2\n", nil
	}}
	results := runProcessChecks(context.Background(), r, []config.ProcessCheck{{Name: "apache", PIDOf: "apache2", MinCount: 1}})
	if len(results) != 1 || !results[0].OK || results[0].Count != 2 || results[0].PIDOf != "apache2" {
		t.Fatalf("pidof result = %+v", results)
	}
}

func TestRunListeningChecks(t *testing.T) {
	r := stubRunner{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "-ltn") && strings.Contains(cmd, ":443$"):
			return "listening", nil
		case strings.Contains(cmd, "-ltn") && strings.Contains(cmd, ":9999$"):
			return "not_listening", nil
		case strings.Contains(cmd, "-lun"):
			return "unsupported", nil
		}
		return "", nil
	}}
	checks := []config.ListeningCheck{
		{Name: "https", Port: 443, Protocol: "tcp"},
		{Name: "closed", Port: 9999, Protocol: "tcp"},
		{Name: "udp-probe", Port: 53, Protocol: "udp"},
	}
	results := runListeningChecks(context.Background(), r, checks)
	if !results[0].OK {
		t.Errorf("https result = %+v", results[0])
	}
	if results[1].OK || results[1].Error == "" {
		t.Errorf("closed result = %+v", results[1])
	}
	if results[2].OK || results[2].Error == "" {
		t.Errorf("unsupported result = %+v", results[2])
	}
}

func TestRunJournalChecks(t *testing.T) {
	r := stubRunner{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "'sshd.service'"):
			return "0\n", nil
		case strings.Contains(cmd, "'app.service'"):
			return "7\n", nil
		case strings.Contains(cmd, "'no-journal'"):
			return "unsupported", nil
		}
		return "", nil
	}}
	checks := []config.JournalCheck{
		{Name: "sshd", Unit: "sshd.service", MaxCount: 0},
		{Name: "app", Unit: "app.service", MaxCount: 3},
		{Name: "no-tool", Unit: "no-journal", MaxCount: 0},
	}
	results := runJournalChecks(context.Background(), r, checks)
	if !results[0].OK || results[0].Count != 0 {
		t.Errorf("sshd result = %+v", results[0])
	}
	if results[1].OK || results[1].Count != 7 {
		t.Errorf("app result = %+v", results[1])
	}
	if results[2].OK || results[2].Error == "" {
		t.Errorf("unsupported result = %+v", results[2])
	}
}

func TestRunFileChecks(t *testing.T) {
	r := stubRunner{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "'/run/app.pid'"):
			return "12 0", nil
		case strings.Contains(cmd, "'/missing'"):
			return "missing", nil
		default:
			return "", nil
		}
	}}
	checks := []config.FileCheck{
		{Name: "pid", Path: "/run/app.pid", MinSizeBytes: 1},
		{Name: "missing", Path: "/missing"},
	}
	results := runFileChecks(context.Background(), r, checks)
	if !results[0].OK || results[0].SizeBytes != 12 {
		t.Errorf("file result = %+v", results[0])
	}
	if results[1].OK || results[1].Error == "" {
		t.Errorf("missing file result = %+v", results[1])
	}
}

func TestRunDirectoryChecksStopsFileCountAtLimit(t *testing.T) {
	r := stubRunner{fn: func(cmd string) (string, error) {
		if strings.Contains(cmd, "find") {
			return "11", nil
		}
		return "4096", nil
	}}
	results := runDirectoryChecks(context.Background(), r, []config.DirectoryCheck{{Name: "cache", Path: "/var/cache/app", MaxUsageBytes: 8192, MaxFileCount: 10}})
	if results[0].OK || !results[0].FileCountCapped || results[0].FileCount != 11 {
		t.Errorf("directory result = %+v", results[0])
	}
}

func TestRunLogChecksDoesNotExposeLogContent(t *testing.T) {
	r := stubRunner{fn: func(cmd string) (string, error) {
		if !strings.Contains(cmd, "grep -E -c") || !strings.Contains(cmd, "'/var/log/app.log'") {
			t.Fatalf("unexpected log command %q", cmd)
		}
		return "3\n", nil
	}}
	results := runLogChecks(context.Background(), r, []config.LogCheck{{Name: "errors", Path: "/var/log/app.log", Pattern: "ERROR", Lines: 100, MaxCount: 2}})
	if results[0].OK || results[0].Count != 3 || results[0].Error == "" {
		t.Errorf("log result = %+v", results[0])
	}
}

func TestRunCommandChecks(t *testing.T) {
	r := stubRunner{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "command -v 'docker'"):
			return "/usr/local/bin/docker\n", nil
		case strings.Contains(cmd, "command -v 'missing-tool'"):
			return "missing", nil
		default:
			t.Fatalf("unexpected command probe command %q", cmd)
			return "", nil
		}
	}}
	results := runCommandChecks(context.Background(), r, []config.CommandCheck{
		{Name: "docker", Command: "docker", Timeout: 5},
		{Name: "missing", Command: "missing-tool", Timeout: 5},
	})
	if len(results) != 2 || !results[0].OK || results[0].ResolvedPath != "/usr/local/bin/docker" {
		t.Fatalf("available command result = %#v", results)
	}
	if results[1].OK || results[1].Error == "" || results[1].ResolvedPath != "" {
		t.Fatalf("missing command result = %#v", results[1])
	}
}

func TestRunHashChecksKeepsFileContentOnTarget(t *testing.T) {
	goodDigest := strings.Repeat("a", 64)
	badDigest := strings.Repeat("b", 64)
	r := stubRunner{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "'/etc/app.conf'"):
			if !strings.Contains(cmd, "sha256sum") || !strings.Contains(cmd, "shasum -a 256") || !strings.Contains(cmd, "openssl dgst -sha256") {
				t.Fatalf("hash fallback command = %q", cmd)
			}
			return goodDigest + "\n", nil
		case strings.Contains(cmd, "'/etc/changed.conf'"):
			return badDigest, nil
		case strings.Contains(cmd, "'/etc/missing.conf'"):
			return "missing", nil
		default:
			t.Fatalf("unexpected hash command %q", cmd)
			return "", nil
		}
	}}
	results := runHashChecks(context.Background(), r, []config.HashCheck{
		{Name: "good", Path: "/etc/app.conf", Algorithm: "sha256", ExpectedDigest: goodDigest, Timeout: 10},
		{Name: "changed", Path: "/etc/changed.conf", Algorithm: "sha256", ExpectedDigest: goodDigest, Timeout: 10},
		{Name: "missing", Path: "/etc/missing.conf", Algorithm: "sha256", ExpectedDigest: goodDigest, Timeout: 10},
	})
	if len(results) != 3 || !results[0].OK || results[0].ObservedDigest != goodDigest {
		t.Fatalf("matching hash result = %#v", results)
	}
	if results[1].OK || results[1].ObservedDigest != badDigest || results[1].Error == "" {
		t.Fatalf("changed hash result = %#v", results[1])
	}
	if results[2].OK || results[2].Error == "" || results[2].ObservedDigest != "" {
		t.Fatalf("missing hash result = %#v", results[2])
	}
}

func TestRunCertificateFileChecks(t *testing.T) {
	r := stubRunner{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "'/etc/ssl/live.pem'"):
			return "notAfter=Dec 31 23:59:59 2099 GMT\n", nil
		case strings.Contains(cmd, "'/etc/ssl/bad.pem'"):
			return "invalid", nil
		default:
			t.Fatalf("unexpected certificate command %q", cmd)
			return "", nil
		}
	}}
	results := runCertificateFileChecks(context.Background(), r, []config.CertificateFileCheck{
		{Name: "live", Path: "/etc/ssl/live.pem", WarnDays: 30, Timeout: 5},
		{Name: "bad", Path: "/etc/ssl/bad.pem", WarnDays: 30, Timeout: 5},
	})
	if len(results) != 2 || !results[0].OK || results[0].ExpiresAt == nil || results[0].ExpiresDays < 365 {
		t.Fatalf("valid certificate result = %#v", results)
	}
	if results[1].OK || results[1].Error == "" || results[1].ExpiresAt != nil {
		t.Fatalf("invalid certificate result = %#v", results[1])
	}
}

func TestRunUnixSocketAndUserChecks(t *testing.T) {
	r := stubRunner{fn: func(cmd string) (string, error) {
		switch {
		case strings.Contains(cmd, "'/run/app.sock'"):
			return "socket", nil
		case strings.Contains(cmd, "'/run/missing.sock'"):
			return "missing", nil
		case strings.Contains(cmd, "id -u 'www-data'"):
			return "33", nil
		case strings.Contains(cmd, "id -u 'missing-user'"):
			return "missing", nil
		default:
			t.Fatalf("unexpected Unix socket/user command %q", cmd)
			return "", nil
		}
	}}
	sockets := runUnixSocketChecks(context.Background(), r, []config.UnixSocketCheck{{Name: "app", Path: "/run/app.sock"}, {Name: "missing", Path: "/run/missing.sock"}})
	if len(sockets) != 2 || !sockets[0].OK || sockets[1].OK || sockets[1].Error == "" {
		t.Fatalf("socket results = %#v", sockets)
	}
	users := runUserChecks(context.Background(), r, []config.UserCheck{{Name: "web", User: "www-data"}, {Name: "missing", User: "missing-user"}})
	if len(users) != 2 || !users[0].OK || users[0].UID != 33 || users[1].OK || users[1].Error == "" {
		t.Fatalf("user results = %#v", users)
	}
}
