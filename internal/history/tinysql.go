package history

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
	tsqldriver "github.com/SimonWaldherr/tinySQL/driver"
)

// TinySQLStore stores history in an embedded tinySQL database.
type TinySQLStore struct {
	db            *sql.DB
	path          string
	retentionDays int
	maxSizeBytes  int64
}

// OpenTinySQL opens or creates a file-backed tinySQL database.
func OpenTinySQL(path string) (*TinySQLStore, error) {
	return OpenTinySQLWithConfig(config.StorageConfig{Path: path})
}

// OpenTinySQLWithConfig opens or creates a file-backed tinySQL database.
func OpenTinySQLWithConfig(storage config.StorageConfig) (*TinySQLStore, error) {
	path := storage.Path
	if path == "" {
		path = "watchssh.tinysql"
	}
	path = expandTilde(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("creating storage directory: %w", err)
	}

	cfg := tsqldriver.DefaultOpenConfig()
	cfg.Mode = "file"
	cfg.FilePath = path
	cfg.Tenant = "watchssh"
	cfg.Autosave = true
	cfg.BusyTimeout = 2 * time.Second
	cfg.PingTimeout = 5 * time.Second
	cfg.MaxOpenConns = 1
	cfg.MaxIdleConns = 1

	db, err := tsqldriver.OpenWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("opening tinySQL store: %w", err)
	}

	store := &TinySQLStore{
		db:            db,
		path:          path,
		retentionDays: storage.RetentionDays,
		maxSizeBytes:  int64(storage.MaxSizeMB) * 1024 * 1024,
	}
	if err := store.initSchema(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *TinySQLStore) initSchema(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS metric_samples (
			id TEXT PRIMARY KEY,
			collected_at TEXT,
			server_name TEXT,
			host TEXT,
			platform TEXT,
			has_error BOOL,
			cpu_usage FLOAT,
			memory_usage FLOAT,
			swap_usage FLOAT,
			load1 FLOAT,
			disk_root_usage FLOAT,
			ping_ok BOOL,
			ping_latency_ms FLOAT,
			dns_ok BOOL,
			tls_cert_min_days FLOAT,
			traceroute_hops FLOAT,
			payload_json TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS alert_firings (
			id TEXT PRIMARY KEY,
			fired_at TEXT,
			rule_name TEXT,
			metric TEXT,
			server TEXT,
			value FLOAT,
			message TEXT,
			payload_json TEXT
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("initializing tinySQL schema: %w", err)
		}
	}
	if err := s.ensureMetricColumns(ctx); err != nil {
		return err
	}
	return nil
}

func (s *TinySQLStore) ensureMetricColumns(ctx context.Context) error {
	stmts := []string{
		`ALTER TABLE metric_samples ADD COLUMN cpu_usage FLOAT`,
		`ALTER TABLE metric_samples ADD COLUMN memory_usage FLOAT`,
		`ALTER TABLE metric_samples ADD COLUMN swap_usage FLOAT`,
		`ALTER TABLE metric_samples ADD COLUMN load1 FLOAT`,
		`ALTER TABLE metric_samples ADD COLUMN disk_root_usage FLOAT`,
		`ALTER TABLE metric_samples ADD COLUMN ping_ok BOOL`,
		`ALTER TABLE metric_samples ADD COLUMN ping_latency_ms FLOAT`,
		`ALTER TABLE metric_samples ADD COLUMN dns_ok BOOL`,
		`ALTER TABLE metric_samples ADD COLUMN tls_cert_min_days FLOAT`,
		`ALTER TABLE metric_samples ADD COLUMN traceroute_hops FLOAT`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil && !isDuplicateColumnError(err) {
			return fmt.Errorf("migrating tinySQL schema: %w", err)
		}
	}
	return nil
}

func isDuplicateColumnError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already") || strings.Contains(msg, "duplicate") || strings.Contains(msg, "exists")
}

// RecordMetrics persists metric samples.
func (s *TinySQLStore) RecordMetrics(ctx context.Context, records []MetricRecord) error {
	if len(records) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin metric history transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	const stmt = `INSERT INTO metric_samples
		(id, collected_at, server_name, host, platform, has_error, cpu_usage, memory_usage, swap_usage, load1, disk_root_usage, ping_ok, ping_latency_ms, dns_ok, tls_cert_min_days, traceroute_hops, payload_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	for _, r := range records {
		if _, err := tx.ExecContext(ctx, stmt, r.ID, r.CollectedAt, r.ServerName, r.Host, r.Platform, r.HasError, nullableFloat(r.CPUUsage), nullableFloat(r.MemoryUsage), nullableFloat(r.SwapUsage), nullableFloat(r.Load1), nullableFloat(r.DiskRootUsage), nullableBool(r.PingOK), nullableFloat(r.PingLatencyMS), nullableBool(r.DNSOK), nullableFloat(r.TLSCertMinDays), nullableFloat(r.TracerouteHops), r.PayloadJSON); err != nil {
			return fmt.Errorf("insert metric history: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit metric history: %w", err)
	}
	if err := s.applyRetention(ctx); err != nil {
		return err
	}
	return nil
}

// RecordFirings persists alert firings.
func (s *TinySQLStore) RecordFirings(ctx context.Context, records []FiringRecord) error {
	if len(records) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin alert history transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	const stmt = `INSERT INTO alert_firings
		(id, fired_at, rule_name, metric, server, value, message, payload_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	for _, r := range records {
		if _, err := tx.ExecContext(ctx, stmt, r.ID, r.FiredAt, r.RuleName, r.Metric, r.Server, r.Value, r.Message, r.PayloadJSON); err != nil {
			return fmt.Errorf("insert alert history: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit alert history: %w", err)
	}
	if err := s.applyRetention(ctx); err != nil {
		return err
	}
	return nil
}

// RecentMetrics returns the newest metric samples, optionally filtered by server.
func (s *TinySQLStore) RecentMetrics(ctx context.Context, serverName string, limit int) ([]MetricRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT id, collected_at, server_name, host, platform, has_error, cpu_usage, memory_usage, swap_usage, load1, disk_root_usage, ping_ok, ping_latency_ms, dns_ok, tls_cert_min_days, traceroute_hops, payload_json
		FROM metric_samples
		ORDER BY collected_at DESC
		LIMIT ?`
	args := []any{limit}
	if serverName != "" {
		query = `SELECT id, collected_at, server_name, host, platform, has_error, cpu_usage, memory_usage, swap_usage, load1, disk_root_usage, ping_ok, ping_latency_ms, dns_ok, tls_cert_min_days, traceroute_hops, payload_json
			FROM metric_samples
			WHERE server_name = ?
			ORDER BY collected_at DESC
			LIMIT ?`
		args = []any{serverName, limit}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query recent metric history: %w", err)
	}
	defer rows.Close()

	var records []MetricRecord
	for rows.Next() {
		var r MetricRecord
		var cpuUsage, memoryUsage, swapUsage, load1, diskRootUsage, pingLatency, tlsCertMinDays, tracerouteHops sql.NullFloat64
		var pingOK, dnsOK sql.NullBool
		if err := rows.Scan(&r.ID, &r.CollectedAt, &r.ServerName, &r.Host, &r.Platform, &r.HasError, &cpuUsage, &memoryUsage, &swapUsage, &load1, &diskRootUsage, &pingOK, &pingLatency, &dnsOK, &tlsCertMinDays, &tracerouteHops, &r.PayloadJSON); err != nil {
			return nil, fmt.Errorf("scan metric history: %w", err)
		}
		r.CPUUsage = floatPtr(cpuUsage)
		r.MemoryUsage = floatPtr(memoryUsage)
		r.SwapUsage = floatPtr(swapUsage)
		r.Load1 = floatPtr(load1)
		r.DiskRootUsage = floatPtr(diskRootUsage)
		r.PingOK = boolPtr(pingOK)
		r.PingLatencyMS = floatPtr(pingLatency)
		r.DNSOK = boolPtr(dnsOK)
		r.TLSCertMinDays = floatPtr(tlsCertMinDays)
		r.TracerouteHops = floatPtr(tracerouteHops)
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate metric history: %w", err)
	}
	return records, nil
}

func (s *TinySQLStore) applyRetention(ctx context.Context) error {
	if s.retentionDays > 0 {
		cutoff := time.Now().UTC().Add(-time.Duration(s.retentionDays) * 24 * time.Hour).Format(time.RFC3339Nano)
		if _, err := s.db.ExecContext(ctx, `DELETE FROM metric_samples WHERE collected_at < ?`, cutoff); err != nil {
			return fmt.Errorf("apply metric age retention: %w", err)
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM alert_firings WHERE fired_at < ?`, cutoff); err != nil {
			return fmt.Errorf("apply alert age retention: %w", err)
		}
	}
	if s.maxSizeBytes > 0 {
		if err := s.trimBySize(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *TinySQLStore) trimBySize(ctx context.Context) error {
	info, err := os.Stat(s.path)
	if err != nil || info.Size() <= s.maxSizeBytes {
		return nil
	}
	for i := 0; i < 100 && info.Size() > s.maxSizeBytes; i++ {
		if err := s.deleteOldestMetric(ctx); err != nil {
			return err
		}
		if err := s.deleteOldestFiring(ctx); err != nil {
			return err
		}
		info, err = os.Stat(s.path)
		if err != nil {
			return nil
		}
	}
	return nil
}

func (s *TinySQLStore) deleteOldestMetric(ctx context.Context) error {
	var id string
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM metric_samples ORDER BY collected_at ASC LIMIT 1`).Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("query oldest metric: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM metric_samples WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete oldest metric: %w", err)
	}
	return nil
}

func (s *TinySQLStore) deleteOldestFiring(ctx context.Context) error {
	var id string
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM alert_firings ORDER BY fired_at ASC LIMIT 1`).Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("query oldest alert: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM alert_firings WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete oldest alert: %w", err)
	}
	return nil
}

func nullableFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullableBool(v *bool) any {
	if v == nil {
		return nil
	}
	return *v
}

func floatPtr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	out := v.Float64
	return &out
}

func boolPtr(v sql.NullBool) *bool {
	if !v.Valid {
		return nil
	}
	out := v.Bool
	return &out
}

// RecentFirings returns the newest alert firings.
func (s *TinySQLStore) RecentFirings(ctx context.Context, limit int) ([]FiringRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, fired_at, rule_name, metric, server, value, message, payload_json
		FROM alert_firings
		ORDER BY fired_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query recent alert history: %w", err)
	}
	defer rows.Close()

	var records []FiringRecord
	for rows.Next() {
		var r FiringRecord
		if err := rows.Scan(&r.ID, &r.FiredAt, &r.RuleName, &r.Metric, &r.Server, &r.Value, &r.Message, &r.PayloadJSON); err != nil {
			return nil, fmt.Errorf("scan alert history: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate alert history: %w", err)
	}
	return records, nil
}

// Close releases the database handle.
func (s *TinySQLStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func expandTilde(path string) string {
	if len(path) > 0 && path[0] == '~' {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}
