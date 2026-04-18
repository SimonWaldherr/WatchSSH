package monitor

import (
	"testing"
	"time"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
)

func makeMetrics(name string, cpu, mem float64) ServerMetrics {
	return ServerMetrics{
		ServerName: name,
		Host:       "192.0.2.1",
		Timestamp:  time.Now(),
		CPU:        &CPUStats{UsagePercent: cpu},
		Memory:     &MemoryStats{UsagePercent: mem},
	}
}

func TestAlertManager_Evaluate_TriggersOnThreshold(t *testing.T) {
	am := NewAlertManager()
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Rules: []config.AlertRule{
				{Name: "HighCPU", Metric: "cpu_usage", Operator: ">", Threshold: 80},
			},
			Cooldown: 0,
		},
	}

	metrics := []ServerMetrics{makeMetrics("srv1", 95, 50)}
	firings := am.Evaluate(metrics, cfg)
	if len(firings) != 1 {
		t.Fatalf("expected 1 firing, got %d", len(firings))
	}
	if firings[0].Server != "srv1" {
		t.Errorf("firing server = %q; want srv1", firings[0].Server)
	}
	if firings[0].Metric != "cpu_usage" {
		t.Errorf("firing metric = %q; want cpu_usage", firings[0].Metric)
	}
}

func TestAlertManager_Evaluate_NoTriggerBelowThreshold(t *testing.T) {
	am := NewAlertManager()
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Rules: []config.AlertRule{
				{Name: "HighCPU", Metric: "cpu_usage", Operator: ">", Threshold: 80},
			},
		},
	}
	metrics := []ServerMetrics{makeMetrics("srv1", 70, 50)}
	firings := am.Evaluate(metrics, cfg)
	if len(firings) != 0 {
		t.Errorf("expected 0 firings, got %d", len(firings))
	}
}

func TestAlertManager_Cooldown(t *testing.T) {
	am := NewAlertManager()
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Rules: []config.AlertRule{
				{Name: "HighCPU", Metric: "cpu_usage", Operator: ">", Threshold: 80},
			},
			Cooldown: 3600, // 1 hour
		},
	}
	metrics := []ServerMetrics{makeMetrics("srv1", 95, 50)}

	first := am.Evaluate(metrics, cfg)
	if len(first) != 1 {
		t.Fatalf("first evaluation: expected 1 firing, got %d", len(first))
	}
	second := am.Evaluate(metrics, cfg)
	if len(second) != 0 {
		t.Errorf("second evaluation within cooldown: expected 0 firings, got %d", len(second))
	}
}

func TestAlertManager_ServerFilter(t *testing.T) {
	am := NewAlertManager()
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Rules: []config.AlertRule{
				{Name: "HighCPU", Metric: "cpu_usage", Operator: ">", Threshold: 80, Servers: []string{"web-01"}},
			},
			Cooldown: 0,
		},
	}
	metrics := []ServerMetrics{
		makeMetrics("web-01", 95, 50), // should fire
		makeMetrics("db-01", 95, 50),  // should NOT fire (different server)
	}
	firings := am.Evaluate(metrics, cfg)
	if len(firings) != 1 {
		t.Fatalf("expected 1 firing, got %d", len(firings))
	}
	if firings[0].Server != "web-01" {
		t.Errorf("expected firing for web-01, got %q", firings[0].Server)
	}
}

func TestAlertManager_NilCPU(t *testing.T) {
	// Ensure alert rules don't fire when CPU metric is nil (e.g. first poll).
	am := NewAlertManager()
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Rules: []config.AlertRule{
				{Name: "HighCPU", Metric: "cpu_usage", Operator: ">", Threshold: 80},
			},
			Cooldown: 0,
		},
	}
	metrics := []ServerMetrics{{
		ServerName: "srv1",
		Timestamp:  time.Now(),
		CPU:        nil, // unavailable
	}}
	firings := am.Evaluate(metrics, cfg)
	if len(firings) != 0 {
		t.Errorf("expected 0 firings for nil CPU, got %d", len(firings))
	}
}

func TestAlertManager_DiskUsage(t *testing.T) {
	am := NewAlertManager()
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Rules: []config.AlertRule{
				{Name: "DiskFull", Metric: "disk_usage", Operator: ">", Threshold: 85, MountPoint: "/"},
			},
			Cooldown: 0,
		},
	}
	metrics := []ServerMetrics{{
		ServerName: "srv1",
		Timestamp:  time.Now(),
		Disks: []DiskStats{
			{MountPoint: "/", UsagePercent: 90},
			{MountPoint: "/data", UsagePercent: 60},
		},
	}}
	firings := am.Evaluate(metrics, cfg)
	if len(firings) != 1 {
		t.Fatalf("expected 1 disk firing, got %d", len(firings))
	}
}

func TestAlertManager_CertExpiresDays(t *testing.T) {
	am := NewAlertManager()
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Rules: []config.AlertRule{
				{Name: "CertExpiringSoon", Metric: "cert_expires_days", Operator: "<", Threshold: 30},
			},
			Cooldown: 0,
		},
	}
	days := 10.0
	metrics := []ServerMetrics{{
		ServerName: "srv1",
		Timestamp:  time.Now(),
		Connectivity: ConnectivityStats{
			HTTP: []HTTPResult{{URL: "https://example.com/health", OK: true, CertExpiresDays: &days}},
		},
	}}
	firings := am.Evaluate(metrics, cfg)
	if len(firings) != 1 {
		t.Fatalf("expected 1 cert firing, got %d", len(firings))
	}
	if firings[0].Metric != "cert_expires_days" {
		t.Fatalf("metric = %q, want cert_expires_days", firings[0].Metric)
	}
}

func TestCmp(t *testing.T) {
	tests := []struct {
		value    float64
		op       string
		thresh   float64
		expected bool
	}{
		{90, ">", 80, true},
		{70, ">", 80, false},
		{80, ">=", 80, true},
		{79, ">=", 80, false},
		{70, "<", 80, true},
		{90, "<", 80, false},
		{80, "<=", 80, true},
		{80, "==", 80, true},
		{81, "==", 80, false},
		{81, "!=", 80, true},
	}
	for _, tt := range tests {
		got := cmp(tt.value, tt.op, tt.thresh)
		if got != tt.expected {
			t.Errorf("cmp(%.0f, %q, %.0f) = %v; want %v", tt.value, tt.op, tt.thresh, got, tt.expected)
		}
	}
}

func TestActionExecutableAllowed(t *testing.T) {
	if !actionExecutableAllowed("/usr/bin/systemctl", []string{"systemctl"}) {
		t.Fatal("expected basename allowlist match")
	}
	if actionExecutableAllowed("/usr/bin/rm", []string{"systemctl"}) {
		t.Fatal("expected executable to be blocked")
	}
}

func TestRunAlertAction_EmptyFiringsNoop(t *testing.T) {
	err := RunAlertAction(config.AlertActionConfig{
		Command:            "echo test",
		AllowedExecutables: []string{"echo"},
		Timeout:            1,
	}, nil)
	if err != nil {
		t.Fatalf("expected nil error for empty firings, got %v", err)
	}
}
