package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/SimonWaldherr/WatchSSH/internal/check"
	"github.com/SimonWaldherr/WatchSSH/internal/config"
	"github.com/SimonWaldherr/WatchSSH/internal/history"
	"github.com/SimonWaldherr/WatchSSH/internal/platform"
	sshclient "github.com/SimonWaldherr/WatchSSH/internal/ssh"
)

// NotifyFunc is called after each collection cycle with the freshly gathered
// metrics and any newly-triggered alert firings.
type NotifyFunc func(metrics []ServerMetrics, firings []Firing)

// Monitor periodically collects metrics from all configured servers.
type Monitor struct {
	cfg      *config.Config
	cfgMu    sync.RWMutex // protects cfg (web UI may modify it concurrently)
	output   OutputWriter
	alertMgr *AlertManager
	store    history.Store
	notify   NotifyFunc
	done     chan struct{}
	wg       sync.WaitGroup
}

// New returns a new Monitor. notify may be nil if no live state update is needed.
func New(cfg *config.Config, notify NotifyFunc) (*Monitor, error) {
	store, err := history.New(cfg.Storage)
	if err != nil {
		return nil, err
	}
	return NewWithStore(cfg, notify, store), nil
}

// NewWithStore returns a monitor backed by an already-open history store.
func NewWithStore(cfg *config.Config, notify NotifyFunc, store history.Store) *Monitor {
	var w OutputWriter
	switch cfg.Output.Type {
	case "json":
		w = &JSONWriter{file: cfg.Output.File}
	default:
		w = &ConsoleWriter{}
	}
	return &Monitor{
		cfg:      cfg,
		output:   w,
		alertMgr: NewAlertManager(),
		store:    store,
		notify:   notify,
		done:     make(chan struct{}),
	}
}

// UpdateConfig replaces the monitor's config with a new one (safe for concurrent use).
func (m *Monitor) UpdateConfig(cfg *config.Config) {
	m.cfgMu.Lock()
	m.cfg = cfg
	m.cfgMu.Unlock()
}

// Start begins the polling loop. It runs the first collection immediately and
// then repeats every cfg.Interval seconds. Call Stop() to terminate.
func (m *Monitor) Start() {
	m.wg.Add(1)
	defer m.wg.Done()

	m.cfgMu.RLock()
	interval := time.Duration(m.cfg.Interval) * time.Second
	m.cfgMu.RUnlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	m.collect()

	for {
		select {
		case <-ticker.C:
			m.collect()
		case <-m.done:
			return
		}
	}
}

// RunOnce performs a single collection cycle and returns.
func (m *Monitor) RunOnce() {
	m.collect()
}

// Stop signals the polling loop to exit and waits for it to finish.
func (m *Monitor) Stop() {
	close(m.done)
	m.wg.Wait()
	if err := m.Close(); err != nil {
		log.Printf("history store close: %v", err)
	}
}

// Close releases resources held by the monitor.
func (m *Monitor) Close() error {
	if m.store == nil {
		return nil
	}
	return m.store.Close()
}

// collect queries all servers concurrently (bounded by cfg.Workers) and writes the aggregated results.
func (m *Monitor) collect() {
	m.cfgMu.RLock()
	cfg := m.cfg // snapshot
	m.cfgMu.RUnlock()

	var (
		mu      sync.Mutex
		results []ServerMetrics
		wg      sync.WaitGroup
	)

	// Determine worker concurrency. 0 or negative means one goroutine per server.
	workers := cfg.Workers
	if workers <= 0 {
		workers = len(cfg.Servers)
	}
	if workers < 1 {
		workers = 1
	}

	// sem is a semaphore that limits concurrent server collections.
	sem := make(chan struct{}, workers)

	for _, srv := range cfg.Servers {
		wg.Add(1)
		go func(srv config.Server) {
			defer wg.Done()
			sem <- struct{}{}        // acquire slot
			defer func() { <-sem }() // release slot
			metrics := m.collectServer(srv, cfg)
			mu.Lock()
			results = append(results, metrics)
			mu.Unlock()
		}(srv)
	}
	wg.Wait()

	// Alert evaluation
	firings := m.alertMgr.Evaluate(results, cfg)
	if len(firings) > 0 && cfg.Alerts.Email != nil {
		if err := SendAlertEmail(*cfg.Alerts.Email, firings); err != nil {
			log.Printf("alert email: %v", err)
		}
	}
	if len(firings) > 0 && cfg.Alerts.Action != nil {
		if err := RunAlertAction(*cfg.Alerts.Action, firings); err != nil {
			log.Printf("alert action: %v", err)
		}
	}
	if len(firings) > 0 && len(cfg.Alerts.Routes) > 0 {
		if err := SendAlertRoutes(cfg.Alerts.Routes, firings); err != nil {
			log.Printf("alert webhook: %v", err)
		}
	}
	if err := m.recordHistory(cfg, results, firings); err != nil {
		log.Printf("history store: %v", err)
	}

	// Notify web state (if web server is running)
	if m.notify != nil {
		m.notify(results, firings)
	}
	if err := m.output.Write(results); err != nil {
		log.Printf("output error: %v", err)
	}
}

func (m *Monitor) recordHistory(cfg *config.Config, metrics []ServerMetrics, firings []Firing) error {
	if m.store == nil || cfg.Storage.Type == "" || cfg.Storage.Type == "none" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Timeout)*time.Second)
	defer cancel()

	metricRecords, err := metricHistoryRecords(metrics)
	if err != nil {
		return err
	}
	if err := m.store.RecordMetrics(ctx, metricRecords); err != nil {
		return err
	}

	firingRecords, err := firingHistoryRecords(firings)
	if err != nil {
		return err
	}
	return m.store.RecordFirings(ctx, firingRecords)
}

func metricHistoryRecords(metrics []ServerMetrics) ([]history.MetricRecord, error) {
	records := make([]history.MetricRecord, 0, len(metrics))
	for i, m := range metrics {
		payload, err := json.Marshal(m)
		if err != nil {
			return nil, fmt.Errorf("marshal metric history: %w", err)
		}
		collectedAt := m.Timestamp.UTC().Format(time.RFC3339Nano)
		records = append(records, history.MetricRecord{
			ID:                   fmt.Sprintf("%s-%d-%s", collectedAt, i, m.ServerName),
			CollectedAt:          collectedAt,
			ServerName:           m.ServerName,
			Host:                 m.Host,
			Platform:             m.Platform,
			HasError:             m.Error != "",
			CPUUsage:             metricCPUUsage(m),
			MemoryUsage:          metricMemoryUsage(m),
			SwapUsage:            metricSwapUsage(m),
			Load1:                metricLoad1(m),
			DiskRootUsage:        metricRootDiskUsage(m),
			PingOK:               metricPingOK(m),
			PingLatencyMS:        metricPingLatency(m),
			DNSOK:                metricDNSOK(m),
			TLSCertMinDays:       metricTLSCertMinDays(m),
			TracerouteHops:       metricTracerouteHops(m),
			BoardTemperatureC:    metricBoardTemperature(m),
			BoardCPUFrequencyMHz: metricBoardCPUFrequency(m),
			BoardWiFiRSSIDbm:     metricBoardWiFiRSSI(m),
			BoardUnderVoltageNow: metricBoardUnderVoltage(m),
			BoardThrottledNow:    metricBoardThrottled(m),
			PayloadJSON:          string(payload),
		})
	}
	return records, nil
}

func metricCPUUsage(m ServerMetrics) *float64 {
	if m.CPU == nil {
		return nil
	}
	return float64Ptr(m.CPU.UsagePercent)
}

func metricMemoryUsage(m ServerMetrics) *float64 {
	if m.Memory == nil {
		return nil
	}
	return float64Ptr(m.Memory.UsagePercent)
}

func metricSwapUsage(m ServerMetrics) *float64 {
	if m.Swap == nil {
		return nil
	}
	return float64Ptr(m.Swap.Percent)
}

func metricLoad1(m ServerMetrics) *float64 {
	if m.Load == nil {
		return nil
	}
	return float64Ptr(m.Load.Load1)
}

func metricRootDiskUsage(m ServerMetrics) *float64 {
	for _, d := range m.Disks {
		if d.MountPoint == "/" {
			return float64Ptr(d.UsagePercent)
		}
	}
	if len(m.Disks) == 0 {
		return nil
	}
	return float64Ptr(m.Disks[0].UsagePercent)
}

func metricPingOK(m ServerMetrics) *bool {
	if !m.Connectivity.PingEnabled {
		return nil
	}
	return boolPtr(m.Connectivity.PingOK)
}

func metricPingLatency(m ServerMetrics) *float64 {
	if !m.Connectivity.PingEnabled || !m.Connectivity.PingOK {
		return nil
	}
	return float64Ptr(m.Connectivity.PingLatency)
}

func metricDNSOK(m ServerMetrics) *bool {
	if len(m.Connectivity.DNS) == 0 {
		return nil
	}
	ok := true
	for _, r := range m.Connectivity.DNS {
		if !r.OK {
			ok = false
			break
		}
	}
	return boolPtr(ok)
}

func metricTLSCertMinDays(m ServerMetrics) *float64 {
	var min *float64
	for _, r := range m.Connectivity.TLS {
		if r.CertExpiresDays == nil {
			continue
		}
		if min == nil || *r.CertExpiresDays < *min {
			v := *r.CertExpiresDays
			min = &v
		}
	}
	return min
}

func metricTracerouteHops(m ServerMetrics) *float64 {
	if len(m.Connectivity.Traceroute) == 0 {
		return nil
	}
	var maxHops int
	for _, r := range m.Connectivity.Traceroute {
		if r.Hops > maxHops {
			maxHops = r.Hops
		}
	}
	return float64Ptr(float64(maxHops))
}

func metricBoardTemperature(m ServerMetrics) *float64 {
	if m.Board == nil {
		return nil
	}
	return m.Board.TemperatureC
}

func metricBoardCPUFrequency(m ServerMetrics) *float64 {
	if m.Board == nil {
		return nil
	}
	return m.Board.CPUFrequencyMHz
}

func metricBoardWiFiRSSI(m ServerMetrics) *float64 {
	if m.Board == nil {
		return nil
	}
	return m.Board.WiFiRSSIDbm
}

func metricBoardUnderVoltage(m ServerMetrics) *bool {
	if m.Board == nil {
		return nil
	}
	return boolPtr(m.Board.UnderVoltageNow)
}

func metricBoardThrottled(m ServerMetrics) *bool {
	if m.Board == nil {
		return nil
	}
	return boolPtr(m.Board.ThrottledNow)
}

func float64Ptr(v float64) *float64 {
	return &v
}

func boolPtr(v bool) *bool {
	return &v
}

func firingHistoryRecords(firings []Firing) ([]history.FiringRecord, error) {
	records := make([]history.FiringRecord, 0, len(firings))
	for i, f := range firings {
		payload, err := json.Marshal(f)
		if err != nil {
			return nil, fmt.Errorf("marshal alert history: %w", err)
		}
		firedAt := f.FiredAt.UTC().Format(time.RFC3339Nano)
		records = append(records, history.FiringRecord{
			ID:          fmt.Sprintf("%s-%d-%s-%s", firedAt, i, f.Server, f.RuleName),
			FiredAt:     firedAt,
			RuleName:    f.RuleName,
			Metric:      f.Metric,
			Server:      f.Server,
			Value:       f.Value,
			Message:     f.Message,
			PayloadJSON: string(payload),
		})
	}
	return records, nil
}

// collectServer connects to / runs on a single server and gathers all metrics.
func (m *Monitor) collectServer(srv config.Server, cfg *config.Config) ServerMetrics {
	metrics := ServerMetrics{
		ServerName:    srv.Name,
		Host:          srv.Host,
		Timestamp:     time.Now(),
		SchemaVersion: schemaVersion,
	}

	timeout := time.Duration(cfg.Timeout) * time.Second

	// ── Connectivity checks (run from monitoring machine, not via SSH) ────────
	if !srv.Local {
		metrics.Connectivity = runConnectivityChecks(srv)
	} else {
		// For local server: only run port/HTTP checks (no ping to self)
		metrics.Connectivity = runLocalConnectivityChecks(srv)
	}

	// ── System metric collection (SSH or local exec) ──────────────────────────
	if srv.Local {
		r := &localRunner{}
		cmdCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := m.gatherAll(cmdCtx, r, &metrics, srv); err != nil {
			metrics.Error = err.Error()
		}
		return metrics
	}

	connCtx, cancel := context.WithTimeout(context.Background(), timeout+5*time.Second)
	defer cancel()

	client, err := sshclient.New(connCtx, srv, cfg, timeout)
	if err != nil {
		metrics.Error = err.Error()
		return metrics
	}
	defer client.Close()

	cmdCtx, cmdCancel := context.WithTimeout(context.Background(), timeout)
	defer cmdCancel()

	if err := m.gatherAll(cmdCtx, client, &metrics, srv); err != nil {
		metrics.Error = err.Error()
	}
	return metrics
}

// runConnectivityChecks runs ping, port, and HTTP checks from the local machine
// against a remote server.
func runConnectivityChecks(srv config.Server) ConnectivityStats {
	cs := ConnectivityStats{}

	if srv.Checks.Ping.Enabled {
		cs.PingEnabled = true
		result := check.Ping(srv.Host, srv.Checks.Ping.Count, srv.Checks.Ping.Timeout)
		cs.PingOK = result.OK
		cs.PingLatency = result.LatencyMs
		cs.PingLoss = result.PacketLoss
	}

	for _, pc := range srv.Checks.Ports {
		r := check.CheckPort(srv.Host, pc.Port, pc.Timeout)
		result := PortResult{Port: r.Port, Open: r.Open, LatencyMs: r.LatencyMs}
		if r.Err != nil {
			result.Error = r.Err.Error()
		}
		cs.Ports = append(cs.Ports, result)
	}

	for _, bc := range srv.Checks.Banner {
		r := check.CheckBanner(bc.Name, bc.Host, bc.Port, bc.ExpectedPrefix, bc.Timeout)
		result := BannerResult{Name: r.Name, Host: r.Host, Port: r.Port, Banner: r.Banner, OK: r.OK, LatencyMs: r.LatencyMs}
		if r.Err != nil {
			result.Error = r.Err.Error()
		}
		cs.Banner = append(cs.Banner, result)
	}

	for _, hc := range srv.Checks.HTTP {
		r := check.CheckHTTP(hc.URL, hc.ExpectedStatus, hc.Timeout)
		cs.HTTP = append(cs.HTTP, HTTPResult{
			URL:             r.URL,
			Method:          r.Method,
			StatusCode:      r.StatusCode,
			OK:              r.OK,
			LatencyMs:       r.LatencyMs,
			CertExpiresDays: r.CertExpiresDays,
			Error:           r.Error,
		})
	}

	for _, dc := range srv.Checks.DNS {
		r := check.CheckDNS(dc.Name, dc.Host, dc.Type, dc.Server, dc.ExpectedAnswer, dc.Timeout)
		result := DNSResult{
			Name:      r.Name,
			Host:      r.Host,
			Type:      r.Type,
			Server:    r.Server,
			Answers:   r.Answers,
			OK:        r.OK,
			LatencyMs: r.LatencyMs,
		}
		if r.Err != nil {
			result.Error = r.Err.Error()
		}
		cs.DNS = append(cs.DNS, result)
	}

	for _, tc := range srv.Checks.Trace {
		r := check.CheckTraceroute(tc.Name, tc.Host, tc.MaxHops, tc.Timeout)
		result := TracerouteResult{
			Name:      r.Name,
			Host:      r.Host,
			OK:        r.OK,
			Hops:      r.Hops,
			LatencyMs: r.LatencyMs,
		}
		if r.Err != nil {
			result.Error = r.Err.Error()
		}
		cs.Traceroute = append(cs.Traceroute, result)
	}

	for _, tc := range srv.Checks.TLS {
		r := check.CheckTLS(tc.Name, tc.Host, tc.Port, tc.ServerName, tc.Timeout)
		result := TLSResult{
			Name:            r.Name,
			Host:            r.Host,
			Port:            r.Port,
			ServerName:      r.ServerName,
			OK:              r.OK,
			LatencyMs:       r.LatencyMs,
			CertExpiresDays: r.CertExpiresDays,
			Issuer:          r.Issuer,
			Subject:         r.Subject,
		}
		if r.Err != nil {
			result.Error = r.Err.Error()
		}
		cs.TLS = append(cs.TLS, result)
	}

	for _, nc := range srv.Checks.NTP {
		r := check.CheckNTP(nc.Name, nc.Host, nc.Port, nc.MaxOffsetMs, nc.Timeout)
		result := NTPResult{Name: r.Name, Host: r.Host, Port: r.Port, OK: r.OK, LatencyMs: r.LatencyMs, OffsetMs: r.OffsetMs, Stratum: r.Stratum}
		if r.Err != nil {
			result.Error = r.Err.Error()
		}
		cs.NTP = append(cs.NTP, result)
	}

	return cs
}

// runLocalConnectivityChecks runs port and HTTP checks for a local server
// (ping to self is skipped).
func runLocalConnectivityChecks(srv config.Server) ConnectivityStats {
	cs := ConnectivityStats{}

	host := "127.0.0.1"
	for _, pc := range srv.Checks.Ports {
		r := check.CheckPort(host, pc.Port, pc.Timeout)
		result := PortResult{Port: r.Port, Open: r.Open, LatencyMs: r.LatencyMs}
		if r.Err != nil {
			result.Error = r.Err.Error()
		}
		cs.Ports = append(cs.Ports, result)
	}

	for _, bc := range srv.Checks.Banner {
		r := check.CheckBanner(bc.Name, bc.Host, bc.Port, bc.ExpectedPrefix, bc.Timeout)
		result := BannerResult{Name: r.Name, Host: r.Host, Port: r.Port, Banner: r.Banner, OK: r.OK, LatencyMs: r.LatencyMs}
		if r.Err != nil {
			result.Error = r.Err.Error()
		}
		cs.Banner = append(cs.Banner, result)
	}

	for _, hc := range srv.Checks.HTTP {
		r := check.CheckHTTP(hc.URL, hc.ExpectedStatus, hc.Timeout)
		cs.HTTP = append(cs.HTTP, HTTPResult{
			URL:             r.URL,
			Method:          r.Method,
			StatusCode:      r.StatusCode,
			OK:              r.OK,
			LatencyMs:       r.LatencyMs,
			CertExpiresDays: r.CertExpiresDays,
			Error:           r.Error,
		})
	}

	for _, dc := range srv.Checks.DNS {
		r := check.CheckDNS(dc.Name, dc.Host, dc.Type, dc.Server, dc.ExpectedAnswer, dc.Timeout)
		result := DNSResult{Name: r.Name, Host: r.Host, Type: r.Type, Server: r.Server, Answers: r.Answers, OK: r.OK, LatencyMs: r.LatencyMs}
		if r.Err != nil {
			result.Error = r.Err.Error()
		}
		cs.DNS = append(cs.DNS, result)
	}

	for _, tc := range srv.Checks.Trace {
		r := check.CheckTraceroute(tc.Name, tc.Host, tc.MaxHops, tc.Timeout)
		result := TracerouteResult{Name: r.Name, Host: r.Host, OK: r.OK, Hops: r.Hops, LatencyMs: r.LatencyMs}
		if r.Err != nil {
			result.Error = r.Err.Error()
		}
		cs.Traceroute = append(cs.Traceroute, result)
	}

	for _, tc := range srv.Checks.TLS {
		r := check.CheckTLS(tc.Name, tc.Host, tc.Port, tc.ServerName, tc.Timeout)
		result := TLSResult{Name: r.Name, Host: r.Host, Port: r.Port, ServerName: r.ServerName, OK: r.OK, LatencyMs: r.LatencyMs, CertExpiresDays: r.CertExpiresDays, Issuer: r.Issuer, Subject: r.Subject}
		if r.Err != nil {
			result.Error = r.Err.Error()
		}
		cs.TLS = append(cs.TLS, result)
	}

	for _, nc := range srv.Checks.NTP {
		r := check.CheckNTP(nc.Name, nc.Host, nc.Port, nc.MaxOffsetMs, nc.Timeout)
		result := NTPResult{Name: r.Name, Host: r.Host, Port: r.Port, OK: r.OK, LatencyMs: r.LatencyMs, OffsetMs: r.OffsetMs, Stratum: r.Stratum}
		if r.Err != nil {
			result.Error = r.Err.Error()
		}
		cs.NTP = append(cs.NTP, result)
	}

	return cs
}

// runner is the subset of sshclient.Client used by gatherAll.
type runner interface {
	Run(ctx context.Context, cmd string) (string, error)
}

// gatherAll detects the remote OS, selects the appropriate platform backend,
// and collects all system metrics via that backend.
func (m *Monitor) gatherAll(ctx context.Context, c runner, metrics *ServerMetrics, srv config.Server) error {
	// Detect the remote operating system family.
	family := platform.Detect(ctx, c)
	metrics.Platform = string(family)

	// Select the platform-specific collector.
	col := platform.New(family)

	// Collect metrics via the platform backend.
	snap, err := col.Collect(ctx, c)
	if err != nil {
		return err
	}

	// Optionally collect Docker container metrics (Linux-only, explicit opt-in).
	if srv.Docker.Enabled {
		platform.CollectDocker(ctx, c, family, snap)
	}

	// Map platform snapshot to monitor ServerMetrics.
	applySnapshot(snap, metrics)

	// Custom SSH command checks (platform-agnostic).
	for _, cc := range srv.Checks.Custom {
		out, cmdErr := c.Run(ctx, cc.Command)
		result := CustomCheckResult{
			Name:   cc.Name,
			Output: strings.TrimSpace(out),
			OK:     cmdErr == nil,
		}
		if cmdErr == nil && cc.ExpectedOutput != "" {
			result.OK = strings.Contains(out, cc.ExpectedOutput)
		}
		metrics.CustomChecks = append(metrics.CustomChecks, result)
	}
	return nil
}

// applySnapshot maps a platform.Snapshot to monitor.ServerMetrics.
func applySnapshot(snap *platform.Snapshot, m *ServerMetrics) {
	m.System = SystemInfo{
		OS:       snap.SystemInfo.OS,
		Kernel:   snap.SystemInfo.Kernel,
		Arch:     snap.SystemInfo.Arch,
		Hostname: snap.SystemInfo.Hostname,
		CPUCores: snap.SystemInfo.CPUCores,
	}

	// Load and uptime are grouped in LoadStats.
	if snap.Load != nil || snap.UptimeSecs != nil {
		ls := &LoadStats{}
		if snap.Load != nil {
			ls.Load1 = snap.Load.Load1
			ls.Load5 = snap.Load.Load5
			ls.Load15 = snap.Load.Load15
			ls.RunningProcesses = snap.Load.RunningProcesses
			ls.TotalProcesses = snap.Load.TotalProcesses
			ls.LastPID = snap.Load.LastPID
		}
		if snap.UptimeSecs != nil {
			ls.UptimeSeconds = *snap.UptimeSecs
		}
		m.Load = ls
	}

	if snap.CPU != nil {
		m.CPU = &CPUStats{
			UsagePercent:  snap.CPU.UsagePercent,
			UserPercent:   snap.CPU.UserPercent,
			SystemPercent: snap.CPU.SystemPercent,
			IdlePercent:   snap.CPU.IdlePercent,
			IOWaitPercent: snap.CPU.IOWaitPercent,
		}
	}

	if snap.Memory != nil {
		m.Memory = &MemoryStats{
			TotalBytes:     snap.Memory.TotalBytes,
			UsedBytes:      snap.Memory.UsedBytes,
			FreeBytes:      snap.Memory.FreeBytes,
			AvailableBytes: snap.Memory.AvailableBytes,
			UsagePercent:   snap.Memory.UsagePercent,
		}
	}

	if snap.Swap != nil {
		m.Swap = &SwapStats{
			TotalBytes: snap.Swap.TotalBytes,
			UsedBytes:  snap.Swap.UsedBytes,
			FreeBytes:  snap.Swap.FreeBytes,
			Percent:    snap.Swap.Percent,
		}
	}

	for _, d := range snap.Disks {
		m.Disks = append(m.Disks, DiskStats{
			Device:             d.Device,
			MountPoint:         d.MountPoint,
			TotalBytes:         d.TotalBytes,
			UsedBytes:          d.UsedBytes,
			FreeBytes:          d.FreeBytes,
			UsagePercent:       d.UsagePercent,
			InodesTotal:        d.InodesTotal,
			InodesUsed:         d.InodesUsed,
			InodesFree:         d.InodesFree,
			InodesUsagePercent: d.InodesUsagePercent,
		})
	}

	for _, i := range snap.Inodes {
		m.Inodes = append(m.Inodes, InodeStats{
			Device:       i.Device,
			MountPoint:   i.MountPoint,
			TotalInodes:  i.TotalInodes,
			UsedInodes:   i.UsedInodes,
			FreeInodes:   i.FreeInodes,
			UsagePercent: i.UsagePercent,
		})
	}

	for _, n := range snap.Network {
		m.Network = append(m.Network, NetworkStats{
			Interface:   n.Interface,
			BytesRecv:   n.BytesRecv,
			BytesSent:   n.BytesSent,
			PacketsRecv: n.PacketsRecv,
			PacketsSent: n.PacketsSent,
			ErrorsRecv:  n.ErrorsRecv,
			ErrorsSent:  n.ErrorsSent,
			DropsRecv:   n.DropsRecv,
			DropsSent:   n.DropsSent,
		})
	}

	for _, u := range snap.Users {
		m.Users = append(m.Users, LoggedInUser{
			User:      u.User,
			TTY:       u.TTY,
			LoginTime: u.LoginTime,
			Host:      u.Host,
		})
	}

	for _, p := range snap.Processes {
		m.Processes = append(m.Processes, ProcessInfo{
			PID:        p.PID,
			User:       p.User,
			CPUPercent: p.CPUPercent,
			MemPercent: p.MemPercent,
			RSSBytes:   p.RSSBytes,
			State:      p.State,
			Command:    p.Command,
		})
	}

	for _, c := range snap.Containers {
		m.Containers = append(m.Containers, ContainerInfo{
			ID:              c.ID,
			Name:            c.Name,
			Image:           c.Image,
			Status:          c.Status,
			CPUPercent:      c.CPUPercent,
			MemUsedBytes:    c.MemUsedBytes,
			MemLimitBytes:   c.MemLimitBytes,
			MemPercent:      c.MemPercent,
			NetRxBytes:      c.NetRxBytes,
			NetTxBytes:      c.NetTxBytes,
			BlockReadBytes:  c.BlockReadBytes,
			BlockWriteBytes: c.BlockWriteBytes,
		})
	}
	if snap.FileDescriptors != nil {
		m.FileDescriptors = &FileDescriptorStats{
			Allocated:    snap.FileDescriptors.Allocated,
			Unused:       snap.FileDescriptors.Unused,
			Max:          snap.FileDescriptors.Max,
			UsagePercent: snap.FileDescriptors.UsagePercent,
		}
	}
	if snap.Board != nil {
		m.Board = &BoardInfo{
			Model:            snap.Board.Model,
			TemperatureC:     snap.Board.TemperatureC,
			CPUFrequencyMHz:  snap.Board.CPUFrequencyMHz,
			ThrottledHex:     snap.Board.ThrottledHex,
			UnderVoltageNow:  snap.Board.UnderVoltageNow,
			ThrottledNow:     snap.Board.ThrottledNow,
			UnderVoltageSeen: snap.Board.UnderVoltageSeen,
			ThrottledSeen:    snap.Board.ThrottledSeen,
			WiFiInterface:    snap.Board.WiFiInterface,
			WiFiRSSIDbm:      snap.Board.WiFiRSSIDbm,
		}
	}
	m.Capabilities = snap.Caps
	m.MetricErrors = snap.Errors
}
