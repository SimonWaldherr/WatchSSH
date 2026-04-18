package monitor

import (
	"context"
	"fmt"
	"os/exec"
)

// localRunner implements runner using os/exec so the monitoring machine can
// be observed without an SSH round-trip.
type localRunner struct{}

func (l *localRunner) Run(ctx context.Context, cmd string) (string, error) {
	out, err := exec.CommandContext(ctx, "sh", "-c", cmd).CombinedOutput()
	if err != nil {
		// Include the captured output in the error for easier debugging.
		return string(out), fmt.Errorf("local command %q: %w", abbrev(cmd, 40), err)
	}
	return string(out), nil
}

// abbrev truncates s to at most n bytes for use in error messages.
func abbrev(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
