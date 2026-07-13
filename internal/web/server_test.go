package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
	"github.com/SimonWaldherr/WatchSSH/internal/history"
	"github.com/SimonWaldherr/WatchSSH/internal/monitor"
	"golang.org/x/crypto/bcrypt"
)

func TestHealthz(t *testing.T) {
	state := NewState(&config.Config{}, "")
	srv := NewServer(state, ":0")

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "ok\n" {
		t.Fatalf("body = %q, want %q", got, "ok\n")
	}
}

func TestDashboardAuthentication(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("correct horse battery staple"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	state := NewState(&config.Config{Web: config.WebConfig{Auth: &config.WebAuthConfig{
		Username:     "ops",
		PasswordHash: string(hash),
	}}}, "")
	srv := NewServer(state, ":0")

	unauthenticated := httptest.NewRequest(http.MethodGet, "/", nil)
	unauthenticatedRecorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unauthenticatedRecorder, unauthenticated)
	if unauthenticatedRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated dashboard status = %d, want %d", unauthenticatedRecorder.Code, http.StatusUnauthorized)
	}
	if got := unauthenticatedRecorder.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q, want DENY", got)
	}
	if got := unauthenticatedRecorder.Header().Get("Content-Security-Policy"); !strings.Contains(got, "frame-ancestors 'none'") {
		t.Fatalf("Content-Security-Policy = %q, missing frame-ancestors", got)
	}
	if got := unauthenticatedRecorder.Header().Get("WWW-Authenticate"); !strings.Contains(got, "Basic") {
		t.Fatalf("WWW-Authenticate = %q, want Basic challenge", got)
	}

	health := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthRecorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(healthRecorder, health)
	if healthRecorder.Code != http.StatusOK {
		t.Fatalf("public health status = %d, want %d", healthRecorder.Code, http.StatusOK)
	}

	authenticated := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	authenticated.SetBasicAuth("ops", "correct horse battery staple")
	authenticatedRecorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(authenticatedRecorder, authenticated)
	if authenticatedRecorder.Code != http.StatusOK {
		t.Fatalf("authenticated API status = %d, want %d", authenticatedRecorder.Code, http.StatusOK)
	}
}

func TestInterfaceModeControlIsRendered(t *testing.T) {
	state := NewState(&config.Config{}, "")
	srv := NewServer(state, ":0")

	req := httptest.NewRequest(http.MethodGet, "/servers", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="ui-mode"`,
		`value="beginner"`,
		`value="advanced"`,
		`value="expert"`,
		"watchssh-ui-mode",
		"Custom remote check",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %q", want)
		}
	}
}

func TestReadyzNotReadyWithoutMetrics(t *testing.T) {
	state := NewState(&config.Config{
		Servers: []config.Server{{Name: "web-01", Host: "192.0.2.10", Username: "monitor"}},
	}, "")
	srv := NewServer(state, ":0")

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if payload["status"] != "not_ready" {
		t.Fatalf("status payload = %v, want not_ready", payload["status"])
	}
}

func TestReadyzReadyWithMetrics(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.Server{{Name: "web-01", Host: "192.0.2.10", Username: "monitor"}},
	}
	state := NewState(cfg, "")
	state.Update([]monitor.ServerMetrics{{ServerName: "web-01", Host: "192.0.2.10"}}, nil)
	srv := NewServer(state, ":0")

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if payload["status"] != "ready" {
		t.Fatalf("status payload = %v, want ready", payload["status"])
	}
}

func TestServerDetailShowsDockerAndCollectorDiagnostics(t *testing.T) {
	cfg := &config.Config{
		Servers: []config.Server{{Name: "localhost", Local: true, Docker: config.DockerConfig{Enabled: true}}},
	}
	state := NewState(cfg, "")
	state.Update([]monitor.ServerMetrics{{
		ServerName: "localhost",
		Timestamp:  time.Now(),
		System: monitor.SystemInfo{
			Hostname: "localhost",
			OS:       "Linux",
		},
		Capabilities: map[string]string{
			"containers": "ok",
			"cpu":        "ok",
		},
		MetricErrors: map[string]string{
			"containers": "docker socket not mounted",
		},
		Containers: []monitor.ContainerInfo{{
			Name:          "api",
			Image:         "ghcr.io/example/api:latest",
			Status:        "Up 2 hours",
			CPUPercent:    12.5,
			MemUsedBytes:  512 * 1024 * 1024,
			MemLimitBytes: 1024 * 1024 * 1024,
		}},
	}}, nil)
	srv := NewServer(state, ":0")

	req := httptest.NewRequest(http.MethodGet, "/server/localhost", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	for _, want := range []string{"Docker Containers", "Collector Status", "docker socket not mounted", "ghcr.io/example/api:latest"} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %q", want)
		}
	}
}

func TestAddServerWithProfileAndChecks(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &config.Config{}
	state := NewState(cfg, cfgPath)
	srv := NewServer(state, ":0")

	form := url.Values{}
	form.Set("profile", "harp")
	form.Set("name", "harp-edge")
	form.Set("host", "harp.example.com")
	form.Set("port", "22")
	form.Set("username", "monitor")
	form.Set("auth_type", "key")
	form.Set("auth_credential", "~/.ssh/id_ed25519")
	form.Set("tags", "edge")
	form.Set("ports", "22")
	form.Set("banner_hosts", "ssh.example.com")
	form.Set("banner_port", "22")
	form.Set("banner_expected_prefix", "SSH-")
	form.Set("http_method", "HEAD")
	form.Set("ntp_hosts", "time.example.com")
	form.Set("ntp_max_offset_ms", "50")
	form.Set("ping", "1")
	form.Set("docker_enabled", "1")
	req := httptest.NewRequest(http.MethodPost, "/servers/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	got := state.Config()
	if len(got.Servers) != 1 {
		t.Fatalf("servers len = %d, want 1", len(got.Servers))
	}
	added := got.Servers[0]
	if added.Name != "harp-edge" || added.Host != "harp.example.com" || !added.Docker.Enabled || !added.Checks.Ping.Enabled {
		t.Fatalf("added server basics = %#v", added)
	}
	if len(added.Checks.HTTP) != 3 || len(added.Checks.DNS) != 1 || len(added.Checks.TLS) != 1 {
		t.Fatalf("profile checks = %#v, want 3 http/1 dns/1 tls", added.Checks)
	}
	if len(added.Checks.Ports) != 3 {
		t.Fatalf("ports = %#v, want manual 22 plus profile 80/443", added.Checks.Ports)
	}
	if len(added.Checks.NTP) != 1 || added.Checks.NTP[0].Host != "time.example.com" || added.Checks.NTP[0].MaxOffsetMs != 50 {
		t.Fatalf("NTP checks = %#v", added.Checks.NTP)
	}
	if len(added.Checks.Banner) != 1 || added.Checks.Banner[0].Host != "ssh.example.com" || added.Checks.Banner[0].ExpectedPrefix != "SSH-" {
		t.Fatalf("banner checks = %#v", added.Checks.Banner)
	}
	for _, want := range []string{"edge", "harp", "reverse-proxy"} {
		if !containsString(added.Tags, want) {
			t.Fatalf("tags = %#v, missing %q", added.Tags, want)
		}
	}
}

func TestAddAlertWithHTTPURL(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	state := NewState(&config.Config{Servers: []config.Server{{Name: "web-01", Local: true}}}, cfgPath)
	srv := NewServer(state, ":0")

	form := url.Values{}
	form.Set("name", "health-slow")
	form.Set("metric", "http_latency")
	form.Set("operator", ">")
	form.Set("threshold", "2000")
	form.Set("url", "https://example.test/health")
	form.Add("servers", "web-01")
	req := httptest.NewRequest(http.MethodPost, "/alerts/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	rules := state.Config().Alerts.Rules
	if len(rules) != 1 || rules[0].URL != "https://example.test/health" {
		t.Fatalf("rules = %#v", rules)
	}
}

func TestAlertsPageShowsRemediations(t *testing.T) {
	state := NewState(&config.Config{
		Servers: []config.Server{{Name: "web-01", Local: true}},
		Alerts: config.AlertsConfig{Remediations: []config.RemediationConfig{{
			Name: "restart-web", Enabled: true, Rules: []string{"health-down"}, Command: "service web restart",
			Cooldown: 300, MaxAttempts: 3, Window: 3600,
		}}, Watchdog: &config.WatchdogConfig{
			Enabled: true, Model: "local-model", Cooldown: 300, AllowedRemediations: []string{"restart-web"},
		}},
	}, "")
	state.Update(nil, []monitor.Firing{{
		Message:      "health check failed",
		Remediations: []monitor.RemediationResult{{Name: "restart-web", Target: "web-01", Status: "succeeded"}},
		Watchdog:     &monitor.WatchdogResult{Model: "local-model", Status: "analyzed", Severity: "critical", Summary: "Restart selected"},
	}})
	srv := NewServer(state, ":0")

	req := httptest.NewRequest(http.MethodGet, "/alerts", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{"AI Watchdog", "local-model", "Automatic Remediations", "restart-web", "Watchdog local-model: analyzed (critical) - Restart selected", "Remediation restart-web on web-01: succeeded"} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %q", want)
		}
	}
}

func TestHistoryDisabledAPI(t *testing.T) {
	state := NewState(&config.Config{}, "")
	srv := NewServer(state, ":0")

	req := httptest.NewRequest(http.MethodGet, "/api/history/metrics", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(rec.Body.String(), "history storage is not enabled") {
		t.Fatalf("response body missing disabled message: %s", rec.Body.String())
	}
}

func TestHistoryPageAndAPI(t *testing.T) {
	store, err := history.OpenTinySQL(filepath.Join(t.TempDir(), "history.tinysql"))
	if err != nil {
		t.Fatalf("OpenTinySQL() error = %v", err)
	}
	defer store.Close()

	cpuUsage := 12.5
	if err := store.RecordMetrics(httptest.NewRequest(http.MethodGet, "/", nil).Context(), []history.MetricRecord{{
		ID:          "metric-1",
		CollectedAt: "2026-07-08T12:00:00Z",
		ServerName:  "localhost",
		Host:        "127.0.0.1",
		Platform:    "Linux",
		CPUUsage:    &cpuUsage,
		PayloadJSON: `{"server_name":"localhost"}`,
	}}); err != nil {
		t.Fatalf("RecordMetrics() error = %v", err)
	}
	if err := store.RecordFirings(httptest.NewRequest(http.MethodGet, "/", nil).Context(), []history.FiringRecord{{
		ID:          "firing-1",
		FiredAt:     "2026-07-08T12:00:01Z",
		RuleName:    "HighCPU",
		Metric:      "cpu_usage",
		Server:      "localhost",
		Value:       91.5,
		Message:     "HighCPU triggered",
		PayloadJSON: `{"rule_name":"HighCPU"}`,
	}}); err != nil {
		t.Fatalf("RecordFirings() error = %v", err)
	}

	state := NewState(&config.Config{Storage: config.StorageConfig{Type: "tinysql"}}, "")
	srv := NewServer(state, ":0", store)

	req := httptest.NewRequest(http.MethodGet, "/history", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("history status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{"Metric Samples", "localhost", "HighCPU"} {
		if !strings.Contains(body, want) {
			t.Fatalf("history page missing %q", want)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/api/history/metrics?server=localhost&limit=1", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("history API status = %d, want %d", rec.Code, http.StatusOK)
	}
	var metrics []history.MetricRecord
	if err := json.Unmarshal(rec.Body.Bytes(), &metrics); err != nil {
		t.Fatalf("unmarshal history API: %v", err)
	}
	if len(metrics) != 1 || metrics[0].ServerName != "localhost" {
		t.Fatalf("history API metrics = %#v, want localhost record", metrics)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/history/summary?limit=10", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("history summary status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"average_cpu_usage": 12.5`) {
		t.Fatalf("history summary missing average CPU: %s", rec.Body.String())
	}
}

func TestPrometheusMetricsEndpoint(t *testing.T) {
	state := NewState(&config.Config{}, "")
	tlsDays := 12.5
	boardTemp := 52.3
	boardFreq := 1400.0
	boardRSSI := -61.0
	state.Update([]monitor.ServerMetrics{{
		ServerName: "localhost",
		Host:       "127.0.0.1",
		Platform:   "Linux",
		CPU:        &monitor.CPUStats{UsagePercent: 12.5},
		Memory:     &monitor.MemoryStats{UsagePercent: 43.2},
		Disks:      []monitor.DiskStats{{MountPoint: "/", Device: "/dev/disk1", UsagePercent: 55.5}},
		Connectivity: monitor.ConnectivityStats{
			DNS:        []monitor.DNSResult{{Name: "dns", Host: "example.com", Type: "A", OK: true, LatencyMs: 12}},
			TLS:        []monitor.TLSResult{{Name: "tls", Host: "example.com", Port: 443, OK: true, CertExpiresDays: &tlsDays}},
			Traceroute: []monitor.TracerouteResult{{Name: "trace", Host: "example.com", OK: true, Hops: 8}},
		},
		Board: &monitor.BoardInfo{
			Model:           "Raspberry Pi 5 Model B",
			TemperatureC:    &boardTemp,
			CPUFrequencyMHz: &boardFreq,
			WiFiInterface:   "wlan0",
			WiFiRSSIDbm:     &boardRSSI,
			ThrottledNow:    true,
		},
	}}, nil)
	srv := NewServer(state, ":0")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{"watchssh_up", "watchssh_cpu_usage_percent", "watchssh_memory_usage_percent", "watchssh_disk_usage_percent", "watchssh_dns_probe_up", "watchssh_tls_probe_up", "watchssh_traceroute_hops", "watchssh_board_temperature_celsius", "watchssh_board_wifi_rssi_dbm", "watchssh_board_throttled"} {
		if !strings.Contains(body, want) {
			t.Fatalf("prometheus metrics missing %q: %s", want, body)
		}
	}
}

func TestAPIProbes(t *testing.T) {
	state := NewState(&config.Config{}, "")
	state.Update([]monitor.ServerMetrics{{
		ServerName: "localhost",
		Host:       "127.0.0.1",
		Connectivity: monitor.ConnectivityStats{
			DNS: []monitor.DNSResult{{Name: "dns", Host: "example.com", Type: "A", OK: true}},
		},
	}}, nil)
	srv := NewServer(state, ":0")

	req := httptest.NewRequest(http.MethodGet, "/api/probes?server=localhost", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"dns"`) {
		t.Fatalf("probe API missing dns result: %s", rec.Body.String())
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
