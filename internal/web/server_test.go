package web

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
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

func TestOperationalEndpointsAndOpenAPI(t *testing.T) {
	state := NewState(&config.Config{}, "")
	srv := NewServer(state, ":0")

	for _, path := range []string{"/healthz", "/livez"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", path, rec.Code, http.StatusOK)
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s Cache-Control = %q, want no-store", path, rec.Header().Get("Cache-Control"))
		}
		if rec.Header().Get("X-Request-ID") == "" {
			t.Fatalf("%s did not return a request ID", path)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("OpenAPI status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "application/vnd.oai.openapi+json") {
		t.Fatalf("OpenAPI Content-Type = %q", rec.Header().Get("Content-Type"))
	}
	var document map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &document); err != nil {
		t.Fatalf("OpenAPI document is invalid JSON: %v", err)
	}
	if document["openapi"] != "3.1.0" {
		t.Fatalf("OpenAPI version = %v, want 3.1.0", document["openapi"])
	}
	paths, ok := document["paths"].(map[string]any)
	if !ok || paths["/api/v1/metrics"] == nil {
		t.Fatalf("OpenAPI document does not declare /api/v1/metrics")
	}
}

func TestVersionedAPIAndProblemResponses(t *testing.T) {
	state := NewState(&config.Config{}, "")
	srv := NewServer(state, ":0")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("versioned metrics status = %d, want %d", rec.Code, http.StatusOK)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/probes", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("versioned probes POST status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "application/problem+json") {
		t.Fatalf("problem Content-Type = %q", rec.Header().Get("Content-Type"))
	}
	var problem map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("problem response is invalid JSON: %v", err)
	}
	if problem["status"] != float64(http.StatusMethodNotAllowed) || problem["request_id"] == "" {
		t.Fatalf("unexpected problem response: %#v", problem)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/history/metrics", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled history status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "application/problem+json") {
		t.Fatalf("disabled history Content-Type = %q", rec.Header().Get("Content-Type"))
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

func TestCSRFMiddlewareProtectsDashboardChanges(t *testing.T) {
	state := NewState(&config.Config{}, "")
	srv := NewServer(state, ":0")

	get := httptest.NewRequest(http.MethodGet, "/config", nil)
	getRecorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRecorder, get)
	var csrfCookie *http.Cookie
	for _, cookie := range getRecorder.Result().Cookies() {
		if cookie.Name == csrfCookieName {
			csrfCookie = cookie
			break
		}
	}
	if csrfCookie == nil || len(csrfCookie.Value) != 64 {
		t.Fatalf("CSRF cookie = %#v, want random token", csrfCookie)
	}
	if csrfCookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("CSRF SameSite = %v, want Strict", csrfCookie.SameSite)
	}

	blocked := httptest.NewRequest(http.MethodPost, "/config", strings.NewReader("web_enabled=0"))
	blocked.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	blocked.AddCookie(csrfCookie)
	blockedRecorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(blockedRecorder, blocked)
	if blockedRecorder.Code != http.StatusForbidden {
		t.Fatalf("POST without CSRF token status = %d, want %d", blockedRecorder.Code, http.StatusForbidden)
	}

	allowed := httptest.NewRequest(http.MethodPost, "/config", strings.NewReader("web_enabled=0&csrf_token="+csrfCookie.Value))
	allowed.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	allowed.AddCookie(csrfCookie)
	allowedRecorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(allowedRecorder, allowed)
	if allowedRecorder.Code != http.StatusSeeOther {
		t.Fatalf("POST with CSRF token status = %d, want %d", allowedRecorder.Code, http.StatusSeeOther)
	}
}

func TestDashboardRequestSizeLimit(t *testing.T) {
	state := NewState(&config.Config{}, "")
	srv := NewServer(state, ":0")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/probes", strings.NewReader(strings.Repeat("x", maxDashboardRequestBody+1)))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized request status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestConfigurationPageExplainsDashboardProtection(t *testing.T) {
	state := NewState(&config.Config{Web: config.WebConfig{Enabled: true, Listen: "127.0.0.1:8080"}}, "")
	srv := NewServer(state, ":0")
	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("configuration page status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "protected by loopback access only") {
		t.Fatal("configuration page does not explain unauthenticated loopback protection")
	}
}

func TestInterfaceModeControlIsRendered(t *testing.T) {
	state := NewState(&config.Config{Servers: []config.Server{{Name: "localhost", Local: true}}}, "")
	srv := NewServer(state, ":0")

	req := httptest.NewRequest(http.MethodGet, "/servers", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`class="skip-link"`,
		`id="main-content"`,
		`aria-label="Primary navigation"`,
		`aria-current="page"`,
		`id="ui-mode"`,
		`id="ui-language"`,
		`value="beginner"`,
		`value="advanced"`,
		`value="expert"`,
		"watchssh-ui-mode",
		"watchssh-ui-language",
		"data-i18n",
		"Custom remote check",
		"Probe Library",
		"/probes/import",
		"/probes/export",
		`role="status" aria-live="polite"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %q", want)
		}
	}
}

func TestDashboardRendersHealthSummary(t *testing.T) {
	state := NewState(&config.Config{}, "")
	state.Update([]monitor.ServerMetrics{
		{ServerName: "ok", Timestamp: time.Now()},
		{ServerName: "warn", Timestamp: time.Now(), CPU: &monitor.CPUStats{UsagePercent: 95}},
		{ServerName: "down", Error: "connection refused"},
		{ServerName: "pending"},
	}, nil)
	srv := NewServer(state, ":0")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Operations Overview",
		`data-health-filter="warn"`,
		`data-server-status="error"`,
		`aria-pressed="true"`,
		"Needs attention",
		"No targets match this status filter.",
		"waiting for their first result",
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

func TestServerDetailShowsAgentlessUnixToolProbes(t *testing.T) {
	expiresAt := time.Date(2099, time.December, 31, 23, 59, 59, 0, time.UTC)
	state := NewState(&config.Config{}, "")
	state.Update([]monitor.ServerMetrics{{
		ServerName:      "app-01",
		FileChecks:      []monitor.FileCheckResult{{Name: "pid", Path: "/run/app.pid", SizeBytes: 12, AgeSeconds: 4, OK: true}},
		DirectoryChecks: []monitor.DirectoryResult{{Name: "cache", Path: "/var/cache/app", UsedBytes: 1024, MaxFileCount: 10, FileCount: 11, FileCountCapped: true, OK: false, Error: "file count exceeds 10"}},
		LogChecks:       []monitor.LogCheckResult{{Name: "errors", Path: "/var/log/app.log", Pattern: "ERROR", Lines: 200, Count: 1, MaxCount: 0, OK: false}},
		CommandChecks:   []monitor.CommandCheckResult{{Name: "docker", Command: "docker", ResolvedPath: "/usr/bin/docker", OK: true}},
		HashChecks:      []monitor.HashCheckResult{{Name: "config", Path: "/etc/app.conf", Algorithm: "sha256", ObservedDigest: strings.Repeat("a", 64), OK: true}},
		CertFileChecks:  []monitor.CertificateFileCheckResult{{Name: "local-cert", Path: "/etc/ssl/local.pem", ExpiresAt: &expiresAt, ExpiresDays: 365, WarnDays: 30, OK: true}},
	}}, nil)
	srv := NewServer(state, ":0")
	req := httptest.NewRequest(http.MethodGet, "/server/app-01", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{"Agentless Unix Tool Probes", "test + stat", "du + find", "tail + grep", "command -v", "sha*sum / shasum / openssl", "openssl x509", "/run/app.pid", "/var/cache/app", "/var/log/app.log", "/etc/app.conf", "/etc/ssl/local.pem"} {
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

func TestProbeWorkspaceAddExportImportAndRemove(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	state := NewState(&config.Config{Servers: []config.Server{{Name: "app-01", Host: "app.internal", Username: "monitor"}, {Name: "app-02", Host: "app-02.internal", Username: "monitor"}}}, cfgPath)
	srv := NewServer(state, ":0")

	form := url.Values{"server": {"app-01"}, "kind": {"tcp"}, "target": {"db.internal"}, "probe_port": {"5432"}, "source": {"target"}, "timeout": {"5"}}
	req := httptest.NewRequest(http.MethodPost, "/probes/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("add probe status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	checks := state.Config().Servers[0].Checks
	if len(checks.Ports) != 1 || checks.Ports[0].Host != "db.internal" || checks.Ports[0].Source != "target" {
		t.Fatalf("added checks = %#v", checks)
	}
	state.UpdateServer("app-01", func(srv *config.Server) {
		srv.Checks.Command = append(srv.Checks.Command, config.CommandCheck{Name: "docker", Command: "docker", Timeout: 5})
		srv.Checks.Hash = append(srv.Checks.Hash, config.HashCheck{Name: "app-config", Path: "/etc/application.conf", Algorithm: "sha256", ExpectedDigest: strings.Repeat("a", 64), Timeout: 10})
		srv.Checks.CertFile = append(srv.Checks.CertFile, config.CertificateFileCheck{Name: "local-cert", Path: "/etc/ssl/local.pem", WarnDays: 30, Timeout: 5})
	})

	req = httptest.NewRequest(http.MethodGet, "/probes/export?server=app-01", nil)
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Disposition"), "watchssh-app-01-probes.json") {
		t.Fatalf("export status/headers = %d %#v", rec.Code, rec.Header())
	}
	var bundle probeBundle
	if err := json.Unmarshal(rec.Body.Bytes(), &bundle); err != nil || bundle.Version != 1 || len(bundle.Checks.Ports) != 1 || len(bundle.Checks.Command) != 1 || len(bundle.Checks.Hash) != 1 || len(bundle.Checks.CertFile) != 1 {
		t.Fatalf("export bundle = %#v, %v", bundle, err)
	}

	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	part, err := writer.CreateFormFile("bundle", "probes.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(rec.Body.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("server", "app-02"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/probes/import", &payload)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	importedChecks := state.Config().Servers[1].Checks
	if rec.Code != http.StatusSeeOther || len(importedChecks.Ports) != 1 || len(importedChecks.Command) != 1 || len(importedChecks.Hash) != 1 || len(importedChecks.CertFile) != 1 {
		t.Fatalf("import status/checks = %d %#v", rec.Code, state.Config().Servers[1].Checks)
	}

	remove := url.Values{"server": {"app-01"}, "kind": {"tcp"}, "index": {"0"}}
	req = httptest.NewRequest(http.MethodPost, "/probes/remove", strings.NewReader(remove.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || len(state.Config().Servers[0].Checks.Ports) != 0 {
		t.Fatalf("remove status/checks = %d %#v", rec.Code, state.Config().Servers[0].Checks)
	}
}

func TestProbeWorkspaceStandardToolProbes(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	state := NewState(&config.Config{Servers: []config.Server{{Name: "app-01", Host: "app.internal", Username: "monitor"}}}, cfgPath)
	srv := NewServer(state, ":0")

	add := func(form url.Values) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/probes/add", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("add probe %v status = %d, want %d (flash: %s)", form, rec.Code, http.StatusSeeOther, rec.Header().Get("Location"))
		}
	}

	add(url.Values{"server": {"app-01"}, "kind": {"service"}, "unit": {"nginx.service"}})
	add(url.Values{"server": {"app-01"}, "kind": {"process"}, "pattern": {"nginx: worker"}, "min_count": {"2"}})
	add(url.Values{"server": {"app-01"}, "kind": {"listening"}, "probe_port": {"443"}, "protocol": {"tcp"}})
	add(url.Values{"server": {"app-01"}, "kind": {"journal"}, "unit": {"sshd.service"}, "priority": {"crit"}, "since_minutes": {"15"}, "max_count": {"2"}})
	add(url.Values{"server": {"app-01"}, "kind": {"file"}, "path": {"/run/app.pid"}, "max_age_seconds": {"3600"}})
	add(url.Values{"server": {"app-01"}, "kind": {"directory"}, "path": {"/var/cache/app"}, "max_usage_bytes": {"1024"}, "max_file_count": {"10"}})
	add(url.Values{"server": {"app-01"}, "kind": {"log"}, "path": {"/var/log/app.log"}, "pattern": {"ERROR"}, "log_lines": {"500"}, "max_count": {"2"}})
	add(url.Values{"server": {"app-01"}, "kind": {"command"}, "required_command": {"docker"}})
	add(url.Values{"server": {"app-01"}, "kind": {"hash"}, "hash_path": {"/etc/application.conf"}, "algorithm": {"sha256"}, "expected_digest": {strings.Repeat("a", 64)}})
	add(url.Values{"server": {"app-01"}, "kind": {"certificate_file"}, "certificate_path": {"/etc/letsencrypt/live/application/fullchain.pem"}, "warn_days": {"21"}})

	checks := state.Config().Servers[0].Checks
	if len(checks.Service) != 1 || checks.Service[0].Unit != "nginx.service" {
		t.Fatalf("service checks = %#v", checks.Service)
	}
	if len(checks.Process) != 1 || checks.Process[0].Pattern != "nginx: worker" || checks.Process[0].MinCount != 2 {
		t.Fatalf("process checks = %#v", checks.Process)
	}
	if len(checks.Listening) != 1 || checks.Listening[0].Port != 443 || checks.Listening[0].Protocol != "tcp" {
		t.Fatalf("listening checks = %#v", checks.Listening)
	}
	if len(checks.Journal) != 1 || checks.Journal[0].Unit != "sshd.service" || checks.Journal[0].Priority != "crit" || checks.Journal[0].SinceMinutes != 15 || checks.Journal[0].MaxCount != 2 {
		t.Fatalf("journal checks = %#v", checks.Journal)
	}
	if len(checks.File) != 1 || checks.File[0].Path != "/run/app.pid" || checks.File[0].MaxAgeSeconds != 3600 {
		t.Fatalf("file checks = %#v", checks.File)
	}
	if len(checks.Directory) != 1 || checks.Directory[0].Path != "/var/cache/app" || checks.Directory[0].MaxFileCount != 10 {
		t.Fatalf("directory checks = %#v", checks.Directory)
	}
	if len(checks.Log) != 1 || checks.Log[0].Path != "/var/log/app.log" || checks.Log[0].Pattern != "ERROR" || checks.Log[0].Lines != 500 {
		t.Fatalf("log checks = %#v", checks.Log)
	}
	if len(checks.Command) != 1 || checks.Command[0].Command != "docker" || checks.Command[0].Timeout != 5 {
		t.Fatalf("command checks = %#v", checks.Command)
	}
	if len(checks.Hash) != 1 || checks.Hash[0].Path != "/etc/application.conf" || checks.Hash[0].Algorithm != "sha256" || checks.Hash[0].ExpectedDigest != strings.Repeat("a", 64) {
		t.Fatalf("hash checks = %#v", checks.Hash)
	}
	if len(checks.CertFile) != 1 || checks.CertFile[0].Path != "/etc/letsencrypt/live/application/fullchain.pem" || checks.CertFile[0].WarnDays != 21 {
		t.Fatalf("certificate file checks = %#v", checks.CertFile)
	}

	for _, kind := range []string{"service", "process", "listening", "journal", "file", "directory", "log", "command", "hash", "certificate_file"} {
		remove := url.Values{"server": {"app-01"}, "kind": {kind}, "index": {"0"}}
		req := httptest.NewRequest(http.MethodPost, "/probes/remove", strings.NewReader(remove.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("remove %s status = %d, want %d", kind, rec.Code, http.StatusSeeOther)
		}
	}
	checks = state.Config().Servers[0].Checks
	if len(checks.Service) != 0 || len(checks.Process) != 0 || len(checks.Listening) != 0 || len(checks.Journal) != 0 || len(checks.File) != 0 || len(checks.Directory) != 0 || len(checks.Log) != 0 || len(checks.Command) != 0 || len(checks.Hash) != 0 || len(checks.CertFile) != 0 {
		t.Fatalf("checks after removal = %#v", checks)
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

func TestAddAlertWithProbeScope(t *testing.T) {
	state := NewState(&config.Config{Servers: []config.Server{{Name: "app-01", Local: true}}}, "")
	srv := NewServer(state, ":0")
	form := url.Values{"name": {"log-errors"}, "metric": {"log_match_count"}, "operator": {">"}, "threshold": {"0"}, "probe": {"errors"}, "servers": {"app-01"}}
	req := httptest.NewRequest(http.MethodPost, "/alerts/add", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	rules := state.Config().Alerts.Rules
	if len(rules) != 1 || rules[0].Probe != "errors" {
		t.Fatalf("rules = %#v", rules)
	}
}

func TestProcessSortingAndAlertLink(t *testing.T) {
	processes := []monitor.ProcessInfo{
		{PID: 1, CPUPercent: 10, RSSBytes: 100, DiskReadBytes: 1},
		{PID: 2, CPUPercent: 5, RSSBytes: 300, DiskWriteBytes: 20},
	}
	sortProcesses(processes, "memory")
	if processes[0].PID != 2 {
		t.Fatalf("memory sort selected PID %d, want 2", processes[0].PID)
	}
	sortProcesses(processes, "disk")
	if processes[0].PID != 2 {
		t.Fatalf("disk sort selected PID %d, want 2", processes[0].PID)
	}
	link := alertLink("Low disk space", "disk_free_bytes", "<=", 1024, "app-01", "/data")
	parsed, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.Query(); got.Get("servers") != "app-01" || got.Get("mount_point") != "/data" || got.Get("metric") != "disk_free_bytes" {
		t.Fatalf("alert link query = %#v", got)
	}
}

func TestServerDetailSupportsMemoryProcessSort(t *testing.T) {
	state := NewState(&config.Config{}, "")
	state.Update([]monitor.ServerMetrics{{
		ServerName: "localhost",
		Processes: []monitor.ProcessInfo{
			{PID: 1, CPUPercent: 10, RSSBytes: 100},
			{PID: 2, CPUPercent: 1, RSSBytes: 300},
		},
	}}, nil)
	srv := NewServer(state, ":0")
	req := httptest.NewRequest(http.MethodGet, "/server/localhost?process_sort=memory", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Top Processes (by memory)") || !strings.Contains(body, "process_sort=memory\" class=\"active\"") {
		t.Fatalf("memory process sort not rendered: %s", body)
	}
	if strings.Index(body, ">2</td>") > strings.Index(body, ">1</td>") {
		t.Fatalf("processes were not sorted by RAM: %s", body)
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
		Remediations: []monitor.RemediationResult{{Name: "restart-web", Target: "web-01", Status: "succeeded", Verified: true}},
		Watchdog:     &monitor.WatchdogResult{Model: "local-model", Status: "analyzed", Severity: "critical", Summary: "Restart selected", RecommendedRemediations: []string{"restart-web"}},
	}})
	srv := NewServer(state, ":0")

	req := httptest.NewRequest(http.MethodGet, "/alerts", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{"AI Advisor", "Human approval required", "local-model", "Automatic Remediations", "restart-web", "AI advisor local-model: analyzed (critical) - Restart selected", "Operator review required for recommended runbooks: restart-web", "Remediation restart-web on web-01: succeeded (verified)", `id="alert-template"`, "TLS certificate expires soon", "HTTP health check failed"} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %q", want)
		}
	}
}

func TestJobsPageShowsConfigurationAndRecentRuns(t *testing.T) {
	state := NewState(&config.Config{Jobs: []config.ScheduledJobConfig{{
		Name: "update-bavaria-osm", Enabled: true, Schedule: "15 3 * * 1", Timeout: 7200,
		Uploads: []config.JobUploadConfig{{Server: "hetzner-osm", Source: "/srv/osm/bavaria.osm.pbf", Destination: "/srv/www/bavaria.osm.pbf"}},
	}}}, "")
	state.UpdateJobs([]monitor.JobResult{{
		Name: "update-bavaria-osm", StartedAt: time.Date(2026, time.July, 24, 3, 15, 0, 0, time.UTC), DurationMs: 1234, Status: "succeeded",
		Uploads: []monitor.JobUploadResult{{Server: "hetzner-osm", Destination: "/srv/www/bavaria.osm.pbf", Bytes: 2048}},
	}})
	srv := NewServer(state, ":0")

	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{"Scheduled Jobs", "update-bavaria-osm", "15 3 * * 1", "hetzner-osm:/srv/www/bavaria.osm.pbf", "Succeeded", "2.0 KiB"} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %q", want)
		}
	}
}

func TestStateRecordsAuditDiff(t *testing.T) {
	state := NewState(&config.Config{}, "")
	state.RecordAudit("app-01", monitor.AuditResult{Users: []monitor.AuditUser{{Name: "alice", UID: 1000}}, Packages: []string{"nginx"}})
	state.RecordAudit("app-01", monitor.AuditResult{Users: []monitor.AuditUser{{Name: "bob", UID: 1001}}, Packages: []string{"nginx", "curl"}})
	history := state.AuditHistory("app-01")
	if len(history) != 2 {
		t.Fatalf("history = %#v", history)
	}
	latest := history[0]
	if len(latest.AddedUsers) != 1 || latest.AddedUsers[0] != "bob (1001)" || len(latest.RemovedUsers) != 1 || latest.RemovedUsers[0] != "alice (1000)" {
		t.Fatalf("user diff = %#v", latest)
	}
	if len(latest.AddedPackages) != 1 || latest.AddedPackages[0] != "curl" {
		t.Fatalf("package diff = %#v", latest)
	}
}

func TestParseSSHAddress(t *testing.T) {
	tests := []struct {
		input string
		port  int
		host  string
		want  int
	}{
		{input: "ssh.example.test", port: 2222, host: "ssh.example.test", want: 2222},
		{input: "ssh.example.test:50622", port: 22, host: "ssh.example.test", want: 50622},
		{input: "[2001:db8::1]:2200", port: 22, host: "2001:db8::1", want: 2200},
		{input: "2001:db8::1", port: 22, host: "2001:db8::1", want: 22},
	}
	for _, test := range tests {
		host, port, err := parseSSHAddress(test.input, test.port)
		if err != nil || host != test.host || port != test.want {
			t.Fatalf("parseSSHAddress(%q, %d) = %q, %d, %v", test.input, test.port, host, port, err)
		}
	}
	if _, _, err := parseSSHAddress("ssh.example.test:70000", 22); err == nil {
		t.Fatal("expected invalid port error")
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

	state := NewState(&config.Config{
		Storage: config.StorageConfig{Type: "tinysql"},
		Servers: []config.Server{{Name: "localhost", Local: true}},
	}, "")
	srv := NewServer(state, ":0", store)

	req := httptest.NewRequest(http.MethodGet, "/history", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("history status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{"Metric Samples", "localhost", "HighCPU", "History summary", "All targets", `value="localhost"`} {
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
	certFileExpires := time.Date(2099, time.December, 31, 23, 59, 59, 0, time.UTC)
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
		CommandChecks:  []monitor.CommandCheckResult{{Name: "docker", OK: true}},
		HashChecks:     []monitor.HashCheckResult{{Name: "app-config", Algorithm: "sha256", OK: true}},
		CertFileChecks: []monitor.CertificateFileCheckResult{{Name: "local-cert", OK: true, ExpiresAt: &certFileExpires, ExpiresDays: 365}},
	}}, nil)
	srv := NewServer(state, ":0")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{"watchssh_up", "watchssh_cpu_usage_percent", "watchssh_memory_usage_percent", "watchssh_disk_usage_percent", "watchssh_dns_probe_up", "watchssh_tls_probe_up", "watchssh_traceroute_hops", "watchssh_command_probe_up", "watchssh_hash_probe_up", "watchssh_certificate_file_probe_up", "watchssh_certificate_file_expires_days", "watchssh_board_temperature_celsius", "watchssh_board_wifi_rssi_dbm", "watchssh_board_throttled"} {
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
