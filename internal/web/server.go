package web

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
	"github.com/SimonWaldherr/WatchSSH/internal/monitor"
	sshclient "github.com/SimonWaldherr/WatchSSH/internal/ssh"
)

// funcMap provides helper functions available inside all HTML templates.
var funcMap = template.FuncMap{
	"serverStatus":      serverStatus,
	"serverStatusLabel": serverStatusLabel,
	"pbarClass":         pbarClass,
	"clamp":             clamp,
	"fmtBytes":          fmtBytes,
	"fmtUptime":         fmtUptime,
	"timeAgo":           timeAgo,
	"rootDisk":          rootDisk,
	"cpuPct":            cpuPct,
	"memPct":            memPct,
	"loadAvg1":          loadAvg1,
	"uptimeSecs":        uptimeSecs,
	"swapPct":           swapPct,
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
	state  *State
	mux    *http.ServeMux
	listen string
}

// NewServer creates a Server backed by state, listening on addr.
func NewServer(state *State, listen string) *Server {
	s := &Server{state: state, mux: http.NewServeMux(), listen: listen}
	s.registerRoutes()
	return s
}

// Start listens on s.listen. It blocks until the server stops or an error occurs.
func (s *Server) Start() error {
	log.Printf("Web dashboard at http://%s", s.listen)
	return http.ListenAndServe(s.listen, s.mux) //nolint:gosec
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
	s.mux.HandleFunc("/server/", s.handleServerDetail)
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
	ServerName string
	Host       string
	Port       int
	Username   string
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
			ServerName: srv.Name,
			Host:       srv.Host,
			Port:       srv.Port,
			Username:   srv.Username,
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
	srv := config.Server{
		Name:     name,
		Host:     host,
		Port:     port,
		Username: r.FormValue("username"),
		Auth:     auth,
		Local:    isLocal,
		Checks: config.Checks{
			Ping: config.PingCheck{
				Enabled: pingEnabled,
				Count:   3,
				Timeout: 5,
			},
		},
	}
	s.state.AddServer(srv)
	if err := s.state.SaveConfig(); err != nil {
		log.Printf("auto-save config after AddServer: %v", err)
	}
	redirectWithFlash(w, r, "/servers", fmt.Sprintf("Server %q added.", name), false)
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
	knownHostsPath := strings.TrimSpace(r.FormValue("known_hosts_path"))

	var strictHostKey *bool
	if shk := r.FormValue("strict_host_key_checking"); shk == "true" || shk == "false" {
		b := shk == "true"
		strictHostKey = &b
	}

	s.state.UpdateGlobalSettings(interval, timeout, workers, outputType, outputFile, webListen, webEnabled, knownHostsPath, strictHostKey)

	if err := s.state.SaveConfig(); err != nil {
		redirectWithFlash(w, r, "/config", "Failed to save config: "+err.Error(), true)
		return
	}
	redirectWithFlash(w, r, "/config", "Configuration saved successfully. Restart WatchSSH for polling-interval and web-listen changes to take full effect.", false)
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
	for _, d := range m.Disks {
		if d.UsagePercent > 90 {
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
