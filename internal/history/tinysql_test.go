package history

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
	tsqldriver "github.com/SimonWaldherr/tinySQL/driver"
)

func TestTinySQLStoreRecordsHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.tinysql")
	store, err := OpenTinySQL(path)
	if err != nil {
		t.Fatalf("OpenTinySQL() error = %v", err)
	}

	ctx := context.Background()
	cpuUsage := 12.5
	memUsage := 43.2
	boardTemp := 51.2
	boardThrottled := true
	if err := store.RecordMetrics(ctx, []MetricRecord{{
		ID:                "metric-1",
		CollectedAt:       "2026-07-08T12:00:00Z",
		ServerName:        "localhost",
		Host:              "127.0.0.1",
		Platform:          "Linux",
		HasError:          false,
		CPUUsage:          &cpuUsage,
		MemoryUsage:       &memUsage,
		BoardTemperatureC: &boardTemp,
		BoardThrottledNow: &boardThrottled,
		PayloadJSON:       `{"server_name":"localhost"}`,
	}}); err != nil {
		t.Fatalf("RecordMetrics() error = %v", err)
	}

	if err := store.RecordFirings(ctx, []FiringRecord{{
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

	assertCount(t, store.db, "metric_samples", 1)
	assertCount(t, store.db, "alert_firings", 1)
	assertHistoryIndexes(t, store.db)

	metrics, err := store.RecentMetrics(ctx, "localhost", 10)
	if err != nil {
		t.Fatalf("RecentMetrics() error = %v", err)
	}
	if len(metrics) != 1 || metrics[0].ServerName != "localhost" {
		t.Fatalf("RecentMetrics() = %#v, want localhost record", metrics)
	}
	if metrics[0].CPUUsage == nil || *metrics[0].CPUUsage != cpuUsage {
		t.Fatalf("RecentMetrics()[0].CPUUsage = %v, want %v", metrics[0].CPUUsage, cpuUsage)
	}
	if metrics[0].BoardTemperatureC == nil || *metrics[0].BoardTemperatureC != boardTemp {
		t.Fatalf("RecentMetrics()[0].BoardTemperatureC = %v, want %v", metrics[0].BoardTemperatureC, boardTemp)
	}
	if metrics[0].BoardThrottledNow == nil || *metrics[0].BoardThrottledNow != boardThrottled {
		t.Fatalf("RecentMetrics()[0].BoardThrottledNow = %v, want %v", metrics[0].BoardThrottledNow, boardThrottled)
	}

	firings, err := store.RecentFirings(ctx, 10)
	if err != nil {
		t.Fatalf("RecentFirings() error = %v", err)
	}
	if len(firings) != 1 || firings[0].RuleName != "HighCPU" {
		t.Fatalf("RecentFirings() = %#v, want HighCPU record", firings)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	store, err = OpenTinySQL(path)
	if err != nil {
		t.Fatalf("OpenTinySQL() after close error = %v", err)
	}

	assertCount(t, store.db, "metric_samples", 1)
	assertCount(t, store.db, "alert_firings", 1)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() after reopen error = %v", err)
	}
}

func assertHistoryIndexes(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sys.indexes`)
	if err != nil {
		t.Fatalf("querying tinySQL indexes: %v", err)
	}
	defer rows.Close()

	got := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scanning tinySQL index: %v", err)
		}
		got[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating tinySQL indexes: %v", err)
	}
	for _, want := range []string{
		"metric_samples_server_name_idx",
		"metric_samples_collected_at_idx",
		"alert_firings_fired_at_idx",
	} {
		if !got[want] {
			t.Fatalf("tinySQL indexes = %#v, missing %q", got, want)
		}
	}
}

func TestTinySQLStoreRetentionDays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.tinysql")
	store, err := OpenTinySQLWithConfig(config.StorageConfig{Path: path, RetentionDays: 1})
	if err != nil {
		t.Fatalf("OpenTinySQLWithConfig() error = %v", err)
	}
	defer store.Close()

	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	if err := store.RecordMetrics(context.Background(), []MetricRecord{{
		ID:          "old-metric",
		CollectedAt: old,
		ServerName:  "localhost",
		PayloadJSON: `{}`,
	}}); err != nil {
		t.Fatalf("RecordMetrics() error = %v", err)
	}

	records, err := store.RecentMetrics(context.Background(), "", 10)
	if err != nil {
		t.Fatalf("RecentMetrics() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("RecentMetrics() len = %d, want 0 after retention", len(records))
	}
}

func TestRetryTinySQLWriteRetriesOnlyTransactionConflicts(t *testing.T) {
	attempts := 0
	err := retryTinySQLWrite(context.Background(), func() error {
		attempts++
		if attempts < 3 {
			return tsqldriver.ErrTransactionConflict
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retryTinySQLWrite() error = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}

	attempts = 0
	want := errors.New("write failed")
	err = retryTinySQLWrite(context.Background(), func() error {
		attempts++
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("retryTinySQLWrite() error = %v, want %v", err, want)
	}
	if attempts != 1 {
		t.Fatalf("non-conflict attempts = %d, want 1", attempts)
	}
}

func assertCount(t *testing.T, db *sql.DB, table string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", table, got, want)
	}
}
