package history

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestTinySQLStoreRecordsHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.tinysql")
	store, err := OpenTinySQL(path)
	if err != nil {
		t.Fatalf("OpenTinySQL() error = %v", err)
	}

	ctx := context.Background()
	if err := store.RecordMetrics(ctx, []MetricRecord{{
		ID:          "metric-1",
		CollectedAt: "2026-07-08T12:00:00Z",
		ServerName:  "localhost",
		Host:        "127.0.0.1",
		Platform:    "Linux",
		HasError:    false,
		PayloadJSON: `{"server_name":"localhost"}`,
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

	metrics, err := store.RecentMetrics(ctx, "localhost", 10)
	if err != nil {
		t.Fatalf("RecentMetrics() error = %v", err)
	}
	if len(metrics) != 1 || metrics[0].ServerName != "localhost" {
		t.Fatalf("RecentMetrics() = %#v, want localhost record", metrics)
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
