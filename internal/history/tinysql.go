package history

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tsqldriver "github.com/SimonWaldherr/tinySQL/driver"
)

// TinySQLStore stores history in an embedded tinySQL database.
type TinySQLStore struct {
	db *sql.DB
}

// OpenTinySQL opens or creates a file-backed tinySQL database.
func OpenTinySQL(path string) (*TinySQLStore, error) {
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

	store := &TinySQLStore{db: db}
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
	return nil
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
		(id, collected_at, server_name, host, platform, has_error, payload_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	for _, r := range records {
		if _, err := tx.ExecContext(ctx, stmt, r.ID, r.CollectedAt, r.ServerName, r.Host, r.Platform, r.HasError, r.PayloadJSON); err != nil {
			return fmt.Errorf("insert metric history: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit metric history: %w", err)
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
	return nil
}

// RecentMetrics returns the newest metric samples, optionally filtered by server.
func (s *TinySQLStore) RecentMetrics(ctx context.Context, serverName string, limit int) ([]MetricRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT id, collected_at, server_name, host, platform, has_error, payload_json
		FROM metric_samples
		ORDER BY collected_at DESC
		LIMIT ?`
	args := []any{limit}
	if serverName != "" {
		query = `SELECT id, collected_at, server_name, host, platform, has_error, payload_json
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
		if err := rows.Scan(&r.ID, &r.CollectedAt, &r.ServerName, &r.Host, &r.Platform, &r.HasError, &r.PayloadJSON); err != nil {
			return nil, fmt.Errorf("scan metric history: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate metric history: %w", err)
	}
	return records, nil
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
