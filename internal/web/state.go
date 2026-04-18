// Package web implements the built-in HTTP monitoring dashboard for WatchSSH.
package web

import (
	"fmt"
	"sort"
	"sync"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
	"github.com/SimonWaldherr/WatchSSH/internal/monitor"
)

// State holds live application data shared between the monitor goroutine and
// the web request handlers. All exported methods are safe for concurrent use.
type State struct {
	mu         sync.RWMutex
	metrics    map[string]monitor.ServerMetrics // keyed by ServerName
	firings    []monitor.Firing
	cfg        *config.Config
	configPath string // path to the YAML config file used for saving
}

// NewState creates a new State backed by cfg. configPath is the YAML file path
// used when saving configuration changes via the web UI (may be empty).
func NewState(cfg *config.Config, configPath string) *State {
	return &State{
		metrics:    make(map[string]monitor.ServerMetrics),
		cfg:        cfg,
		configPath: configPath,
	}
}

// Update stores the latest collection results. It is called by the monitor
// after each poll cycle.
func (s *State) Update(batch []monitor.ServerMetrics, firings []monitor.Firing) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range batch {
		s.metrics[m.ServerName] = m
	}
	if len(firings) > 0 {
		s.firings = append(s.firings, firings...)
		if len(s.firings) > 200 {
			s.firings = s.firings[len(s.firings)-200:]
		}
	}
}

// Metrics returns a snapshot of all current per-server metrics, sorted by name.
func (s *State) Metrics() []monitor.ServerMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]monitor.ServerMetrics, 0, len(s.metrics))
	for _, m := range s.metrics {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ServerName < out[j].ServerName
	})
	return out
}

// MetricsByName returns the latest metrics for one server, and whether it was found.
func (s *State) MetricsByName(name string) (monitor.ServerMetrics, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.metrics[name]
	return m, ok
}

// Firings returns a copy of the recent alert firings.
func (s *State) Firings() []monitor.Firing {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]monitor.Firing, len(s.firings))
	copy(out, s.firings)
	return out
}

// Config returns a copy of the current configuration.
func (s *State) Config() config.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return *s.cfg
}

// AddServer appends a server to the live configuration.
func (s *State) AddServer(srv config.Server) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Servers = append(s.cfg.Servers, srv)
}

// RemoveServer removes the server with the given name from the live config and
// deletes its cached metrics.
func (s *State) RemoveServer(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := s.cfg.Servers[:0]
	for _, srv := range s.cfg.Servers {
		if srv.Name != name {
			filtered = append(filtered, srv)
		}
	}
	s.cfg.Servers = filtered
	delete(s.metrics, name)
}

// AddAlertRule appends an alert rule to the live configuration.
func (s *State) AddAlertRule(rule config.AlertRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Alerts.Rules = append(s.cfg.Alerts.Rules, rule)
}

// RemoveAlertRule removes the alert rule with the given name.
func (s *State) RemoveAlertRule(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := s.cfg.Alerts.Rules[:0]
	for _, r := range s.cfg.Alerts.Rules {
		if r.Name != name {
			filtered = append(filtered, r)
		}
	}
	s.cfg.Alerts.Rules = filtered
}

// UpdateGlobalSettings replaces the global (non-server, non-alert) config fields.
func (s *State) UpdateGlobalSettings(interval, timeout, workers int, outputType, outputFile, webListen string, webEnabled bool, knownHostsPath string, strictHostKey *bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Interval = interval
	s.cfg.Timeout = timeout
	s.cfg.Workers = workers
	s.cfg.Output.Type = outputType
	s.cfg.Output.File = outputFile
	s.cfg.Web.Enabled = webEnabled
	s.cfg.Web.Listen = webListen
	s.cfg.KnownHostsPath = knownHostsPath
	s.cfg.StrictHostKeyChecking = strictHostKey
}

// SaveConfig writes the current configuration to the config file path that
// was supplied when the State was created. Returns an error if no path is set.
func (s *State) SaveConfig() error {
	s.mu.RLock()
	cfg := *s.cfg
	path := s.configPath
	s.mu.RUnlock()
	if path == "" {
		return fmt.Errorf("no config file path configured")
	}
	return config.Save(&cfg, path)
}
