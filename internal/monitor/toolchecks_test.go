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
