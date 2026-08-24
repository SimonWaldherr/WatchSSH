package monitor

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
	sshclient "github.com/SimonWaldherr/WatchSSH/internal/ssh"
)

// RemediationManager tracks automatic command attempts in memory. It prevents
// a persistent symptom from repeatedly restarting the same target.
type RemediationManager struct {
	mu       sync.Mutex
	last     map[string]time.Time
	attempts map[string][]time.Time
}

func NewRemediationManager() *RemediationManager {
	return &RemediationManager{
		last:     make(map[string]time.Time),
		attempts: make(map[string][]time.Time),
	}
}

func (m *Monitor) runRemediations(cfg *config.Config, firings []Firing) {
	if len(cfg.Alerts.Remediations) == 0 || len(firings) == 0 {
		return
	}
	servers := make(map[string]config.Server, len(cfg.Servers))
	for _, server := range cfg.Servers {
		servers[server.Name] = server
	}

	for firingIndex := range firings {
		firing := &firings[firingIndex]
		for _, remediation := range cfg.Alerts.Remediations {
			if !remediation.Enabled || (remediation.Mode != "" && remediation.Mode != "alert") || !remediationMatches(remediation, *firing) {
				continue
			}
			firing.Remediations = append(firing.Remediations, m.executeRemediation(cfg, servers, remediation, firing.Server)...)
		}
	}
}

func (m *Monitor) executeRemediation(cfg *config.Config, servers map[string]config.Server, remediation config.RemediationConfig, sourceServer string) []RemediationResult {
	targets := remediation.Targets
	if len(targets) == 0 {
		targets = []string{sourceServer}
	}
	results := make([]RemediationResult, 0, len(targets))
	for _, targetName := range targets {
		target, exists := servers[targetName]
		if !exists {
			results = append(results, RemediationResult{
				Name: remediation.Name, Target: targetName, StartedAt: time.Now(), Status: "failed",
				Error: "target server is no longer configured",
			})
			continue
		}
		allowed, status := m.remediationMgr.allow(remediation, target.Name, time.Now())
		if !allowed {
			results = append(results, RemediationResult{
				Name: remediation.Name, Target: target.Name, StartedAt: time.Now(), Status: status,
			})
			continue
		}
		results = append(results, runRemediation(cfg, target, remediation))
	}
	return results
}

func remediationMatches(remediation config.RemediationConfig, firing Firing) bool {
	return routeFieldMatches(remediation.Rules, firing.RuleName) &&
		routeFieldMatches(remediation.Metrics, firing.Metric) &&
		routeFieldMatches(remediation.Servers, firing.Server)
}

func (rm *RemediationManager) allow(remediation config.RemediationConfig, target string, now time.Time) (bool, string) {
	key := remediation.Name + "|" + target
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if last := rm.last[key]; !last.IsZero() && now.Sub(last) < time.Duration(remediation.Cooldown)*time.Second {
		return false, "skipped_cooldown"
	}
	windowStart := now.Add(-time.Duration(remediation.Window) * time.Second)
	attempts := rm.attempts[key][:0]
	for _, attempt := range rm.attempts[key] {
		if !attempt.Before(windowStart) {
			attempts = append(attempts, attempt)
		}
	}
	if len(attempts) >= remediation.MaxAttempts {
		rm.attempts[key] = attempts
		return false, "skipped_rate_limit"
	}
	rm.last[key] = now
	rm.attempts[key] = append(attempts, now)
	return true, ""
}

func runRemediation(cfg *config.Config, target config.Server, remediation config.RemediationConfig) RemediationResult {
	startedAt := time.Now()
	result := RemediationResult{Name: remediation.Name, Target: target.Name, StartedAt: startedAt}
	var commandRunner runner
	closeRunner := func() {}
	if target.Local {
		commandRunner = &localRunner{}
	} else {
		connectTimeout := time.Duration(remediation.Timeout) * time.Second
		connectCtx, cancelConnect := context.WithTimeout(context.Background(), connectTimeout)
		client, connectErr := sshclient.New(connectCtx, target, cfg, connectTimeout)
		cancelConnect()
		if connectErr != nil {
			result.DurationMs = durationMilliseconds(startedAt)
			result.Error = fmt.Sprintf("connecting over SSH: %v", connectErr)
			result.Status = "failed"
			return result
		}
		commandRunner = client
		closeRunner = func() { _ = client.Close() }
	}
	defer closeRunner()

	recoveryOutput, err := runRemediationCommand(commandRunner, remediation.Command, remediation.Timeout)
	result.Output = abbreviatedRemediationOutput(recoveryOutput)
	if err != nil {
		result.DurationMs = durationMilliseconds(startedAt)
		result.Error = remediationCommandError("recovery", err, remediation.Timeout)
		result.Status = "failed"
		return result
	}

	if strings.TrimSpace(remediation.VerifyCommand) != "" {
		if err := waitForRemediationVerification(remediation.VerifyDelay); err != nil {
			result.DurationMs = durationMilliseconds(startedAt)
			result.Error = err.Error()
			result.Status = "failed"
			return result
		}
		verificationOutput, verifyErr := runRemediationCommand(commandRunner, remediation.VerifyCommand, remediation.VerifyTimeout)
		result.Output = combinedRemediationOutput(recoveryOutput, verificationOutput)
		if verifyErr != nil {
			result.DurationMs = durationMilliseconds(startedAt)
			result.Error = remediationCommandError("verification", verifyErr, remediation.VerifyTimeout)
			result.Status = "failed"
			return result
		}
		result.Verified = true
	}

	result.DurationMs = durationMilliseconds(startedAt)
	result.Status = "succeeded"
	return result
}

func runRemediationCommand(r runner, command string, timeoutSeconds int) (string, error) {
	timeout := time.Duration(timeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	output, err := r.Run(ctx, command)
	if ctx.Err() == context.DeadlineExceeded {
		return output, context.DeadlineExceeded
	}
	return output, err
}

func waitForRemediationVerification(delaySeconds int) error {
	if delaySeconds <= 0 {
		return nil
	}
	timer := time.NewTimer(time.Duration(delaySeconds) * time.Second)
	defer timer.Stop()
	<-timer.C
	return nil
}

func remediationCommandError(step string, err error, timeoutSeconds int) string {
	if err == context.DeadlineExceeded {
		return fmt.Sprintf("%s timed out after %ds", step, timeoutSeconds)
	}
	return fmt.Sprintf("%s command failed: %v", step, err)
}

func combinedRemediationOutput(recoveryOutput, verificationOutput string) string {
	recoveryOutput = strings.TrimSpace(recoveryOutput)
	verificationOutput = strings.TrimSpace(verificationOutput)
	if verificationOutput == "" {
		return abbreviatedRemediationOutput(recoveryOutput)
	}
	if recoveryOutput == "" {
		return abbreviatedRemediationOutput("verification:\n" + verificationOutput)
	}
	return abbreviatedRemediationOutput("recovery:\n" + recoveryOutput + "\nverification:\n" + verificationOutput)
}

func abbreviatedRemediationOutput(output string) string {
	output = strings.TrimSpace(output)
	const limit = 4096
	if len(output) <= limit {
		return output
	}
	return output[:limit-3] + "..."
}
