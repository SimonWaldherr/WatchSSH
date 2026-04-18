package monitor

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
)

// AlertManager evaluates alert rules against collected metrics and tracks
// cooldowns so repeated alerts are not sent too frequently.
type AlertManager struct {
	mu        sync.Mutex
	lastFired map[string]time.Time // key: ruleName + "|" + serverName
}

// NewAlertManager returns a new AlertManager.
func NewAlertManager() *AlertManager {
	return &AlertManager{lastFired: make(map[string]time.Time)}
}

// Evaluate checks all configured rules against the given metrics snapshot and
// returns newly-triggered Firings (respecting the configured cooldown).
func (am *AlertManager) Evaluate(metrics []ServerMetrics, cfg *config.Config) []Firing {
	if len(cfg.Alerts.Rules) == 0 {
		return nil
	}
	cooldown := time.Duration(cfg.Alerts.Cooldown) * time.Second
	now := time.Now()

	var firings []Firing
	for _, rule := range cfg.Alerts.Rules {
		for _, srv := range metrics {
			if !ruleAppliesToServer(rule, srv.ServerName) {
				continue
			}
			if srv.Error != "" {
				continue // connection errors are surfaced elsewhere
			}
			value, triggered := evaluateRule(rule, srv)
			if !triggered {
				continue
			}
			key := rule.Name + "|" + srv.ServerName
			am.mu.Lock()
			last, seen := am.lastFired[key]
			fire := !seen || now.Sub(last) >= cooldown
			if fire {
				am.lastFired[key] = now
			}
			am.mu.Unlock()
			if fire {
				firings = append(firings, Firing{
					RuleName: rule.Name,
					Metric:   rule.Metric,
					Server:   srv.ServerName,
					Value:    value,
					Message:  formatAlertMessage(rule, srv.ServerName, value),
					FiredAt:  now,
				})
			}
		}
	}
	return firings
}

// SendAlertEmail delivers email notifications for the given firings.
// It is a no-op when there are no firings or the email config is incomplete.
func SendAlertEmail(cfg config.EmailConfig, firings []Firing) error {
	if len(firings) == 0 || cfg.SMTPHost == "" || len(cfg.To) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("WatchSSH Alert Notification\n")
	sb.WriteString(strings.Repeat("─", 40) + "\n\n")
	for _, f := range firings {
		sb.WriteString(f.Message + "\n")
		sb.WriteString(fmt.Sprintf("  Fired at: %s\n\n", f.FiredAt.Format(time.RFC3339)))
	}

	subject := fmt.Sprintf("WatchSSH: %d alert(s) triggered", len(firings))
	return sendSMTPAlert(cfg, subject, sb.String())
}

// RunAlertAction executes a guarded local command when alerts fire.
// The command receives a JSON array of firings on stdin.
func RunAlertAction(cfg config.AlertActionConfig, firings []Firing) error {
	if len(firings) == 0 || strings.TrimSpace(cfg.Command) == "" {
		return nil
	}
	exe, args, err := parseActionCommand(cfg.Command)
	if err != nil {
		return err
	}
	if !actionExecutableAllowed(exe, cfg.AllowedExecutables) {
		return fmt.Errorf("alert action executable %q is not allowlisted", exe)
	}

	payload, err := json.Marshal(firings)
	if err != nil {
		return fmt.Errorf("marshal alert firings: %w", err)
	}

	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Env = append(os.Environ(),
		"WATCHSSH_ALERT_COUNT="+strconv.Itoa(len(firings)),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("alert action timed out after %s", timeout)
		}
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("alert action failed: %w: %s", err, msg)
		}
		return fmt.Errorf("alert action failed: %w", err)
	}
	return nil
}

// ── Rule evaluation ───────────────────────────────────────────────────────────

func evaluateRule(rule config.AlertRule, srv ServerMetrics) (float64, bool) {
	switch rule.Metric {
	case "cpu_usage":
		if srv.CPU == nil {
			return 0, false
		}
		return srv.CPU.UsagePercent, cmp(srv.CPU.UsagePercent, rule.Operator, rule.Threshold)
	case "mem_usage":
		if srv.Memory == nil {
			return 0, false
		}
		return srv.Memory.UsagePercent, cmp(srv.Memory.UsagePercent, rule.Operator, rule.Threshold)
	case "swap_usage":
		if srv.Swap == nil {
			return 0, false
		}
		return srv.Swap.Percent, cmp(srv.Swap.Percent, rule.Operator, rule.Threshold)
	case "load1":
		if srv.Load == nil {
			return 0, false
		}
		return srv.Load.Load1, cmp(srv.Load.Load1, rule.Operator, rule.Threshold)
	case "load5":
		if srv.Load == nil {
			return 0, false
		}
		return srv.Load.Load5, cmp(srv.Load.Load5, rule.Operator, rule.Threshold)
	case "load15":
		if srv.Load == nil {
			return 0, false
		}
		return srv.Load.Load15, cmp(srv.Load.Load15, rule.Operator, rule.Threshold)
	case "disk_usage":
		for _, d := range srv.Disks {
			if rule.MountPoint != "" && d.MountPoint != rule.MountPoint {
				continue
			}
			if cmp(d.UsagePercent, rule.Operator, rule.Threshold) {
				return d.UsagePercent, true
			}
		}
	case "ping_latency":
		if srv.Connectivity.PingEnabled {
			return srv.Connectivity.PingLatency, cmp(srv.Connectivity.PingLatency, rule.Operator, rule.Threshold)
		}
	case "ping_failed":
		if srv.Connectivity.PingEnabled && !srv.Connectivity.PingOK {
			return 1, true
		}
	case "port_closed":
		for _, p := range srv.Connectivity.Ports {
			if rule.Port != 0 && p.Port != rule.Port {
				continue
			}
			if !p.Open {
				return float64(p.Port), true
			}
		}
	case "http_failed":
		for _, h := range srv.Connectivity.HTTP {
			if !h.OK {
				return float64(h.StatusCode), true
			}
		}
	case "cert_expires_days":
		for _, h := range srv.Connectivity.HTTP {
			if h.CertExpiresDays == nil {
				continue
			}
			if cmp(*h.CertExpiresDays, rule.Operator, rule.Threshold) {
				return *h.CertExpiresDays, true
			}
		}
	case "custom_failed":
		for _, c := range srv.CustomChecks {
			if !c.OK {
				return 1, true
			}
		}
	}
	return 0, false
}

func cmp(value float64, operator string, threshold float64) bool {
	switch operator {
	case ">":
		return value > threshold
	case ">=":
		return value >= threshold
	case "<":
		return value < threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	case "!=":
		return value != threshold
	}
	return false
}

func ruleAppliesToServer(rule config.AlertRule, name string) bool {
	if len(rule.Servers) == 0 {
		return true
	}
	for _, s := range rule.Servers {
		if s == name {
			return true
		}
	}
	return false
}

func formatAlertMessage(rule config.AlertRule, server string, value float64) string {
	return fmt.Sprintf("[%s] Server %q — %s: %.2f %s %.2f",
		rule.Name, server, rule.Metric, value, rule.Operator, rule.Threshold)
}

// ── SMTP email sending ────────────────────────────────────────────────────────

func sendSMTPAlert(cfg config.EmailConfig, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)
	msg := buildEmailMsg(cfg.From, cfg.To, subject, body)
	switch strings.ToLower(cfg.TLSMode) {
	case "tls":
		return smtpTLSDirect(cfg, addr, msg)
	case "none":
		return smtpPlain(cfg, addr, msg)
	default: // "starttls"
		return smtpSTARTTLS(cfg, addr, msg)
	}
}

func parseActionCommand(command string) (string, []string, error) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", nil, fmt.Errorf("alert action command is empty")
	}
	return parts[0], parts[1:], nil
}

func actionExecutableAllowed(executable string, allowed []string) bool {
	exeBase := filepath.Base(executable)
	for _, entry := range allowed {
		v := strings.TrimSpace(entry)
		if v == "" {
			continue
		}
		if v == executable || filepath.Base(v) == exeBase {
			return true
		}
	}
	return false
}

func buildEmailMsg(from string, to []string, subject, body string) []byte {
	var sb strings.Builder
	sb.WriteString("From: " + from + "\r\n")
	sb.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	sb.WriteString("Subject: " + subject + "\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(body)
	return []byte(sb.String())
}

func smtpAuthMethod(cfg config.EmailConfig) smtp.Auth {
	if cfg.Username == "" {
		return nil
	}
	return smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.SMTPHost)
}

func smtpPlain(cfg config.EmailConfig, addr string, msg []byte) error {
	return smtp.SendMail(addr, smtpAuthMethod(cfg), cfg.From, cfg.To, msg)
}

func smtpSTARTTLS(cfg config.EmailConfig, addr string, msg []byte) error {
	c, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("SMTP dial: %w", err)
	}
	defer c.Close()

	if ok, _ := c.Extension("STARTTLS"); ok {
		// ServerName is set so the standard TLS certificate chain is verified.
		tlsCfg := &tls.Config{ServerName: cfg.SMTPHost} //nolint:gosec // InsecureSkipVerify is NOT set; gosec flags any tls.Config literal
		if err := c.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("STARTTLS: %w", err)
		}
	}
	if auth := smtpAuthMethod(cfg); auth != nil {
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("SMTP auth: %w", err)
		}
	}
	return smtpSendMsg(c, cfg.From, cfg.To, msg)
}

func smtpTLSDirect(cfg config.EmailConfig, addr string, msg []byte) error {
	// ServerName is set so the standard TLS certificate chain is verified.
	tlsCfg := &tls.Config{ServerName: cfg.SMTPHost} //nolint:gosec // InsecureSkipVerify is NOT set; gosec flags any tls.Config literal
	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		return fmt.Errorf("TLS dial: %w", err)
	}
	host, _, _ := net.SplitHostPort(addr)
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("SMTP client: %w", err)
	}
	defer c.Close()

	if auth := smtpAuthMethod(cfg); auth != nil {
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("SMTP auth: %w", err)
		}
	}
	return smtpSendMsg(c, cfg.From, cfg.To, msg)
}

func smtpSendMsg(c *smtp.Client, from string, to []string, msg []byte) error {
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}
	for _, addr := range to {
		if err := c.Rcpt(addr); err != nil {
			return fmt.Errorf("RCPT TO %s: %w", addr, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return fmt.Errorf("writing message body: %w", err)
	}
	return w.Close()
}
