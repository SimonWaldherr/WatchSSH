package monitor

import (
	"bufio"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
)

func makeMetrics(name string, cpu, mem float64) ServerMetrics {
	return ServerMetrics{
		ServerName: name,
		Host:       "192.0.2.1",
		Timestamp:  time.Now(),
		CPU:        &CPUStats{UsagePercent: cpu},
		Memory:     &MemoryStats{UsagePercent: mem},
	}
}

func TestAlertManager_Evaluate_TriggersOnThreshold(t *testing.T) {
	am := NewAlertManager()
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Rules: []config.AlertRule{
				{Name: "HighCPU", Metric: "cpu_usage", Operator: ">", Threshold: 80},
			},
			Cooldown: 0,
		},
	}

	metrics := []ServerMetrics{makeMetrics("srv1", 95, 50)}
	firings := am.Evaluate(metrics, cfg)
	if len(firings) != 1 {
		t.Fatalf("expected 1 firing, got %d", len(firings))
	}
	if firings[0].Server != "srv1" {
		t.Errorf("firing server = %q; want srv1", firings[0].Server)
	}
	if firings[0].Metric != "cpu_usage" {
		t.Errorf("firing metric = %q; want cpu_usage", firings[0].Metric)
	}
}

func TestAlertManager_Evaluate_NoTriggerBelowThreshold(t *testing.T) {
	am := NewAlertManager()
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Rules: []config.AlertRule{
				{Name: "HighCPU", Metric: "cpu_usage", Operator: ">", Threshold: 80},
			},
		},
	}
	metrics := []ServerMetrics{makeMetrics("srv1", 70, 50)}
	firings := am.Evaluate(metrics, cfg)
	if len(firings) != 0 {
		t.Errorf("expected 0 firings, got %d", len(firings))
	}
}

func TestAlertManager_Cooldown(t *testing.T) {
	am := NewAlertManager()
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Rules: []config.AlertRule{
				{Name: "HighCPU", Metric: "cpu_usage", Operator: ">", Threshold: 80},
			},
			Cooldown: 3600, // 1 hour
		},
	}
	metrics := []ServerMetrics{makeMetrics("srv1", 95, 50)}

	first := am.Evaluate(metrics, cfg)
	if len(first) != 1 {
		t.Fatalf("first evaluation: expected 1 firing, got %d", len(first))
	}
	second := am.Evaluate(metrics, cfg)
	if len(second) != 0 {
		t.Errorf("second evaluation within cooldown: expected 0 firings, got %d", len(second))
	}
}

func TestAlertManager_ServerFilter(t *testing.T) {
	am := NewAlertManager()
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Rules: []config.AlertRule{
				{Name: "HighCPU", Metric: "cpu_usage", Operator: ">", Threshold: 80, Servers: []string{"web-01"}},
			},
			Cooldown: 0,
		},
	}
	metrics := []ServerMetrics{
		makeMetrics("web-01", 95, 50), // should fire
		makeMetrics("db-01", 95, 50),  // should NOT fire (different server)
	}
	firings := am.Evaluate(metrics, cfg)
	if len(firings) != 1 {
		t.Fatalf("expected 1 firing, got %d", len(firings))
	}
	if firings[0].Server != "web-01" {
		t.Errorf("expected firing for web-01, got %q", firings[0].Server)
	}
}

func TestAlertManager_NilCPU(t *testing.T) {
	// Ensure alert rules don't fire when CPU metric is nil (e.g. first poll).
	am := NewAlertManager()
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Rules: []config.AlertRule{
				{Name: "HighCPU", Metric: "cpu_usage", Operator: ">", Threshold: 80},
			},
			Cooldown: 0,
		},
	}
	metrics := []ServerMetrics{{
		ServerName: "srv1",
		Timestamp:  time.Now(),
		CPU:        nil, // unavailable
	}}
	firings := am.Evaluate(metrics, cfg)
	if len(firings) != 0 {
		t.Errorf("expected 0 firings for nil CPU, got %d", len(firings))
	}
}

func TestAlertManager_NTPOffset(t *testing.T) {
	am := NewAlertManager()
	cfg := &config.Config{Alerts: config.AlertsConfig{Rules: []config.AlertRule{{
		Name: "ClockDrift", Metric: "ntp_offset", Operator: ">", Threshold: 50,
	}}, Cooldown: 0}}
	metrics := []ServerMetrics{{
		ServerName:   "srv1",
		Timestamp:    time.Now(),
		Connectivity: ConnectivityStats{NTP: []NTPResult{{Host: "time.example", OK: true, OffsetMs: 73}}},
	}}
	if firings := am.Evaluate(metrics, cfg); len(firings) != 1 {
		t.Fatalf("expected NTP offset alert, got %d", len(firings))
	}
}

func TestAlertManager_ConnectivityLatencyAndLoss(t *testing.T) {
	am := NewAlertManager()
	cfg := &config.Config{Alerts: config.AlertsConfig{Rules: []config.AlertRule{
		{Name: "Loss", Metric: "ping_loss", Operator: ">", Threshold: 5},
		{Name: "Banner", Metric: "banner_failed", Operator: "==", Threshold: 1},
		{Name: "TLS", Metric: "tls_latency", Operator: ">", Threshold: 100},
	}, Cooldown: 0}}
	metrics := []ServerMetrics{{
		ServerName: "app-01",
		Timestamp:  time.Now(),
		Connectivity: ConnectivityStats{
			PingEnabled: true,
			PingLoss:    12.5,
			Banner:      []BannerResult{{Host: "app-01", Port: 22, OK: false}},
			TLS:         []TLSResult{{Host: "app-01", Port: 443, OK: true, LatencyMs: 150}},
		},
	}}
	if firings := am.Evaluate(metrics, cfg); len(firings) != 3 {
		t.Fatalf("expected 3 connectivity firings, got %#v", firings)
	}
}

func TestAlertManager_HTTPURLFilter(t *testing.T) {
	metrics := []ServerMetrics{{
		ServerName: "web-01",
		Connectivity: ConnectivityStats{HTTP: []HTTPResult{
			{URL: "https://example.test/ready", OK: false, StatusCode: 503},
			{URL: "https://example.test/health", OK: true, StatusCode: 200, LatencyMs: 2500},
		}},
	}}
	cfg := &config.Config{Alerts: config.AlertsConfig{Cooldown: 0, Rules: []config.AlertRule{
		{Name: "health-slow", Metric: "http_latency", Operator: ">", Threshold: 2000, URL: "https://example.test/health"},
		{Name: "health-down", Metric: "http_failed", Operator: "==", Threshold: 1, URL: "https://example.test/health"},
	}}}
	firings := NewAlertManager().Evaluate(metrics, cfg)
	if len(firings) != 1 || firings[0].RuleName != "health-slow" || !strings.Contains(firings[0].Message, "https://example.test/health") {
		t.Fatalf("URL-filtered firings = %#v", firings)
	}
}

func TestAlertManager_DiskUsage(t *testing.T) {
	am := NewAlertManager()
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Rules: []config.AlertRule{
				{Name: "DiskFull", Metric: "disk_usage", Operator: ">", Threshold: 85, MountPoint: "/"},
			},
			Cooldown: 0,
		},
	}
	metrics := []ServerMetrics{{
		ServerName: "srv1",
		Timestamp:  time.Now(),
		Disks: []DiskStats{
			{MountPoint: "/", UsagePercent: 90},
			{MountPoint: "/data", UsagePercent: 60},
		},
	}}
	firings := am.Evaluate(metrics, cfg)
	if len(firings) != 1 {
		t.Fatalf("expected 1 disk firing, got %d", len(firings))
	}
}

func TestAlertManager_DiskInodeUsage(t *testing.T) {
	am := NewAlertManager()
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Rules: []config.AlertRule{
				{Name: "InodesFull", Metric: "disk_inode_usage", Operator: ">", Threshold: 80, MountPoint: "/"},
			},
			Cooldown: 0,
		},
	}
	metrics := []ServerMetrics{{
		ServerName: "srv1",
		Timestamp:  time.Now(),
		Disks: []DiskStats{
			{MountPoint: "/", InodesTotal: 1000, InodesUsagePercent: 91},
			{MountPoint: "/data", InodesTotal: 1000, InodesUsagePercent: 20},
		},
	}}
	firings := am.Evaluate(metrics, cfg)
	if len(firings) != 1 {
		t.Fatalf("expected 1 inode firing, got %d", len(firings))
	}
}

func TestAlertManager_FileDescriptorUsage(t *testing.T) {
	am := NewAlertManager()
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Rules: []config.AlertRule{
				{Name: "FileDescriptorsHigh", Metric: "file_descriptor_usage", Operator: ">", Threshold: 80},
			},
			Cooldown: 0,
		},
	}
	metrics := []ServerMetrics{{
		ServerName:      "srv1",
		Timestamp:       time.Now(),
		FileDescriptors: &FileDescriptorStats{UsagePercent: 90},
	}}
	firings := am.Evaluate(metrics, cfg)
	if len(firings) != 1 {
		t.Fatalf("expected 1 file descriptor firing, got %d", len(firings))
	}
}

func TestAlertManager_ProcessAndNetworkMetrics(t *testing.T) {
	am := NewAlertManager()
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Rules: []config.AlertRule{
				{Name: "TooManyProcesses", Metric: "processes_total", Operator: ">", Threshold: 200},
				{Name: "NetworkErrors", Metric: "network_errors", Operator: ">", Threshold: 0},
				{Name: "NetworkDrops", Metric: "network_drops", Operator: ">", Threshold: 0},
			},
			Cooldown: 0,
		},
	}
	metrics := []ServerMetrics{{
		ServerName: "srv1",
		Timestamp:  time.Now(),
		Load:       &LoadStats{RunningProcesses: 2, TotalProcesses: 250},
		Network: []NetworkStats{
			{Interface: "eth0", ErrorsRecv: 1, DropsSent: 2},
		},
	}}
	firings := am.Evaluate(metrics, cfg)
	if len(firings) != 3 {
		t.Fatalf("expected 3 firings, got %d", len(firings))
	}
}

func TestAlertManager_CertExpiresDays(t *testing.T) {
	am := NewAlertManager()
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Rules: []config.AlertRule{
				{Name: "CertExpiringSoon", Metric: "cert_expires_days", Operator: "<", Threshold: 30},
			},
			Cooldown: 0,
		},
	}
	days := 10.0
	metrics := []ServerMetrics{{
		ServerName: "srv1",
		Timestamp:  time.Now(),
		Connectivity: ConnectivityStats{
			HTTP: []HTTPResult{{URL: "https://example.com/health", OK: true, CertExpiresDays: &days}},
		},
	}}
	firings := am.Evaluate(metrics, cfg)
	if len(firings) != 1 {
		t.Fatalf("expected 1 cert firing, got %d", len(firings))
	}
	if firings[0].Metric != "cert_expires_days" {
		t.Fatalf("metric = %q, want cert_expires_days", firings[0].Metric)
	}
}

func TestAlertManager_NetworkProbeMetrics(t *testing.T) {
	tlsDays := 5.0
	metrics := []ServerMetrics{{
		ServerName: "web-01",
		Connectivity: ConnectivityStats{
			DNS: []DNSResult{{
				Host:      "example.com",
				Type:      "A",
				OK:        false,
				LatencyMs: 120,
			}},
			Traceroute: []TracerouteResult{{
				Host: "example.com",
				OK:   true,
				Hops: 24,
			}},
			TLS: []TLSResult{{
				Host:            "example.com",
				Port:            443,
				OK:              true,
				CertExpiresDays: &tlsDays,
			}},
		},
	}}
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Cooldown: 1,
			Rules: []config.AlertRule{
				{Name: "DNSFailed", Metric: "dns_failed", Operator: "==", Threshold: 1},
				{Name: "TraceTooLong", Metric: "traceroute_hops", Operator: ">", Threshold: 20},
				{Name: "TLSExpires", Metric: "tls_cert_expires_days", Operator: "<", Threshold: 10},
			},
		},
	}

	firings := NewAlertManager().Evaluate(metrics, cfg)
	if len(firings) != 3 {
		t.Fatalf("firings len = %d, want 3: %#v", len(firings), firings)
	}
}

func TestAlertManager_BoardMetrics(t *testing.T) {
	temp := 82.5
	rssi := -78.0
	metrics := []ServerMetrics{{
		ServerName: "pi-01",
		Board: &BoardInfo{
			TemperatureC:    &temp,
			UnderVoltageNow: true,
			ThrottledNow:    true,
			WiFiRSSIDbm:     &rssi,
		},
	}}
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Cooldown: 0,
			Rules: []config.AlertRule{
				{Name: "PiHot", Metric: "board_temperature", Operator: ">", Threshold: 80},
				{Name: "PiUndervoltage", Metric: "board_under_voltage", Operator: "==", Threshold: 1},
				{Name: "PiThrottled", Metric: "board_throttled", Operator: "==", Threshold: 1},
				{Name: "PiWiFiWeak", Metric: "board_wifi_rssi", Operator: "<", Threshold: -75},
			},
		},
	}

	firings := NewAlertManager().Evaluate(metrics, cfg)
	if len(firings) != 4 {
		t.Fatalf("firings len = %d, want 4: %#v", len(firings), firings)
	}
}

func TestCmp(t *testing.T) {
	tests := []struct {
		value    float64
		op       string
		thresh   float64
		expected bool
	}{
		{90, ">", 80, true},
		{70, ">", 80, false},
		{80, ">=", 80, true},
		{79, ">=", 80, false},
		{70, "<", 80, true},
		{90, "<", 80, false},
		{80, "<=", 80, true},
		{80, "==", 80, true},
		{81, "==", 80, false},
		{81, "!=", 80, true},
	}
	for _, tt := range tests {
		got := cmp(tt.value, tt.op, tt.thresh)
		if got != tt.expected {
			t.Errorf("cmp(%.0f, %q, %.0f) = %v; want %v", tt.value, tt.op, tt.thresh, got, tt.expected)
		}
	}
}

func TestActionExecutableAllowed(t *testing.T) {
	if !actionExecutableAllowed("/usr/bin/systemctl", []string{"systemctl"}) {
		t.Fatal("expected basename allowlist match")
	}
	if actionExecutableAllowed("/usr/bin/rm", []string{"systemctl"}) {
		t.Fatal("expected executable to be blocked")
	}
}

func TestRunAlertAction_EmptyFiringsNoop(t *testing.T) {
	err := RunAlertAction(config.AlertActionConfig{
		Command:            "echo test",
		AllowedExecutables: []string{"echo"},
		Timeout:            1,
	}, nil)
	if err != nil {
		t.Fatalf("expected nil error for empty firings, got %v", err)
	}
}

func TestSendAlertRoutes(t *testing.T) {
	var received webhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	t.Setenv("WATCHSSH_WEBHOOK_TOKEN", "Bearer test-token")

	firings := []Firing{{RuleName: "HighCPU", Metric: "cpu_usage", Server: "app-01", FiredAt: time.Now()}}
	err := SendAlertRoutes([]config.AlertRoute{{
		Name:    "primary",
		Metrics: []string{"cpu_usage"},
		Webhook: config.WebhookConfig{
			URL:       server.URL,
			Method:    http.MethodPost,
			Timeout:   1,
			HeaderEnv: map[string]string{"Authorization": "WATCHSSH_WEBHOOK_TOKEN"},
		},
	}}, firings)
	if err != nil {
		t.Fatal(err)
	}
	if received.Route != "primary" || len(received.Alerts) != 1 || received.Alerts[0].Server != "app-01" {
		t.Fatalf("received payload = %#v", received)
	}
}

func TestWebhookBodyTemplate(t *testing.T) {
	body, err := webhookBody(`{"text": {{json .Summary}}, "count": {{json .Alerts}}}`, webhookPayload{
		Summary: "High CPU",
		Alerts:  []Firing{{Server: "app-01"}},
	})
	if err != nil || !json.Valid(body) {
		t.Fatalf("body = %s, err = %v", body, err)
	}
}

func TestSendAlertRoutesContinue(t *testing.T) {
	var primaryCalls atomic.Int32
	var escalationCalls atomic.Int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		primaryCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer primary.Close()
	escalation := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		escalationCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer escalation.Close()

	err := SendAlertRoutes([]config.AlertRoute{
		{Name: "primary", Continue: true, Webhook: config.WebhookConfig{URL: primary.URL}},
		{Name: "escalation", Webhook: config.WebhookConfig{URL: escalation.URL}},
	}, []Firing{{RuleName: "HighCPU", Metric: "cpu_usage", Server: "app-01", FiredAt: time.Now()}})
	if err != nil {
		t.Fatal(err)
	}
	if primaryCalls.Load() != 1 || escalationCalls.Load() != 1 {
		t.Fatalf("route calls = primary:%d escalation:%d, want 1:1", primaryCalls.Load(), escalationCalls.Load())
	}
}

func TestSendAlertRoutesIRC(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	lines := make(chan []string, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			lines <- nil
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		var received []string
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				lines <- received
				return
			}
			line = strings.TrimSpace(line)
			received = append(received, line)
			if strings.HasPrefix(line, "USER ") {
				_, _ = conn.Write([]byte(":test 001 watchssh :Welcome\r\n"))
			}
			if strings.HasPrefix(line, "QUIT ") {
				lines <- received
				return
			}
		}
	}()

	err = SendAlertRoutes([]config.AlertRoute{{
		Name: "irc", IRC: &config.IRCConfig{
			Address: listener.Addr().String(), Nick: "watchssh", Username: "watchssh", RealName: "WatchSSH", Channel: "#ops", Timeout: 1,
		},
	}}, []Firing{{Message: "CPU high on app-01", FiredAt: time.Now()}})
	if err != nil {
		t.Fatal(err)
	}
	got := <-lines
	joined, notified := false, false
	for _, line := range got {
		joined = joined || line == "JOIN #ops"
		notified = notified || line == "PRIVMSG #ops :CPU high on app-01"
	}
	if !joined || !notified {
		t.Fatalf("IRC commands = %#v, want JOIN and PRIVMSG", got)
	}
}

func TestSendAlertRoutesSyslog(t *testing.T) {
	receiver, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()

	err = SendAlertRoutes([]config.AlertRoute{{
		Name: "syslog", Syslog: &config.SyslogConfig{
			Address: receiver.LocalAddr().String(), Network: "udp", AppName: "watchssh-test", Timeout: 1,
		},
	}}, []Firing{{Message: "CPU high on app-01", FiredAt: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)}})
	if err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 2048)
	if err := receiver.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	n, _, err := receiver.ReadFrom(buffer)
	if err != nil {
		t.Fatal(err)
	}
	got := string(buffer[:n])
	if !strings.HasPrefix(got, "<131>1 2026-07-11T12:00:00Z ") || !strings.Contains(got, " watchssh-test - WATCHSSH - CPU high on app-01") {
		t.Fatalf("syslog record = %q", got)
	}
}

func TestRunRemediationsLocalAndCooldown(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.Server{{Name: "web-01", Local: true}},
		Alerts: config.AlertsConfig{Remediations: []config.RemediationConfig{{
			Name: "restart-web", Enabled: true, Rules: []string{"WebUnavailable"},
			Command: "printf restarted", Timeout: 1, Cooldown: 60, MaxAttempts: 3, Window: 3600,
		}}},
	}
	monitor := &Monitor{remediationMgr: NewRemediationManager()}
	firings := []Firing{{RuleName: "WebUnavailable", Metric: "http_failed", Server: "web-01"}}
	monitor.runRemediations(cfg, firings)
	if len(firings[0].Remediations) != 1 {
		t.Fatalf("remediation results = %#v", firings[0].Remediations)
	}
	first := firings[0].Remediations[0]
	if first.Status != "succeeded" || first.Output != "restarted" {
		t.Fatalf("first remediation = %#v", first)
	}

	secondFirings := []Firing{{RuleName: "WebUnavailable", Metric: "http_failed", Server: "web-01"}}
	monitor.runRemediations(cfg, secondFirings)
	if got := secondFirings[0].Remediations; len(got) != 1 || got[0].Status != "skipped_cooldown" {
		t.Fatalf("second remediation results = %#v", got)
	}
}

func TestRemediationManagerRateLimit(t *testing.T) {
	manager := NewRemediationManager()
	remediation := config.RemediationConfig{Name: "restart-web", Cooldown: 1, MaxAttempts: 2, Window: 60}
	now := time.Now()
	if allowed, status := manager.allow(remediation, "web-01", now); !allowed || status != "" {
		t.Fatalf("first attempt = allowed:%v status:%q", allowed, status)
	}
	if allowed, status := manager.allow(remediation, "web-01", now.Add(2*time.Second)); !allowed || status != "" {
		t.Fatalf("second attempt = allowed:%v status:%q", allowed, status)
	}
	if allowed, status := manager.allow(remediation, "web-01", now.Add(4*time.Second)); allowed || status != "skipped_rate_limit" {
		t.Fatalf("third attempt = allowed:%v status:%q", allowed, status)
	}
}

func TestRunWatchdogSelectsOnlyAllowlistedRemediations(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		var request chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Model != "local-model" || request.ResponseFormat == nil {
			t.Fatalf("watchdog request = %#v", request)
		}
		if strings.Contains(request.Messages[1].Content, "sensitive-web-01") {
			t.Fatal("watchdog telemetry sent a server identifier without opt-in")
		}
		if !strings.Contains(request.Messages[1].Content, `"failed_probe_count":1`) || !strings.Contains(request.Messages[1].Content, `"failed_probe_types":["http"]`) {
			t.Fatalf("watchdog telemetry is missing probe failure summary: %s", request.Messages[1].Content)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"summary\":\"HTTP health check failed\",\"severity\":\"critical\",\"remediations\":[\"restart-web\",\"not-allowed\"]}"}}]}`))
	}))
	defer api.Close()

	cfg := &config.Config{
		Servers: []config.Server{{Name: "sensitive-web-01", Local: true}},
		Alerts: config.AlertsConfig{
			Remediations: []config.RemediationConfig{{
				Name: "restart-web", Description: "Restart the web service", Enabled: true, Mode: "watchdog",
				Command: "printf restarted", Timeout: 1, Cooldown: 60, MaxAttempts: 3, Window: 3600,
			}},
			Watchdog: &config.WatchdogConfig{
				Enabled: true, BaseURL: api.URL, Model: "local-model", Timeout: 1, Cooldown: 60,
				MaxInputBytes: 4096, MaxTokens: 100, ResponseFormat: "json_object", AllowedRemediations: []string{"restart-web"},
			},
		},
	}
	monitor := &Monitor{remediationMgr: NewRemediationManager(), watchdogMgr: NewWatchdogManager()}
	metrics := []ServerMetrics{{
		ServerName: "sensitive-web-01", Timestamp: time.Now(),
		Connectivity: ConnectivityStats{HTTP: []HTTPResult{{URL: "https://private.example.test/health", OK: false, StatusCode: 503}}},
	}}
	firings := []Firing{{RuleName: "health-down", Metric: "http_failed", Server: "sensitive-web-01", Value: 503}}
	monitor.runWatchdog(cfg, metrics, firings)

	result := firings[0].Watchdog
	if result == nil || result.Status != "analyzed" || result.Severity != "critical" || result.Summary != "HTTP health check failed" {
		t.Fatalf("watchdog result = %#v", result)
	}
	if len(result.Remediations) != 1 || result.Remediations[0].Status != "succeeded" || result.Remediations[0].Output != "restarted" {
		t.Fatalf("watchdog remediations = %#v", result.Remediations)
	}
	if len(result.RejectedRemediations) != 1 || result.RejectedRemediations[0] != "not-allowed" {
		t.Fatalf("rejected remediations = %#v", result.RejectedRemediations)
	}

	monitor.runRemediations(cfg, firings)
	if len(firings[0].Remediations) != 0 {
		t.Fatalf("watchdog-only remediation ran through deterministic alert path: %#v", firings[0].Remediations)
	}
}

func TestRunWatchdogDefersActionsBelowMinimumSeverity(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"summary\":\"Transient latency increase\",\"severity\":\"warning\",\"remediations\":[\"restart-web\"]}"}}]}`))
	}))
	defer api.Close()

	cfg := &config.Config{
		Servers: []config.Server{{Name: "web-01", Local: true}},
		Alerts: config.AlertsConfig{
			Remediations: []config.RemediationConfig{{
				Name: "restart-web", Enabled: true, Mode: "watchdog", Command: "printf restarted", Timeout: 1, Cooldown: 60, MaxAttempts: 3, Window: 3600,
			}},
			Watchdog: &config.WatchdogConfig{
				Enabled: true, BaseURL: api.URL, Model: "local-model", Timeout: 1, Cooldown: 60,
				MaxInputBytes: 4096, MaxTokens: 100, ResponseFormat: "json_object", MinRemediationSeverity: "critical", AllowedRemediations: []string{"restart-web"},
			},
		},
	}
	monitor := &Monitor{remediationMgr: NewRemediationManager(), watchdogMgr: NewWatchdogManager()}
	metrics := []ServerMetrics{{ServerName: "web-01", Timestamp: time.Now()}}
	firings := []Firing{{RuleName: "high-latency", Metric: "http_latency", Server: "web-01", Value: 2500}}
	monitor.runWatchdog(cfg, metrics, firings)

	result := firings[0].Watchdog
	if result == nil || result.Status != "analyzed" || result.Severity != "warning" {
		t.Fatalf("watchdog result = %#v", result)
	}
	if len(result.Remediations) != 0 || len(result.DeferredRemediations) != 1 || result.DeferredRemediations[0] != "restart-web" {
		t.Fatalf("watchdog action gate = %#v", result)
	}
}
