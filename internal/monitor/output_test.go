package monitor

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestConsoleWriterRefreshesInteractiveSnapshot(t *testing.T) {
	var out bytes.Buffer
	writer := &ConsoleWriter{out: &out, interactive: true}
	metrics := []ServerMetrics{{ServerName: "app-01", Timestamp: time.Now()}}
	if err := writer.Write(metrics); err != nil {
		t.Fatal(err)
	}
	if err := writer.Write(metrics); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "\x1b[") || !strings.Contains(out.String(), "\x1b[J") {
		t.Fatalf("interactive output did not clear prior snapshot: %q", out.String())
	}
}

func TestConsoleWriterAppendsNonInteractiveOutput(t *testing.T) {
	var out bytes.Buffer
	writer := &ConsoleWriter{out: &out, interactive: false}
	metrics := []ServerMetrics{{ServerName: "app-01", Timestamp: time.Now()}}
	if err := writer.Write(metrics); err != nil {
		t.Fatal(err)
	}
	if err := writer.Write(metrics); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("non-interactive output contains terminal control sequences: %q", out.String())
	}
}

func TestRenderServerMetricsShowsUnixProbeSummaryWithoutLogContent(t *testing.T) {
	output := renderServerMetrics(ServerMetrics{
		ServerName: "app-01",
		Timestamp:  time.Now(),
		FileChecks: []FileCheckResult{{Name: "pid", SizeBytes: 12, AgeSeconds: 3, OK: true}},
		LogChecks:  []LogCheckResult{{Name: "errors", Pattern: "super-secret-log-line", Count: 1, OK: false}},
	}, 100)
	if !strings.Contains(output, "Unix SSH probes:") || !strings.Contains(output, "pid") || !strings.Contains(output, "errors") {
		t.Fatalf("Unix probe summary missing: %s", output)
	}
	if strings.Contains(output, "super-secret-log-line") {
		t.Fatalf("console output exposed log pattern: %s", output)
	}
}
