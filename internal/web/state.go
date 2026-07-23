// Package web implements the built-in HTTP monitoring dashboard for WatchSSH.
package web

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
	"github.com/SimonWaldherr/WatchSSH/internal/monitor"
)

// State holds live application data shared between the monitor goroutine and
// the web request handlers. All exported methods are safe for concurrent use.
type State struct {
	mu         sync.RWMutex
	metrics    map[string]monitor.ServerMetrics // keyed by ServerName
	firings    []monitor.Firing
	reviews    []RunbookReview
	changes    []ChangeEvent
	audits     map[string][]AuditSnapshot
	cfg        *config.Config
	configPath string // path to the YAML config file used for saving
}

// AuditSnapshot stores a bounded in-memory audit and its diff against the
// preceding audit for the same server.
type AuditSnapshot struct {
	CollectedAt     time.Time
	Result          monitor.AuditResult
	AddedUsers      []string
	RemovedUsers    []string
	AddedPackages   []string
	RemovedPackages []string
}

// RunbookReview is an operator-owned review item created from an AI advisor
// recommendation. Its state never triggers command execution.
type RunbookReview struct {
	ID        string
	Server    string
	Rule      string
	Runbook   string
	Summary   string
	CreatedAt time.Time
	Status    string // pending, acknowledged, declined, completed
	Actor     string
	Note      string
}

// ChangeEvent records an operational change for alert correlation.
type ChangeEvent struct {
	ID        string
	Server    string
	Kind      string
	Summary   string
	Actor     string
	StartedAt time.Time
}

// NewState creates a new State backed by cfg. configPath is the YAML file path
// used when saving configuration changes via the web UI (may be empty).
func NewState(cfg *config.Config, configPath string) *State {
	return &State{
		metrics:    make(map[string]monitor.ServerMetrics),
		audits:     make(map[string][]AuditSnapshot),
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
		if m.Audit != nil {
			s.recordAuditLocked(m.ServerName, *m.Audit, m.Timestamp)
		}
	}
	if len(firings) > 0 {
		s.firings = append(s.firings, firings...)
		if len(s.firings) > 200 {
			s.firings = s.firings[len(s.firings)-200:]
		}
		for _, firing := range firings {
			if firing.Watchdog == nil {
				continue
			}
			for _, runbook := range firing.Watchdog.RecommendedRemediations {
				s.reviews = append(s.reviews, RunbookReview{ID: fmt.Sprintf("review-%d", time.Now().UnixNano()), Server: firing.Server, Rule: firing.RuleName, Runbook: runbook, Summary: firing.Watchdog.Summary, CreatedAt: time.Now(), Status: "pending"})
			}
		}
		if len(s.reviews) > 200 {
			s.reviews = s.reviews[len(s.reviews)-200:]
		}
	}
}

// RecordAudit saves an on-demand audit result and exposes it as the target's
// latest audit fact. It does not modify configuration or execute remediation.
func (s *State) RecordAudit(server string, result monitor.AuditResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordAuditLocked(server, result, time.Now())
	if metric, ok := s.metrics[server]; ok {
		metric.Audit = &result
		s.metrics[server] = metric
	}
}

func (s *State) recordAuditLocked(server string, result monitor.AuditResult, collectedAt time.Time) {
	snapshot := AuditSnapshot{CollectedAt: collectedAt, Result: result}
	if previous := s.audits[server]; len(previous) > 0 {
		prior := previous[len(previous)-1].Result
		snapshot.AddedUsers, snapshot.RemovedUsers = auditUserDiff(prior.Users, result.Users)
		snapshot.AddedPackages, snapshot.RemovedPackages = auditStringDiff(prior.Packages, result.Packages)
	}
	s.audits[server] = append(s.audits[server], snapshot)
	if len(s.audits[server]) > 20 {
		s.audits[server] = s.audits[server][len(s.audits[server])-20:]
	}
}

func auditUserDiff(before, after []monitor.AuditUser) ([]string, []string) {
	prior, current := make([]string, 0, len(before)), make([]string, 0, len(after))
	for _, user := range before {
		prior = append(prior, fmt.Sprintf("%s (%d)", user.Name, user.UID))
	}
	for _, user := range after {
		current = append(current, fmt.Sprintf("%s (%d)", user.Name, user.UID))
	}
	return auditStringDiff(prior, current)
}

func auditStringDiff(before, after []string) ([]string, []string) {
	left, right := make(map[string]struct{}, len(before)), make(map[string]struct{}, len(after))
	for _, value := range before {
		left[value] = struct{}{}
	}
	for _, value := range after {
		right[value] = struct{}{}
	}
	added, removed := []string{}, []string{}
	for value := range right {
		if _, ok := left[value]; !ok {
			added = append(added, value)
		}
	}
	for value := range left {
		if _, ok := right[value]; !ok {
			removed = append(removed, value)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

// AuditHistory returns newest-first audit snapshots for a target.
func (s *State) AuditHistory(server string) []AuditSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]AuditSnapshot(nil), s.audits[server]...)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// RunbookReviews returns a newest-first snapshot of operator review items.
func (s *State) RunbookReviews() []RunbookReview {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]RunbookReview(nil), s.reviews...)
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// UpdateRunbookReview records an operator decision. It never runs a command.
func (s *State) UpdateRunbookReview(id, status, actor, note string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.reviews {
		if s.reviews[i].ID == id {
			s.reviews[i].Status, s.reviews[i].Actor, s.reviews[i].Note = status, actor, note
			return true
		}
	}
	return false
}

// RecordChange adds a user-declared change event for correlation in the UI.
func (s *State) RecordChange(server, kind, summary, actor string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.changes = append(s.changes, ChangeEvent{ID: fmt.Sprintf("change-%d", time.Now().UnixNano()), Server: server, Kind: kind, Summary: summary, Actor: actor, StartedAt: time.Now()})
	if len(s.changes) > 200 {
		s.changes = s.changes[len(s.changes)-200:]
	}
}

// Changes returns a newest-first snapshot of declared operational changes.
func (s *State) Changes() []ChangeEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]ChangeEvent(nil), s.changes...)
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out
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

// UpdateServer applies an in-place configuration change to one named server.
// The callback runs while the state lock is held and must not block.
func (s *State) UpdateServer(name string, update func(*config.Server)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.cfg.Servers {
		if s.cfg.Servers[i].Name == name {
			update(&s.cfg.Servers[i])
			return true
		}
	}
	return false
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
func (s *State) UpdateGlobalSettings(interval, timeout, workers int, outputType, outputFile, storageType, storagePath string, storageRetentionDays, storageMaxSizeMB int, webListen string, webEnabled bool, knownHostsPath string, strictHostKey *bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Interval = interval
	s.cfg.Timeout = timeout
	s.cfg.Workers = workers
	s.cfg.Output.Type = outputType
	s.cfg.Output.File = outputFile
	s.cfg.Storage.Type = storageType
	s.cfg.Storage.Path = storagePath
	s.cfg.Storage.RetentionDays = storageRetentionDays
	s.cfg.Storage.MaxSizeMB = storageMaxSizeMB
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
