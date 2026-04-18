package monitor

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/SimonWaldherr/WatchSSH/internal/check"
	"github.com/SimonWaldherr/WatchSSH/internal/config"
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
	notify   NotifyFunc
	done     chan struct{}
	wg       sync.WaitGroup
}

// New returns a new Monitor. notify may be nil if no live state update is needed.
func New(cfg *config.Config, notify NotifyFunc) *Monitor {
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

	// Notify web state (if web server is running)
	if m.notify != nil {
		m.notify(results, firings)
	}
	if err := m.output.Write(results); err != nil {
		log.Printf("output error: %v", err)
	}
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
	}

	for _, pc := range srv.Checks.Ports {
		r := check.CheckPort(srv.Host, pc.Port, pc.Timeout)
		cs.Ports = append(cs.Ports, PortResult{Port: r.Port, Open: r.Open})
	}

	for _, hc := range srv.Checks.HTTP {
		r := check.CheckHTTP(hc.URL, hc.ExpectedStatus, hc.Timeout)
		cs.HTTP = append(cs.HTTP, HTTPResult{
			URL:             r.URL,
			StatusCode:      r.StatusCode,
			OK:              r.OK,
			LatencyMs:       r.LatencyMs,
			CertExpiresDays: r.CertExpiresDays,
		})
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
		cs.Ports = append(cs.Ports, PortResult{Port: r.Port, Open: r.Open})
	}

	for _, hc := range srv.Checks.HTTP {
		r := check.CheckHTTP(hc.URL, hc.ExpectedStatus, hc.Timeout)
		cs.HTTP = append(cs.HTTP, HTTPResult{
			URL:             r.URL,
			StatusCode:      r.StatusCode,
			OK:              r.OK,
			LatencyMs:       r.LatencyMs,
			CertExpiresDays: r.CertExpiresDays,
		})
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
	}

	// Load and uptime are grouped in LoadStats.
	if snap.Load != nil || snap.UptimeSecs != nil {
		ls := &LoadStats{}
		if snap.Load != nil {
			ls.Load1 = snap.Load.Load1
			ls.Load5 = snap.Load.Load5
			ls.Load15 = snap.Load.Load15
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
			Device:       d.Device,
			MountPoint:   d.MountPoint,
			TotalBytes:   d.TotalBytes,
			UsedBytes:    d.UsedBytes,
			FreeBytes:    d.FreeBytes,
			UsagePercent: d.UsagePercent,
		})
	}

	for _, n := range snap.Network {
		m.Network = append(m.Network, NetworkStats{
			Interface:   n.Interface,
			BytesRecv:   n.BytesRecv,
			BytesSent:   n.BytesSent,
			PacketsRecv: n.PacketsRecv,
			PacketsSent: n.PacketsSent,
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
	m.Capabilities = snap.Caps
	m.MetricErrors = snap.Errors
}
