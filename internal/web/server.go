package web

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
	"github.com/SimonWaldherr/WatchSSH/internal/history"
	"github.com/SimonWaldherr/WatchSSH/internal/monitor"
	sshclient "github.com/SimonWaldherr/WatchSSH/internal/ssh"
)

// funcMap provides helper functions available inside all HTML templates.
var funcMap = template.FuncMap{
	"serverStatus":         serverStatus,
	"serverStatusLabel":    serverStatusLabel,
	"pbarClass":            pbarClass,
	"clamp":                clamp,
	"fmtBytes":             fmtBytes,
	"fmtOptBool":           fmtOptBool,
	"fmtOptFloat":          fmtOptFloat,
	"fmtUptime":            fmtUptime,
	"timeAgo":              timeAgo,
	"rootDisk":             rootDisk,
	"cpuPct":               cpuPct,
	"memPct":               memPct,
	"loadAvg1":             loadAvg1,
	"uptimeSecs":           uptimeSecs,
	"swapPct":              swapPct,
	"fdInUse":              fdInUse,
	"netErrors":            netErrors,
	"netDrops":             netDrops,
	"dockerSummary":        dockerSummary,
	"hasDockerDiagnostics": hasDockerDiagnostics,
	"metricCapability":     metricCapability,
	"metricError":          metricError,
	"statusClass":          statusClass,
	"capabilityRows":       capabilityRows,
	"not": func(v any) bool {
		if v == nil {
			return true
		}
		switch val := v.(type) {
		case bool:
			return !val
		case string:
			return val == ""
		case int:
			return val == 0
		}
		return false
	},
	"derefBool": func(b *bool, def bool) bool {
		if b == nil {
			return def
		}
		return *b
	},
}

// templates is the shared template set, parsed once at package init.
var templates = template.Must(
	template.New("").Funcs(funcMap).Parse(allTemplates),
)

// Server is the HTTP monitoring dashboard.
type Server struct {
	state   *State
	mux     *http.ServeMux
	listen  string
	history history.Store
}

// NewServer creates a Server backed by state, listening on addr.
func NewServer(state *State, listen string, stores ...history.Store) *Server {
	s := &Server{state: state, mux: http.NewServeMux(), listen: listen}
	if len(stores) > 0 {
		s.history = stores[0]
	}
	s.registerRoutes()
	return s
}

// Start listens on s.listen. It blocks until the server stops or an error occurs.
func (s *Server) Start() error {
	log.Printf("Web dashboard at http://%s", s.listen)
	return http.ListenAndServe(s.listen, s.mux) //nolint:gosec
}

type capabilityRow struct {
	Name   string
	Status string
	Error  string
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/static/style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=300")
		fmt.Fprint(w, css)
	})
	s.mux.HandleFunc("/healthz", s.handleHealthz)
	s.mux.HandleFunc("/readyz", s.handleReadyz)
	s.mux.HandleFunc("/api/test-connection", s.handleTestConnection)
	s.mux.HandleFunc("/api/metrics", s.handleAPIMetrics)
	s.mux.HandleFunc("/api/probes", s.handleAPIProbes)
	s.mux.HandleFunc("/api/history/metrics", s.handleAPIHistoryMetrics)
	s.mux.HandleFunc("/api/history/alerts", s.handleAPIHistoryAlerts)
	s.mux.HandleFunc("/api/history/summary", s.handleAPIHistorySummary)
	s.mux.HandleFunc("/metrics", s.handlePrometheusMetrics)
	s.mux.HandleFunc("/server/", s.handleServerDetail)
	s.mux.HandleFunc("/history", s.handleHistory)
	s.mux.HandleFunc("/servers/add", s.handleAddServer)
	s.mux.HandleFunc("/servers/remove", s.handleRemoveServer)
	s.mux.HandleFunc("/servers", s.handleServers)
	s.mux.HandleFunc("/alerts/add", s.handleAddAlert)
	s.mux.HandleFunc("/alerts/remove", s.handleRemoveAlert)
	s.mux.HandleFunc("/alerts", s.handleAlerts)
	s.mux.HandleFunc("/config", s.handleConfig)
	s.mux.HandleFunc("/", s.handleDashboard)
}

// render executes a named template from the shared template set.
func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template %q error: %v", name, err)
		http.Error(w, "internal template error", http.StatusInternalServerError)
	}
}

// ── Dashboard ─────────────────────────────────────────────────────────────────

type dashboardData struct {
	Title    string
	Page     string
	Refresh  bool
	Flash    string
	FlashErr bool
	Servers  []monitor.ServerMetrics
	Firings  []monitor.Firing
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	firings := s.state.Firings()
	// Show only firings from the last 24 h on the dashboard.
	cutoff := time.Now().Add(-24 * time.Hour)
	recent := firings[:0]
	for _, f := range firings {
		if f.FiredAt.After(cutoff) {
			recent = append(recent, f)
		}
	}
	// Reverse so newest first.
	for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
		recent[i], recent[j] = recent[j], recent[i]
	}
	s.render(w, "dashboard", dashboardData{
		Title:   "Dashboard",
		Page:    "dashboard",
		Refresh: true,
		Servers: s.state.Metrics(),
		Firings: recent,
	})
}

// ── History ───────────────────────────────────────────────────────────────────

type historyData struct {
	Title          string
	Page           string
	Refresh        bool
	StorageEnabled bool
	MetricSamples  []history.MetricRecord
	AlertFirings   []history.FiringRecord
	ServerFilter   string
	Error          string
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	cfg := s.state.Config()
	data := historyData{
		Title:          "History",
		Page:           "history",
		StorageEnabled: cfg.Storage.Type == "tinysql" && s.history != nil,
		ServerFilter:   strings.TrimSpace(r.URL.Query().Get("server")),
	}
	if !data.StorageEnabled {
		s.render(w, "history-page", data)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	metrics, err := s.history.RecentMetrics(ctx, data.ServerFilter, 100)
	if err != nil {
		data.Error = err.Error()
		s.render(w, "history-page", data)
		return
	}
	firings, err := s.history.RecentFirings(ctx, 100)
	if err != nil {
		data.Error = err.Error()
		s.render(w, "history-page", data)
		return
	}
	data.MetricSamples = metrics
	data.AlertFirings = firings
	s.render(w, "history-page", data)
}

// ── Server detail ─────────────────────────────────────────────────────────────

type serverDetailData struct {
	Title   string
	Page    string
	Refresh bool
	Metrics monitor.ServerMetrics
}

func (s *Server) handleServerDetail(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/server/")
	if name == "" {
		http.Redirect(w, r, "/servers", http.StatusFound)
		return
	}
	m, ok := s.state.MetricsByName(name)
	if !ok {
		http.Error(w, "server not found", http.StatusNotFound)
		return
	}
	s.render(w, "server-detail", serverDetailData{
		Title:   m.ServerName,
		Page:    "dashboard",
		Refresh: true,
		Metrics: m,
	})
}

// ── Server management ─────────────────────────────────────────────────────────

type serversData struct {
	Title    string
	Page     string
	Refresh  bool
	Flash    string
	FlashErr bool
	Servers  []serverRow
}

type serverRow struct {
	ServerName    string
	Host          string
	Port          int
	Username      string
	Tags          []string
	CheckSummary  string
	DockerEnabled bool
	monitor.ServerMetrics
}

func (s *Server) handleServers(w http.ResponseWriter, r *http.Request) {
	metrics := s.state.Metrics()
	cfg := s.state.Config()

	metricsMap := make(map[string]monitor.ServerMetrics, len(metrics))
	for _, m := range metrics {
		metricsMap[m.ServerName] = m
	}

	rows := make([]serverRow, 0, len(cfg.Servers))
	for _, srv := range cfg.Servers {
		row := serverRow{
			ServerName:    srv.Name,
			Host:          srv.Host,
			Port:          srv.Port,
			Username:      srv.Username,
			Tags:          srv.Tags,
			CheckSummary:  serverCheckSummary(srv),
			DockerEnabled: srv.Docker.Enabled,
		}
		if m, ok := metricsMap[srv.Name]; ok {
			row.ServerMetrics = m
		} else {
			row.ServerMetrics = monitor.ServerMetrics{ServerName: srv.Name, Host: srv.Host}
		}
		rows = append(rows, row)
	}

	flash, flashErr := flashFromQuery(r)
	s.render(w, "servers-manage", serversData{
		Title:    "Servers",
		Page:     "servers",
		Flash:    flash,
		FlashErr: flashErr,
		Servers:  rows,
	})
}

func (s *Server) handleAddServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/servers", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		redirectWithFlash(w, r, "/servers", "Invalid form data: "+err.Error(), true)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		redirectWithFlash(w, r, "/servers", "Name is required.", true)
		return
	}

	isLocal := r.FormValue("local") == "1"
	host := strings.TrimSpace(r.FormValue("host"))
	if !isLocal && host == "" {
		redirectWithFlash(w, r, "/servers", "Host is required for non-local servers.", true)
		return
	}

	port, _ := strconv.Atoi(r.FormValue("port"))
	if port == 0 {
		port = 22
	}

	authType := r.FormValue("auth_type")
	cred := r.FormValue("auth_credential")

	auth := config.Auth{Type: config.AuthType(authType)}
	switch config.AuthType(authType) {
	case config.AuthTypeKey:
		auth.KeyFile = cred
	case config.AuthTypePassword:
		auth.Password = cred
	}

	pingEnabled := r.FormValue("ping") == "1"
	profile := strings.TrimSpace(r.FormValue("profile"))
	tags := splitList(r.FormValue("tags"))
	checks := checksFromServerForm(r)
	if pingEnabled {
		checks.Ping = config.PingCheck{Enabled: true, Count: formInt(r, "ping_count", 3), Timeout: formInt(r, "ping_timeout", 5)}
	}
	if profile != "" {
		applyServerProfile(profile, host, &tags, &checks, &isLocal)
	}
	srv := config.Server{
		Name:     name,
		Host:     host,
		Port:     port,
		Username: r.FormValue("username"),
		Auth:     auth,
		Local:    isLocal,
		Tags:     tags,
		Checks:   checks,
		Docker:   config.DockerConfig{Enabled: r.FormValue("docker_enabled") == "1"},
	}
	if srv.Local {
		srv.Host = ""
		srv.Port = 0
		srv.Username = ""
		srv.Auth = config.Auth{}
	}
	s.state.AddServer(srv)
	if err := s.state.SaveConfig(); err != nil {
		log.Printf("auto-save config after AddServer: %v", err)
	}
	redirectWithFlash(w, r, "/servers", fmt.Sprintf("Server %q added.", name), false)
}

func checksFromServerForm(r *http.Request) config.Checks {
	httpStatus := formInt(r, "http_expected_status", 200)
	httpTimeout := formInt(r, "http_timeout", 10)
	httpMethod := strings.ToUpper(defaultString(strings.TrimSpace(r.FormValue("http_method")), http.MethodGet))
	httpExpectedBody := strings.TrimSpace(r.FormValue("http_expected_body"))
	dnsType := strings.TrimSpace(r.FormValue("dns_type"))
	if dnsType == "" {
		dnsType = "A"
	}
	dnsTimeout := formInt(r, "dns_timeout", 5)
	tlsTimeout := formInt(r, "tls_timeout", 5)
	traceTimeout := formInt(r, "traceroute_timeout", 10)

	checks := config.Checks{}
	for _, port := range parsePorts(r.FormValue("ports")) {
		checks.Ports = append(checks.Ports, config.PortCheck{Port: port, Timeout: formInt(r, "port_timeout", 5)})
	}
	for _, rawURL := range splitList(r.FormValue("http_urls")) {
		checks.HTTP = append(checks.HTTP, config.HTTPCheck{URL: rawURL, Method: httpMethod, ExpectedStatus: httpStatus, ExpectedBody: httpExpectedBody, Timeout: httpTimeout})
	}
	for _, dnsHost := range splitList(r.FormValue("dns_hosts")) {
		checks.DNS = append(checks.DNS, config.DNSCheck{
			Name:           dnsHost,
			Host:           dnsHost,
			Type:           dnsType,
			Server:         strings.TrimSpace(r.FormValue("dns_server")),
			ExpectedAnswer: strings.TrimSpace(r.FormValue("dns_expected_answer")),
			Timeout:        dnsTimeout,
		})
	}
	for _, traceHost := range splitList(r.FormValue("traceroute_hosts")) {
		checks.Trace = append(checks.Trace, config.TracerouteCheck{Name: traceHost, Host: traceHost, MaxHops: formInt(r, "traceroute_max_hops", 30), Timeout: traceTimeout})
	}
	for _, tlsHost := range splitList(r.FormValue("tls_hosts")) {
		checks.TLS = append(checks.TLS, config.TLSCheck{
			Name:       tlsHost,
			Host:       tlsHost,
			Port:       formInt(r, "tls_port", 443),
			ServerName: defaultString(strings.TrimSpace(r.FormValue("tls_server_name")), tlsHost),
			Timeout:    tlsTimeout,
		})
	}
	for _, ntpHost := range splitList(r.FormValue("ntp_hosts")) {
		checks.NTP = append(checks.NTP, config.NTPCheck{
			Name:        ntpHost,
			Host:        ntpHost,
			Port:        formInt(r, "ntp_port", 123),
			MaxOffsetMs: formFloat(r, "ntp_max_offset_ms", 0),
			Timeout:     formInt(r, "ntp_timeout", 5),
		})
	}
	customName := strings.TrimSpace(r.FormValue("custom_name"))
	customCommand := strings.TrimSpace(r.FormValue("custom_command"))
	if customName != "" && customCommand != "" {
		checks.Custom = append(checks.Custom, config.CustomCheck{
			Name:           customName,
			Command:        customCommand,
			ExpectedOutput: strings.TrimSpace(r.FormValue("custom_expected_output")),
		})
	}
	return checks
}

func applyServerProfile(profile, host string, tags *[]string, checks *config.Checks, isLocal *bool) {
	switch profile {
	case "local":
		*isLocal = true
		addTags(tags, "local")
	case "web":
		addTags(tags, "web")
		addPort(checks, 80, 5)
		addPort(checks, 443, 5)
		if host != "" {
			addHTTP(checks, "https://"+host+"/health", 200, 10)
			addDNS(checks, host, "A", "", "", 5)
			addTLS(checks, host, 443, host, 5)
		}
	case "harp":
		addTags(tags, "harp", "reverse-proxy")
		addPort(checks, 80, 5)
		addPort(checks, 443, 5)
		if host != "" {
			addHTTP(checks, "https://"+host+"/health", 200, 5)
			addHTTP(checks, "https://"+host+"/readyz", 200, 5)
			addHTTP(checks, "https://"+host+"/metrics", 200, 5)
			addDNS(checks, host, "A", "1.1.1.1", "", 5)
			addTLS(checks, host, 443, host, 5)
		}
	case "raspberry-pi":
		addTags(tags, "raspberry-pi", "sbc")
		if !checks.Ping.Enabled {
			checks.Ping = config.PingCheck{Enabled: true, Count: 3, Timeout: 5}
		}
	}
}

func serverCheckSummary(srv config.Server) string {
	var parts []string
	if srv.Checks.Ping.Enabled {
		parts = append(parts, "ping")
	}
	if len(srv.Checks.Ports) > 0 {
		parts = append(parts, fmt.Sprintf("%d ports", len(srv.Checks.Ports)))
	}
	if len(srv.Checks.HTTP) > 0 {
		parts = append(parts, fmt.Sprintf("%d http", len(srv.Checks.HTTP)))
	}
	if len(srv.Checks.DNS) > 0 {
		parts = append(parts, fmt.Sprintf("%d dns", len(srv.Checks.DNS)))
	}
	if len(srv.Checks.TLS) > 0 {
		parts = append(parts, fmt.Sprintf("%d tls", len(srv.Checks.TLS)))
	}
	if len(srv.Checks.Trace) > 0 {
		parts = append(parts, fmt.Sprintf("%d trace", len(srv.Checks.Trace)))
	}
	if len(srv.Checks.NTP) > 0 {
		parts = append(parts, fmt.Sprintf("%d ntp", len(srv.Checks.NTP)))
	}
	if len(srv.Checks.Custom) > 0 {
		parts = append(parts, fmt.Sprintf("%d custom", len(srv.Checks.Custom)))
	}
	if srv.Docker.Enabled {
		parts = append(parts, "docker")
	}
	if len(parts) == 0 {
		return "system metrics"
	}
	return strings.Join(parts, ", ")
}

func splitList(input string) []string {
	fields := strings.FieldsFunc(input, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ';'
	})
	out := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		v := strings.TrimSpace(field)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func parsePorts(input string) []int {
	parts := splitList(input)
	ports := make([]int, 0, len(parts))
	seen := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		port, err := strconv.Atoi(part)
		if err != nil || port < 1 || port > 65535 {
			continue
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		ports = append(ports, port)
	}
	return ports
}

func formInt(r *http.Request, name string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(r.FormValue(name)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func formFloat(r *http.Request, name string, fallback float64) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(r.FormValue(name)), 64)
	if err != nil || value < 0 {
		return fallback
	}
	return value
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func addTags(tags *[]string, values ...string) {
	existing := make(map[string]struct{}, len(*tags)+len(values))
	for _, tag := range *tags {
		existing[tag] = struct{}{}
	}
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := existing[value]; ok {
			continue
		}
		*tags = append(*tags, value)
		existing[value] = struct{}{}
	}
}

func addPort(checks *config.Checks, port int, timeout int) {
	for _, p := range checks.Ports {
		if p.Port == port {
			return
		}
	}
	checks.Ports = append(checks.Ports, config.PortCheck{Port: port, Timeout: timeout})
}

func addHTTP(checks *config.Checks, rawURL string, status int, timeout int) {
	for _, h := range checks.HTTP {
		if h.URL == rawURL {
			return
		}
	}
	checks.HTTP = append(checks.HTTP, config.HTTPCheck{URL: rawURL, ExpectedStatus: status, Timeout: timeout})
}

func addDNS(checks *config.Checks, host string, typ string, server string, expected string, timeout int) {
	for _, d := range checks.DNS {
		if d.Host == host && d.Type == typ && d.Server == server {
			return
		}
	}
	checks.DNS = append(checks.DNS, config.DNSCheck{Name: host, Host: host, Type: typ, Server: server, ExpectedAnswer: expected, Timeout: timeout})
}

func addTLS(checks *config.Checks, host string, port int, serverName string, timeout int) {
	for _, t := range checks.TLS {
		if t.Host == host && t.Port == port && t.ServerName == serverName {
			return
		}
	}
	checks.TLS = append(checks.TLS, config.TLSCheck{Name: host, Host: host, Port: port, ServerName: serverName, Timeout: timeout})
}

func (s *Server) handleRemoveServer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/servers", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		redirectWithFlash(w, r, "/servers", "Invalid form data.", true)
		return
	}
	name := r.FormValue("name")
	s.state.RemoveServer(name)
	if err := s.state.SaveConfig(); err != nil {
		log.Printf("auto-save config after RemoveServer: %v", err)
	}
	redirectWithFlash(w, r, "/servers", fmt.Sprintf("Server %q removed.", name), false)
}

// ── Alert management ──────────────────────────────────────────────────────────

type alertsData struct {
	Title       string
	Page        string
	Refresh     bool
	Flash       string
	FlashErr    bool
	Firings     []monitor.Firing
	Rules       []config.AlertRule
	EmailCfg    *config.EmailConfig
	ServerNames []string
}

func (s *Server) handleAlerts(w http.ResponseWriter, r *http.Request) {
	cfg := s.state.Config()
	firings := s.state.Firings()
	// Newest first.
	for i, j := 0, len(firings)-1; i < j; i, j = i+1, j-1 {
		firings[i], firings[j] = firings[j], firings[i]
	}
	serverNames := make([]string, 0, len(cfg.Servers))
	for _, srv := range cfg.Servers {
		serverNames = append(serverNames, srv.Name)
	}
	flash, flashErr := flashFromQuery(r)
	s.render(w, "alerts-page", alertsData{
		Title:       "Alerts",
		Page:        "alerts",
		Flash:       flash,
		FlashErr:    flashErr,
		Firings:     firings,
		Rules:       cfg.Alerts.Rules,
		EmailCfg:    cfg.Alerts.Email,
		ServerNames: serverNames,
	})
}

func (s *Server) handleAddAlert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/alerts", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		redirectWithFlash(w, r, "/alerts", "Invalid form data: "+err.Error(), true)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	metric := r.FormValue("metric")
	operator := r.FormValue("operator")
	if name == "" || metric == "" {
		redirectWithFlash(w, r, "/alerts", "Name and metric are required.", true)
		return
	}

	threshold, _ := strconv.ParseFloat(r.FormValue("threshold"), 64)
	port, _ := strconv.Atoi(r.FormValue("port"))

	// Accept both multi-value (from <select multiple>) and comma-separated text.
	var servers []string
	if sel := r.Form["servers"]; len(sel) > 0 {
		for _, s := range sel {
			if t := strings.TrimSpace(s); t != "" {
				servers = append(servers, t)
			}
		}
	} else if sv := strings.TrimSpace(r.FormValue("servers_text")); sv != "" {
		for _, s := range strings.Split(sv, ",") {
			if t := strings.TrimSpace(s); t != "" {
				servers = append(servers, t)
			}
		}
	}

	rule := config.AlertRule{
		Name:       name,
		Metric:     metric,
		Operator:   operator,
		Threshold:  threshold,
		MountPoint: r.FormValue("mount_point"),
		Port:       port,
		Servers:    servers,
	}
	s.state.AddAlertRule(rule)
	if err := s.state.SaveConfig(); err != nil {
		log.Printf("auto-save config after AddAlertRule: %v", err)
	}
	redirectWithFlash(w, r, "/alerts", fmt.Sprintf("Alert rule %q added.", name), false)
}

func (s *Server) handleRemoveAlert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/alerts", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		redirectWithFlash(w, r, "/alerts", "Invalid form data.", true)
		return
	}
	name := r.FormValue("name")
	s.state.RemoveAlertRule(name)
	if err := s.state.SaveConfig(); err != nil {
		log.Printf("auto-save config after RemoveAlertRule: %v", err)
	}
	redirectWithFlash(w, r, "/alerts", fmt.Sprintf("Alert rule %q removed.", name), false)
}

// ── SSH connection test ───────────────────────────────────────────────────────

// handleTestConnection accepts a JSON body describing a server and attempts to
// open and immediately close an SSH connection. It returns JSON
// {"ok":true/false,"message":"..."} and always responds with HTTP 200 so the
// browser can read the body even on failure.
func (s *Server) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	type request struct {
		Host       string `json:"host"`
		Port       int    `json:"port"`
		Username   string `json:"username"`
		AuthType   string `json:"auth_type"`
		Credential string `json:"credential"`
		Local      bool   `json:"local"`
	}
	type response struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	writeJSON := func(ok bool, msg string) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(response{OK: ok, Message: msg})
	}

	var req request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(false, "invalid request body: "+err.Error())
		return
	}

	if req.Local {
		writeJSON(true, "Local server — no SSH connection needed.")
		return
	}

	if req.Host == "" {
		writeJSON(false, "Host is required.")
		return
	}
	if req.Port == 0 {
		req.Port = 22
	}

	auth := config.Auth{Type: config.AuthType(req.AuthType)}
	switch config.AuthType(req.AuthType) {
	case config.AuthTypeKey:
		auth.KeyFile = req.Credential
	case config.AuthTypePassword:
		auth.Password = req.Credential
	}

	srv := config.Server{
		Name:     req.Host,
		Host:     req.Host,
		Port:     req.Port,
		Username: req.Username,
		Auth:     auth,
	}

	globalCfg := s.state.Config()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	cl, err := sshclient.New(ctx, srv, &globalCfg, 10*time.Second)
	if err != nil {
		writeJSON(false, "Connection failed: "+err.Error())
		return
	}
	cl.Close()
	writeJSON(true, fmt.Sprintf("Successfully connected to %s:%d as %s.", req.Host, req.Port, req.Username))
}

// ── Global configuration editor ───────────────────────────────────────────────

type configPageData struct {
	Title    string
	Page     string
	Refresh  bool
	Flash    string
	FlashErr bool
	Config   config.Config
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.handleConfigSave(w, r)
		return
	}
	cfg := s.state.Config()
	flash, flashErr := flashFromQuery(r)
	s.render(w, "config-page", configPageData{
		Title:    "Configuration",
		Page:     "config",
		Flash:    flash,
		FlashErr: flashErr,
		Config:   cfg,
	})
}

func (s *Server) handleConfigSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		redirectWithFlash(w, r, "/config", "Invalid form data: "+err.Error(), true)
		return
	}

	interval, _ := strconv.Atoi(r.FormValue("interval"))
	if interval <= 0 {
		interval = 60
	}
	timeout, _ := strconv.Atoi(r.FormValue("timeout"))
	if timeout <= 0 {
		timeout = 30
	}
	workers, _ := strconv.Atoi(r.FormValue("workers"))

	webEnabled := r.FormValue("web_enabled") == "1"
	webListen := strings.TrimSpace(r.FormValue("web_listen"))
	if webListen == "" {
		webListen = ":8080"
	}

	outputType := r.FormValue("output_type")
	if outputType == "" {
		outputType = "console"
	}
	outputFile := strings.TrimSpace(r.FormValue("output_file"))
	storageType := r.FormValue("storage_type")
	if storageType == "" {
		storageType = "none"
	}
	storagePath := strings.TrimSpace(r.FormValue("storage_path"))
	if storageType == "tinysql" && storagePath == "" {
		storagePath = "watchssh.tinysql"
	}
	storageRetentionDays, _ := strconv.Atoi(r.FormValue("storage_retention_days"))
	if storageType == "tinysql" && storageRetentionDays == 0 {
		storageRetentionDays = 30
	}
	storageMaxSizeMB, _ := strconv.Atoi(r.FormValue("storage_max_size_mb"))
	knownHostsPath := strings.TrimSpace(r.FormValue("known_hosts_path"))

	var strictHostKey *bool
	if shk := r.FormValue("strict_host_key_checking"); shk == "true" || shk == "false" {
		b := shk == "true"
		strictHostKey = &b
	}

	s.state.UpdateGlobalSettings(interval, timeout, workers, outputType, outputFile, storageType, storagePath, storageRetentionDays, storageMaxSizeMB, webListen, webEnabled, knownHostsPath, strictHostKey)

	if err := s.state.SaveConfig(); err != nil {
		redirectWithFlash(w, r, "/config", "Failed to save config: "+err.Error(), true)
		return
	}
	redirectWithFlash(w, r, "/config", "Configuration saved successfully. Restart WatchSSH for polling-interval, storage, and web-listen changes to take full effect.", false)
}

// ── JSON API ──────────────────────────────────────────────────────────────────

func (s *Server) handleAPIMetrics(w http.ResponseWriter, r *http.Request) {
	_ = r
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	metrics := s.state.Metrics()
	// Use json encoder for streaming
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(metrics)
}

type probeAPIResult struct {
	Server       string                    `json:"server"`
	Host         string                    `json:"host"`
	Connectivity monitor.ConnectivityStats `json:"connectivity"`
}

func (s *Server) handleAPIProbes(w http.ResponseWriter, r *http.Request) {
	serverFilter := strings.TrimSpace(r.URL.Query().Get("server"))
	results := make([]probeAPIResult, 0)
	for _, m := range s.state.Metrics() {
		if serverFilter != "" && m.ServerName != serverFilter {
			continue
		}
		results = append(results, probeAPIResult{
			Server:       m.ServerName,
			Host:         m.Host,
			Connectivity: m.Connectivity,
		})
	}
	writeJSON(w, http.StatusOK, results)
}

func (s *Server) handleAPIHistoryMetrics(w http.ResponseWriter, r *http.Request) {
	if !s.historyEnabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "history storage is not enabled"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	records, err := s.history.RecentMetrics(ctx, strings.TrimSpace(r.URL.Query().Get("server")), parseLimit(r, 100))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (s *Server) handleAPIHistoryAlerts(w http.ResponseWriter, r *http.Request) {
	if !s.historyEnabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "history storage is not enabled"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	records, err := s.history.RecentFirings(ctx, parseLimit(r, 100))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, records)
}

type historySummary struct {
	ServerName      string   `json:"server_name"`
	Samples         int      `json:"samples"`
	LatestAt        string   `json:"latest_at"`
	LatestCPU       *float64 `json:"latest_cpu_usage,omitempty"`
	LatestMemory    *float64 `json:"latest_memory_usage,omitempty"`
	LatestDiskRoot  *float64 `json:"latest_disk_root_usage,omitempty"`
	LatestDNSOK     *bool    `json:"latest_dns_ok,omitempty"`
	LatestTLSDays   *float64 `json:"latest_tls_cert_min_days,omitempty"`
	LatestTraceHops *float64 `json:"latest_traceroute_hops,omitempty"`
	LatestBoardTemp *float64 `json:"latest_board_temperature_c,omitempty"`
	LatestBoardRSSI *float64 `json:"latest_board_wifi_rssi_dbm,omitempty"`
	AverageCPU      *float64 `json:"average_cpu_usage,omitempty"`
	AverageMemory   *float64 `json:"average_memory_usage,omitempty"`
	AverageDiskRoot *float64 `json:"average_disk_root_usage,omitempty"`
	Errors          int      `json:"errors"`
}

func (s *Server) handleAPIHistorySummary(w http.ResponseWriter, r *http.Request) {
	if !s.historyEnabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "history storage is not enabled"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	records, err := s.history.RecentMetrics(ctx, strings.TrimSpace(r.URL.Query().Get("server")), parseLimit(r, 500))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, summarizeHistory(records))
}

func (s *Server) handlePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	_ = r
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	var b strings.Builder
	b.WriteString("# HELP watchssh_up Whether WatchSSH collected the host successfully.\n")
	b.WriteString("# TYPE watchssh_up gauge\n")
	b.WriteString("# HELP watchssh_cpu_usage_percent CPU usage percent.\n")
	b.WriteString("# TYPE watchssh_cpu_usage_percent gauge\n")
	b.WriteString("# HELP watchssh_memory_usage_percent Memory usage percent.\n")
	b.WriteString("# TYPE watchssh_memory_usage_percent gauge\n")
	b.WriteString("# HELP watchssh_disk_usage_percent Disk usage percent by mount point.\n")
	b.WriteString("# TYPE watchssh_disk_usage_percent gauge\n")
	b.WriteString("# HELP watchssh_dns_probe_up Whether a DNS probe succeeded.\n")
	b.WriteString("# TYPE watchssh_dns_probe_up gauge\n")
	b.WriteString("# HELP watchssh_tcp_probe_up Whether a TCP port probe succeeded.\n")
	b.WriteString("# TYPE watchssh_tcp_probe_up gauge\n")
	b.WriteString("# HELP watchssh_tcp_probe_latency_ms TCP port probe latency in milliseconds.\n")
	b.WriteString("# TYPE watchssh_tcp_probe_latency_ms gauge\n")
	b.WriteString("# HELP watchssh_http_probe_up Whether an HTTP probe succeeded.\n")
	b.WriteString("# TYPE watchssh_http_probe_up gauge\n")
	b.WriteString("# HELP watchssh_http_probe_latency_ms HTTP probe latency in milliseconds.\n")
	b.WriteString("# TYPE watchssh_http_probe_latency_ms gauge\n")
	b.WriteString("# HELP watchssh_dns_probe_latency_ms DNS probe latency in milliseconds.\n")
	b.WriteString("# TYPE watchssh_dns_probe_latency_ms gauge\n")
	b.WriteString("# HELP watchssh_tls_probe_up Whether a TLS probe succeeded.\n")
	b.WriteString("# TYPE watchssh_tls_probe_up gauge\n")
	b.WriteString("# HELP watchssh_tls_cert_expires_days TLS certificate days until expiry.\n")
	b.WriteString("# TYPE watchssh_tls_cert_expires_days gauge\n")
	b.WriteString("# HELP watchssh_traceroute_probe_up Whether a traceroute probe succeeded.\n")
	b.WriteString("# TYPE watchssh_traceroute_probe_up gauge\n")
	b.WriteString("# HELP watchssh_traceroute_hops Observed traceroute hop count.\n")
	b.WriteString("# TYPE watchssh_traceroute_hops gauge\n")
	b.WriteString("# HELP watchssh_ntp_probe_up Whether an NTP probe succeeded.\n")
	b.WriteString("# TYPE watchssh_ntp_probe_up gauge\n")
	b.WriteString("# HELP watchssh_ntp_probe_latency_ms NTP probe latency in milliseconds.\n")
	b.WriteString("# TYPE watchssh_ntp_probe_latency_ms gauge\n")
	b.WriteString("# HELP watchssh_ntp_offset_ms NTP clock offset in milliseconds.\n")
	b.WriteString("# TYPE watchssh_ntp_offset_ms gauge\n")
	b.WriteString("# HELP watchssh_board_temperature_celsius Board temperature in Celsius for Raspberry Pi and compatible SBCs.\n")
	b.WriteString("# TYPE watchssh_board_temperature_celsius gauge\n")
	b.WriteString("# HELP watchssh_board_cpu_frequency_mhz Current board CPU frequency in MHz.\n")
	b.WriteString("# TYPE watchssh_board_cpu_frequency_mhz gauge\n")
	b.WriteString("# HELP watchssh_board_wifi_rssi_dbm Wi-Fi RSSI in dBm from /proc/net/wireless.\n")
	b.WriteString("# TYPE watchssh_board_wifi_rssi_dbm gauge\n")
	b.WriteString("# HELP watchssh_board_under_voltage Whether the board is currently under-voltage throttled.\n")
	b.WriteString("# TYPE watchssh_board_under_voltage gauge\n")
	b.WriteString("# HELP watchssh_board_throttled Whether the board is currently throttled.\n")
	b.WriteString("# TYPE watchssh_board_throttled gauge\n")
	for _, m := range s.state.Metrics() {
		labels := prometheusLabels(map[string]string{"server": m.ServerName, "host": m.Host, "platform": m.Platform})
		up := 1
		if m.Error != "" {
			up = 0
		}
		fmt.Fprintf(&b, "watchssh_up%s %d\n", labels, up)
		if m.CPU != nil {
			fmt.Fprintf(&b, "watchssh_cpu_usage_percent%s %.6f\n", labels, m.CPU.UsagePercent)
		}
		if m.Memory != nil {
			fmt.Fprintf(&b, "watchssh_memory_usage_percent%s %.6f\n", labels, m.Memory.UsagePercent)
		}
		for _, d := range m.Disks {
			diskLabels := prometheusLabels(map[string]string{"server": m.ServerName, "host": m.Host, "mount": d.MountPoint, "device": d.Device})
			fmt.Fprintf(&b, "watchssh_disk_usage_percent%s %.6f\n", diskLabels, d.UsagePercent)
		}
		for _, p := range m.Connectivity.Ports {
			probeLabels := prometheusLabels(map[string]string{"server": m.ServerName, "host": m.Host, "port": strconv.Itoa(p.Port)})
			fmt.Fprintf(&b, "watchssh_tcp_probe_up%s %d\n", probeLabels, boolGauge(p.Open))
			fmt.Fprintf(&b, "watchssh_tcp_probe_latency_ms%s %.6f\n", probeLabels, p.LatencyMs)
		}
		for _, h := range m.Connectivity.HTTP {
			probeLabels := prometheusLabels(map[string]string{"server": m.ServerName, "target": h.URL, "method": h.Method})
			fmt.Fprintf(&b, "watchssh_http_probe_up%s %d\n", probeLabels, boolGauge(h.OK))
			fmt.Fprintf(&b, "watchssh_http_probe_latency_ms%s %.6f\n", probeLabels, h.LatencyMs)
		}
		for _, d := range m.Connectivity.DNS {
			probeLabels := prometheusLabels(map[string]string{"server": m.ServerName, "probe": d.Name, "target": d.Host, "type": d.Type, "resolver": d.Server})
			fmt.Fprintf(&b, "watchssh_dns_probe_up%s %d\n", probeLabels, boolGauge(d.OK))
			fmt.Fprintf(&b, "watchssh_dns_probe_latency_ms%s %.6f\n", probeLabels, d.LatencyMs)
		}
		for _, t := range m.Connectivity.TLS {
			probeLabels := prometheusLabels(map[string]string{"server": m.ServerName, "probe": t.Name, "target": t.Host, "server_name": t.ServerName})
			fmt.Fprintf(&b, "watchssh_tls_probe_up%s %d\n", probeLabels, boolGauge(t.OK))
			if t.CertExpiresDays != nil {
				fmt.Fprintf(&b, "watchssh_tls_cert_expires_days%s %.6f\n", probeLabels, *t.CertExpiresDays)
			}
		}
		for _, t := range m.Connectivity.Traceroute {
			probeLabels := prometheusLabels(map[string]string{"server": m.ServerName, "probe": t.Name, "target": t.Host})
			fmt.Fprintf(&b, "watchssh_traceroute_probe_up%s %d\n", probeLabels, boolGauge(t.OK))
			fmt.Fprintf(&b, "watchssh_traceroute_hops%s %d\n", probeLabels, t.Hops)
		}
		for _, n := range m.Connectivity.NTP {
			probeLabels := prometheusLabels(map[string]string{"server": m.ServerName, "probe": n.Name, "target": n.Host, "port": strconv.Itoa(n.Port), "stratum": strconv.Itoa(n.Stratum)})
			fmt.Fprintf(&b, "watchssh_ntp_probe_up%s %d\n", probeLabels, boolGauge(n.OK))
			fmt.Fprintf(&b, "watchssh_ntp_probe_latency_ms%s %.6f\n", probeLabels, n.LatencyMs)
			fmt.Fprintf(&b, "watchssh_ntp_offset_ms%s %.6f\n", probeLabels, n.OffsetMs)
		}
		if m.Board != nil {
			boardLabels := prometheusLabels(map[string]string{"server": m.ServerName, "host": m.Host, "model": m.Board.Model})
			if m.Board.TemperatureC != nil {
				fmt.Fprintf(&b, "watchssh_board_temperature_celsius%s %.6f\n", boardLabels, *m.Board.TemperatureC)
			}
			if m.Board.CPUFrequencyMHz != nil {
				fmt.Fprintf(&b, "watchssh_board_cpu_frequency_mhz%s %.6f\n", boardLabels, *m.Board.CPUFrequencyMHz)
			}
			if m.Board.WiFiRSSIDbm != nil {
				wifiLabels := prometheusLabels(map[string]string{"server": m.ServerName, "host": m.Host, "model": m.Board.Model, "interface": m.Board.WiFiInterface})
				fmt.Fprintf(&b, "watchssh_board_wifi_rssi_dbm%s %.6f\n", wifiLabels, *m.Board.WiFiRSSIDbm)
			}
			fmt.Fprintf(&b, "watchssh_board_under_voltage%s %d\n", boardLabels, boolGauge(m.Board.UnderVoltageNow))
			fmt.Fprintf(&b, "watchssh_board_throttled%s %d\n", boardLabels, boolGauge(m.Board.ThrottledNow))
		}
	}
	_, _ = w.Write([]byte(b.String()))
}

func summarizeHistory(records []history.MetricRecord) []historySummary {
	type acc struct {
		summary                      historySummary
		cpuSum, memSum, dskSum       float64
		cpuCount, memCount, dskCount int
	}
	byServer := make(map[string]*acc)
	for _, r := range records {
		a := byServer[r.ServerName]
		if a == nil {
			a = &acc{summary: historySummary{
				ServerName:      r.ServerName,
				LatestAt:        r.CollectedAt,
				LatestCPU:       r.CPUUsage,
				LatestMemory:    r.MemoryUsage,
				LatestDiskRoot:  r.DiskRootUsage,
				LatestDNSOK:     r.DNSOK,
				LatestTLSDays:   r.TLSCertMinDays,
				LatestTraceHops: r.TracerouteHops,
				LatestBoardTemp: r.BoardTemperatureC,
				LatestBoardRSSI: r.BoardWiFiRSSIDbm,
			}}
			byServer[r.ServerName] = a
		}
		a.summary.Samples++
		if r.HasError {
			a.summary.Errors++
		}
		if r.CPUUsage != nil {
			a.cpuSum += *r.CPUUsage
			a.cpuCount++
		}
		if r.MemoryUsage != nil {
			a.memSum += *r.MemoryUsage
			a.memCount++
		}
		if r.DiskRootUsage != nil {
			a.dskSum += *r.DiskRootUsage
			a.dskCount++
		}
	}
	out := make([]historySummary, 0, len(byServer))
	for _, a := range byServer {
		if a.cpuCount > 0 {
			a.summary.AverageCPU = float64Ptr(a.cpuSum / float64(a.cpuCount))
		}
		if a.memCount > 0 {
			a.summary.AverageMemory = float64Ptr(a.memSum / float64(a.memCount))
		}
		if a.dskCount > 0 {
			a.summary.AverageDiskRoot = float64Ptr(a.dskSum / float64(a.dskCount))
		}
		out = append(out, a.summary)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ServerName < out[j].ServerName })
	return out
}

func (s *Server) historyEnabled() bool {
	cfg := s.state.Config()
	return cfg.Storage.Type == "tinysql" && s.history != nil
}

func prometheusLabels(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		if labels[k] == "" {
			continue
		}
		parts = append(parts, k+`="`+prometheusEscape(labels[k])+`"`)
	}
	if len(parts) == 0 {
		return ""
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func prometheusEscape(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, "\n", `\n`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	return v
}

func boolGauge(v bool) int {
	if v {
		return 1
	}
	return 0
}

func float64Ptr(v float64) *float64 {
	return &v
}

func parseLimit(r *http.Request, fallback int) int {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 {
		return fallback
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	_ = r
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	_ = r
	cfg := s.state.Config()
	metrics := s.state.Metrics()
	ready := len(cfg.Servers) > 0 && len(metrics) > 0
	missing := len(cfg.Servers) - len(metrics)
	if missing < 0 {
		missing = 0
	}
	status := http.StatusOK
	state := "ready"
	if !ready {
		status = http.StatusServiceUnavailable
		state = "not_ready"
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":          state,
		"configured":      len(cfg.Servers),
		"with_data":       len(metrics),
		"missing_metrics": missing,
	})
}

// ── Template helper functions ─────────────────────────────────────────────────

func serverStatus(m monitor.ServerMetrics) string {
	if m.Error != "" {
		return "error"
	}
	if m.Timestamp.IsZero() {
		return "unknown"
	}
	if (m.CPU != nil && m.CPU.UsagePercent > 90) || (m.Memory != nil && m.Memory.UsagePercent > 90) {
		return "warn"
	}
	if m.FileDescriptors != nil && m.FileDescriptors.UsagePercent > 90 {
		return "warn"
	}
	for _, d := range m.Disks {
		if d.UsagePercent > 90 || d.InodesUsagePercent > 90 {
			return "warn"
		}
	}
	if m.Connectivity.PingEnabled && !m.Connectivity.PingOK {
		return "error"
	}
	for _, p := range m.Connectivity.Ports {
		if !p.Open {
			return "warn"
		}
	}
	for _, h := range m.Connectivity.HTTP {
		if !h.OK {
			return "warn"
		}
	}
	for _, d := range m.Connectivity.DNS {
		if !d.OK {
			return "warn"
		}
	}
	for _, t := range m.Connectivity.Traceroute {
		if !t.OK {
			return "warn"
		}
	}
	for _, t := range m.Connectivity.TLS {
		if !t.OK {
			return "warn"
		}
	}
	for _, n := range m.Connectivity.NTP {
		if !n.OK {
			return "warn"
		}
	}
	for _, c := range m.CustomChecks {
		if !c.OK {
			return "warn"
		}
	}
	return "ok"
}

func serverStatusLabel(m monitor.ServerMetrics) string {
	switch serverStatus(m) {
	case "error":
		return "ERROR"
	case "warn":
		return "WARN"
	case "unknown":
		return "UNKNOWN"
	default:
		return "OK"
	}
}

func pbarClass(pct float64) string {
	if pct >= 90 {
		return "error"
	}
	if pct >= 75 {
		return "warn"
	}
	return ""
}

func clamp(pct float64) float64 {
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

func fmtBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func fmtOptFloat(v *float64) string {
	if v == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.1f", *v)
}

func fmtOptBool(v *bool) string {
	if v == nil {
		return "n/a"
	}
	if *v {
		return "ok"
	}
	return "failed"
}

func fmtUptime(sec float64) string {
	d := time.Duration(sec) * time.Second
	days := int(d.Hours()) / 24
	d -= time.Duration(days*24) * time.Hour
	if days > 0 {
		return fmt.Sprintf("%dd %s", days, d.Round(time.Second))
	}
	return d.Round(time.Second).String()
}

func timeAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return t.Format("2006-01-02 15:04")
	}
}

func rootDisk(disks []monitor.DiskStats) *monitor.DiskStats {
	for i, d := range disks {
		if d.MountPoint == "/" {
			return &disks[i]
		}
	}
	if len(disks) > 0 {
		return &disks[0]
	}
	return nil
}

// ── Flash message helpers ─────────────────────────────────────────────────────

func redirectWithFlash(w http.ResponseWriter, r *http.Request, target, msg string, isErr bool) {
	params := url.Values{"flash": {msg}}
	if isErr {
		params.Set("flash_err", "1")
	}
	http.Redirect(w, r, target+"?"+params.Encode(), http.StatusSeeOther)
}

func flashFromQuery(r *http.Request) (string, bool) {
	msg := r.URL.Query().Get("flash")
	isErr := r.URL.Query().Get("flash_err") == "1"
	return msg, isErr
}

// ── Nil-safe metric accessor helpers for templates ─────────────────────────

// cpuPct returns the CPU usage percent, or 0 if the metric is unavailable.
func cpuPct(m monitor.ServerMetrics) float64 {
	if m.CPU == nil {
		return 0
	}
	return m.CPU.UsagePercent
}

// memPct returns the memory usage percent, or 0 if the metric is unavailable.
func memPct(m monitor.ServerMetrics) float64 {
	if m.Memory == nil {
		return 0
	}
	return m.Memory.UsagePercent
}

// loadAvg1 returns the 1-minute load average, or 0 if unavailable.
func loadAvg1(m monitor.ServerMetrics) float64 {
	if m.Load == nil {
		return 0
	}
	return m.Load.Load1
}

// uptimeSecs returns the uptime in seconds, or 0 if unavailable.
func uptimeSecs(m monitor.ServerMetrics) float64 {
	if m.Load == nil {
		return 0
	}
	return m.Load.UptimeSeconds
}

// swapPct returns the swap usage percent, or 0 if unavailable.
func swapPct(m monitor.ServerMetrics) float64 {
	if m.Swap == nil {
		return 0
	}
	return m.Swap.Percent
}

func fdInUse(fd *monitor.FileDescriptorStats) int64 {
	if fd == nil {
		return 0
	}
	used := fd.Allocated - fd.Unused
	if used < 0 {
		return 0
	}
	return used
}

func netErrors(n monitor.NetworkStats) int64 {
	return n.ErrorsRecv + n.ErrorsSent
}

func netDrops(n monitor.NetworkStats) int64 {
	return n.DropsRecv + n.DropsSent
}

func hasDockerDiagnostics(m monitor.ServerMetrics) bool {
	return len(m.Containers) > 0 || metricCapability(m, "containers") != "" || metricError(m, "containers") != ""
}

func dockerSummary(m monitor.ServerMetrics) string {
	switch status := metricCapability(m, "containers"); {
	case len(m.Containers) > 0:
		return fmt.Sprintf("%d running", len(m.Containers))
	case status == "ok":
		return "0 running"
	case status != "":
		return status
	default:
		return "off"
	}
}

func metricCapability(m monitor.ServerMetrics, name string) string {
	if m.Capabilities == nil {
		return ""
	}
	return m.Capabilities[name]
}

func metricError(m monitor.ServerMetrics, name string) string {
	if m.MetricErrors == nil {
		return ""
	}
	return m.MetricErrors[name]
}

func statusClass(status string) string {
	switch status {
	case "ok":
		return "ok"
	case "error":
		return "error"
	case "unavailable":
		return "warn"
	default:
		return "unknown"
	}
}

func capabilityRows(m monitor.ServerMetrics) []capabilityRow {
	keys := make(map[string]struct{}, len(m.Capabilities)+len(m.MetricErrors))
	for name := range m.Capabilities {
		keys[name] = struct{}{}
	}
	for name := range m.MetricErrors {
		keys[name] = struct{}{}
	}
	if len(keys) == 0 {
		return nil
	}
	names := make([]string, 0, len(keys))
	for name := range keys {
		names = append(names, name)
	}
	sort.Strings(names)

	rows := make([]capabilityRow, 0, len(names))
	for _, name := range names {
		status := metricCapability(m, name)
		if status == "" {
			status = "unavailable"
		}
		rows = append(rows, capabilityRow{
			Name:   name,
			Status: status,
			Error:  metricError(m, name),
		})
	}
	return rows
}
