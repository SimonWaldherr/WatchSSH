package monitor

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"text/template"
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

type webhookPayload struct {
	SchemaVersion string    `json:"schema_version"`
	Route         string    `json:"route"`
	FiredAt       time.Time `json:"fired_at"`
	Summary       string    `json:"summary"`
	Alerts        []Firing  `json:"alerts"`
}

// SendAlertRoutes delivers each firing to the first matching route. A route
// with Continue set also permits following routes to receive the same firing.
func SendAlertRoutes(routes []config.AlertRoute, firings []Firing) error {
	if len(routes) == 0 || len(firings) == 0 {
		return nil
	}
	handled := make([]bool, len(firings))
	var errs []error
	for _, route := range routes {
		matched := make([]Firing, 0, len(firings))
		matchedIndexes := make([]int, 0, len(firings))
		for i, firing := range firings {
			if handled[i] || !routeApplies(route, firing) {
				continue
			}
			matched = append(matched, firing)
			matchedIndexes = append(matchedIndexes, i)
		}
		if len(matched) == 0 {
			continue
		}
		if err := sendAlertRoute(route, matched); err != nil {
			errs = append(errs, fmt.Errorf("route %q: %w", route.Name, err))
		}
		if !route.Continue {
			for _, i := range matchedIndexes {
				handled[i] = true
			}
		}
	}
	return errors.Join(errs...)
}

func sendAlertRoute(route config.AlertRoute, firings []Firing) error {
	switch {
	case route.IRC != nil:
		return sendIRCAlert(*route.IRC, firings)
	case route.Syslog != nil:
		return sendSyslogAlert(*route.Syslog, firings)
	default:
		return sendWebhook(route, firings)
	}
}

func sendIRCAlert(cfg config.IRCConfig, firings []Firing) error {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10
	}
	dialer := net.Dialer{Timeout: time.Duration(timeout) * time.Second}
	var (
		conn net.Conn
		err  error
	)
	if cfg.TLS {
		host, _, splitErr := net.SplitHostPort(cfg.Address)
		if splitErr != nil {
			return fmt.Errorf("IRC TLS address must include host and port: %w", splitErr)
		}
		conn, err = tls.DialWithDialer(&dialer, "tcp", cfg.Address, &tls.Config{ServerName: host})
	} else {
		conn, err = dialer.Dial("tcp", cfg.Address)
	}
	if err != nil {
		return fmt.Errorf("connecting to IRC server: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(time.Duration(timeout) * time.Second)); err != nil {
		return fmt.Errorf("setting IRC deadline: %w", err)
	}

	writer := bufio.NewWriter(conn)
	if cfg.PasswordEnv != "" {
		password, ok := os.LookupEnv(cfg.PasswordEnv)
		if !ok || password == "" {
			return fmt.Errorf("IRC password environment variable %q is not set", cfg.PasswordEnv)
		}
		if err := writeIRCLine(writer, "PASS "+password); err != nil {
			return err
		}
	}
	if err := writeIRCLine(writer, "NICK "+cfg.Nick); err != nil {
		return err
	}
	if err := writeIRCLine(writer, "USER "+cfg.Username+" 0 * :"+cfg.RealName); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("sending IRC registration: %w", err)
	}

	reader := bufio.NewReader(conn)
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			return fmt.Errorf("waiting for IRC welcome: %w", readErr)
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "PING ") {
			if err := writeIRCLine(writer, "PONG "+strings.TrimPrefix(line, "PING ")); err != nil {
				return err
			}
			if err := writer.Flush(); err != nil {
				return fmt.Errorf("responding to IRC ping: %w", err)
			}
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "001" {
			break
		}
		if len(fields) >= 2 && (fields[1] == "433" || fields[1] == "ERROR") {
			return fmt.Errorf("IRC registration rejected: %s", line)
		}
	}

	if err := writeIRCLine(writer, "JOIN "+cfg.Channel); err != nil {
		return err
	}
	for _, firing := range firings {
		if err := writeIRCLine(writer, "PRIVMSG "+cfg.Channel+" :"+ircMessage(firing.Message)); err != nil {
			return err
		}
	}
	if err := writeIRCLine(writer, "QUIT :WatchSSH alert delivery"); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("sending IRC alert: %w", err)
	}
	return nil
}

func writeIRCLine(writer *bufio.Writer, line string) error {
	if strings.ContainsAny(line, "\r\n") {
		return fmt.Errorf("IRC command contains a line break")
	}
	if _, err := writer.WriteString(line + "\r\n"); err != nil {
		return fmt.Errorf("writing IRC command: %w", err)
	}
	return nil
}

func ircMessage(message string) string {
	message = strings.NewReplacer("\r", " ", "\n", " ").Replace(message)
	const maxBytes = 400 // leaves room for command, channel, and IRC framing.
	if len(message) > maxBytes {
		return message[:maxBytes-3] + "..."
	}
	return message
}

func sendSyslogAlert(cfg config.SyslogConfig, firings []Firing) error {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10
	}
	network := cfg.Network
	if network == "" {
		network = "udp"
	}
	conn, err := net.DialTimeout(network, cfg.Address, time.Duration(timeout)*time.Second)
	if err != nil {
		return fmt.Errorf("connecting to syslog receiver: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(time.Duration(timeout) * time.Second)); err != nil {
		return fmt.Errorf("setting syslog deadline: %w", err)
	}

	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "-"
	}
	appName := syslogToken(cfg.AppName)
	if appName == "" {
		appName = "watchssh"
	}
	for _, firing := range firings {
		firedAt := firing.FiredAt
		if firedAt.IsZero() {
			firedAt = time.Now()
		}
		record := fmt.Sprintf("<131>1 %s %s %s - WATCHSSH - %s\n",
			firedAt.UTC().Format(time.RFC3339), syslogToken(host), appName, syslogMessage(firing.Message))
		if _, err := io.WriteString(conn, record); err != nil {
			return fmt.Errorf("sending syslog record: %w", err)
		}
	}
	return nil
}

func syslogToken(value string) string {
	return strings.Map(func(r rune) rune {
		if r <= 32 || r == 127 {
			return '-'
		}
		return r
	}, value)
}

func syslogMessage(message string) string {
	return strings.TrimSpace(strings.NewReplacer("\r", " ", "\n", " ").Replace(message))
}

func routeApplies(route config.AlertRoute, firing Firing) bool {
	return routeFieldMatches(route.Rules, firing.RuleName) &&
		routeFieldMatches(route.Metrics, firing.Metric) &&
		routeFieldMatches(route.Servers, firing.Server)
}

func routeFieldMatches(filters []string, value string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, filter := range filters {
		if filter == value {
			return true
		}
	}
	return false
}

func sendWebhook(route config.AlertRoute, firings []Firing) error {
	url := route.Webhook.URL
	if route.Webhook.URLEnv != "" {
		url = os.Getenv(route.Webhook.URLEnv)
		if url == "" {
			return fmt.Errorf("webhook URL environment variable %q is not set", route.Webhook.URLEnv)
		}
	}
	payload := webhookPayload{
		SchemaVersion: "1",
		Route:         route.Name,
		FiredAt:       firings[0].FiredAt,
		Summary:       fmt.Sprintf("WatchSSH: %d alert(s) matched route %s", len(firings), route.Name),
		Alerts:        firings,
	}
	body, err := webhookBody(route.Webhook.BodyTemplate, payload)
	if err != nil {
		return err
	}
	timeout := route.Webhook.Timeout
	if timeout <= 0 {
		timeout = 10
	}
	method := route.Webhook.Method
	if method == "" {
		method = http.MethodPost
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "WatchSSH/2 alert-webhook")
	for header, value := range route.Webhook.Headers {
		req.Header.Set(header, value)
	}
	for header, env := range route.Webhook.HeaderEnv {
		value, ok := os.LookupEnv(env)
		if !ok {
			return fmt.Errorf("webhook header environment variable %q is not set", env)
		}
		req.Header.Set(header, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		response, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("received %s: %s", resp.Status, strings.TrimSpace(string(response)))
	}
	return nil
}

func webhookBody(bodyTemplate string, payload webhookPayload) ([]byte, error) {
	if strings.TrimSpace(bodyTemplate) == "" {
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encoding payload: %w", err)
		}
		return body, nil
	}
	jsonValue := func(value any) (string, error) {
		encoded, err := json.Marshal(value)
		return string(encoded), err
	}
	tpl, err := template.New("webhook").Funcs(template.FuncMap{"json": jsonValue}).Option("missingkey=error").Parse(bodyTemplate)
	if err != nil {
		return nil, fmt.Errorf("parsing body_template: %w", err)
	}
	var body bytes.Buffer
	if err := tpl.Execute(&body, payload); err != nil {
		return nil, fmt.Errorf("executing body_template: %w", err)
	}
	if !json.Valid(body.Bytes()) {
		return nil, fmt.Errorf("body_template did not produce valid JSON")
	}
	return body.Bytes(), nil
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
	case "mem_available_bytes":
		if srv.Memory == nil {
			return 0, false
		}
		value := float64(srv.Memory.AvailableBytes)
		return value, cmp(value, rule.Operator, rule.Threshold)
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
	case "disk_free_bytes":
		for _, d := range srv.Disks {
			if rule.MountPoint != "" && d.MountPoint != rule.MountPoint {
				continue
			}
			value := float64(d.FreeBytes)
			if cmp(value, rule.Operator, rule.Threshold) {
				return value, true
			}
		}
	case "disk_inode_usage":
		for _, d := range srv.Disks {
			if d.InodesTotal == 0 {
				continue
			}
			if rule.MountPoint != "" && d.MountPoint != rule.MountPoint {
				continue
			}
			if cmp(d.InodesUsagePercent, rule.Operator, rule.Threshold) {
				return d.InodesUsagePercent, true
			}
		}
	case "processes_running":
		if srv.Load == nil {
			return 0, false
		}
		value := float64(srv.Load.RunningProcesses)
		return value, cmp(value, rule.Operator, rule.Threshold)
	case "processes_total":
		if srv.Load == nil {
			return 0, false
		}
		value := float64(srv.Load.TotalProcesses)
		return value, cmp(value, rule.Operator, rule.Threshold)
	case "file_descriptor_usage":
		if srv.FileDescriptors == nil {
			return 0, false
		}
		return srv.FileDescriptors.UsagePercent, cmp(srv.FileDescriptors.UsagePercent, rule.Operator, rule.Threshold)
	case "network_errors":
		value := float64(totalNetworkErrors(srv.Network))
		return value, cmp(value, rule.Operator, rule.Threshold)
	case "network_drops":
		value := float64(totalNetworkDrops(srv.Network))
		return value, cmp(value, rule.Operator, rule.Threshold)
	case "ping_latency":
		if srv.Connectivity.PingEnabled {
			return srv.Connectivity.PingLatency, cmp(srv.Connectivity.PingLatency, rule.Operator, rule.Threshold)
		}
	case "ping_failed":
		if srv.Connectivity.PingEnabled && !srv.Connectivity.PingOK {
			return 1, true
		}
	case "ping_loss":
		if srv.Connectivity.PingEnabled {
			return srv.Connectivity.PingLoss, cmp(srv.Connectivity.PingLoss, rule.Operator, rule.Threshold)
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
	case "port_latency":
		for _, p := range srv.Connectivity.Ports {
			if rule.Port != 0 && p.Port != rule.Port {
				continue
			}
			if cmp(p.LatencyMs, rule.Operator, rule.Threshold) {
				return p.LatencyMs, true
			}
		}
	case "banner_failed":
		for _, b := range srv.Connectivity.Banner {
			if !b.OK {
				return 1, true
			}
		}
	case "banner_latency":
		for _, b := range srv.Connectivity.Banner {
			if cmp(b.LatencyMs, rule.Operator, rule.Threshold) {
				return b.LatencyMs, true
			}
		}
	case "http_failed":
		for _, h := range srv.Connectivity.HTTP {
			if rule.URL != "" && h.URL != rule.URL {
				continue
			}
			if !h.OK {
				return float64(h.StatusCode), true
			}
		}
	case "http_latency":
		for _, h := range srv.Connectivity.HTTP {
			if rule.URL != "" && h.URL != rule.URL {
				continue
			}
			if cmp(h.LatencyMs, rule.Operator, rule.Threshold) {
				return h.LatencyMs, true
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
	case "dns_failed":
		for _, d := range srv.Connectivity.DNS {
			if !d.OK {
				return 1, true
			}
		}
	case "dns_latency":
		for _, d := range srv.Connectivity.DNS {
			if cmp(d.LatencyMs, rule.Operator, rule.Threshold) {
				return d.LatencyMs, true
			}
		}
	case "traceroute_failed":
		for _, t := range srv.Connectivity.Traceroute {
			if !t.OK {
				return 1, true
			}
		}
	case "traceroute_hops":
		for _, t := range srv.Connectivity.Traceroute {
			value := float64(t.Hops)
			if cmp(value, rule.Operator, rule.Threshold) {
				return value, true
			}
		}
	case "tls_failed":
		for _, t := range srv.Connectivity.TLS {
			if !t.OK {
				return 1, true
			}
		}
	case "tls_cert_expires_days":
		for _, t := range srv.Connectivity.TLS {
			if t.CertExpiresDays == nil {
				continue
			}
			if cmp(*t.CertExpiresDays, rule.Operator, rule.Threshold) {
				return *t.CertExpiresDays, true
			}
		}
	case "tls_latency":
		for _, t := range srv.Connectivity.TLS {
			if cmp(t.LatencyMs, rule.Operator, rule.Threshold) {
				return t.LatencyMs, true
			}
		}
	case "ntp_failed":
		for _, n := range srv.Connectivity.NTP {
			if !n.OK {
				return 1, true
			}
		}
	case "ntp_latency":
		for _, n := range srv.Connectivity.NTP {
			if cmp(n.LatencyMs, rule.Operator, rule.Threshold) {
				return n.LatencyMs, true
			}
		}
	case "ntp_offset":
		for _, n := range srv.Connectivity.NTP {
			if cmp(n.OffsetMs, rule.Operator, rule.Threshold) {
				return n.OffsetMs, true
			}
		}
	case "board_temperature":
		if srv.Board == nil || srv.Board.TemperatureC == nil {
			return 0, false
		}
		return *srv.Board.TemperatureC, cmp(*srv.Board.TemperatureC, rule.Operator, rule.Threshold)
	case "board_under_voltage":
		if srv.Board != nil && srv.Board.UnderVoltageNow {
			return 1, true
		}
	case "board_throttled":
		if srv.Board != nil && srv.Board.ThrottledNow {
			return 1, true
		}
	case "board_wifi_rssi":
		if srv.Board == nil || srv.Board.WiFiRSSIDbm == nil {
			return 0, false
		}
		return *srv.Board.WiFiRSSIDbm, cmp(*srv.Board.WiFiRSSIDbm, rule.Operator, rule.Threshold)
	case "custom_failed":
		for _, c := range srv.CustomChecks {
			if !c.OK {
				return 1, true
			}
		}
	}
	return 0, false
}

func totalNetworkErrors(network []NetworkStats) int64 {
	var total int64
	for _, n := range network {
		total += n.ErrorsRecv + n.ErrorsSent
	}
	return total
}

func totalNetworkDrops(network []NetworkStats) int64 {
	var total int64
	for _, n := range network {
		total += n.DropsRecv + n.DropsSent
	}
	return total
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
	scope := ""
	if rule.URL != "" {
		scope = fmt.Sprintf(" (%s)", rule.URL)
	}
	return fmt.Sprintf("[%s] Server %q — %s%s: %.2f %s %.2f",
		rule.Name, server, rule.Metric, scope, value, rule.Operator, rule.Threshold)
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
