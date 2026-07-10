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

func TestAlertManager_NTPOffset(t *testing.T) {
	am := NewAlertManager()
	cfg := &config.Config{Alerts: config.AlertsConfig{Rules: []config.AlertRule{{
		Name: "ClockDrift", Metric: "ntp_offset", Operator: ">", Threshold: 50,
	}}, Cooldown: 0}}
	metrics := []ServerMetrics{{
		ServerName:   "srv1",
		Timestamp:    time.Now(),
		Connectivity: ConnectivityStats{NTP: []NTPResult{{Host: "time.example", OK: true, OffsetMs: 73}}},
	}}
	if firings := am.Evaluate(metrics, cfg); len(firings) != 1 {
		t.Fatalf("expected NTP offset alert, got %d", len(firings))
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

func TestAlertManager_DiskInodeUsage(t *testing.T) {
	am := NewAlertManager()
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Rules: []config.AlertRule{
				{Name: "InodesFull", Metric: "disk_inode_usage", Operator: ">", Threshold: 80, MountPoint: "/"},
			},
			Cooldown: 0,
		},
	}
	metrics := []ServerMetrics{{
		ServerName: "srv1",
		Timestamp:  time.Now(),
		Disks: []DiskStats{
			{MountPoint: "/", InodesTotal: 1000, InodesUsagePercent: 91},
			{MountPoint: "/data", InodesTotal: 1000, InodesUsagePercent: 20},
		},
	}}
	firings := am.Evaluate(metrics, cfg)
	if len(firings) != 1 {
		t.Fatalf("expected 1 inode firing, got %d", len(firings))
	}
}

func TestAlertManager_FileDescriptorUsage(t *testing.T) {
	am := NewAlertManager()
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Rules: []config.AlertRule{
				{Name: "FileDescriptorsHigh", Metric: "file_descriptor_usage", Operator: ">", Threshold: 80},
			},
			Cooldown: 0,
		},
	}
	metrics := []ServerMetrics{{
		ServerName:      "srv1",
		Timestamp:       time.Now(),
		FileDescriptors: &FileDescriptorStats{UsagePercent: 90},
	}}
	firings := am.Evaluate(metrics, cfg)
	if len(firings) != 1 {
		t.Fatalf("expected 1 file descriptor firing, got %d", len(firings))
	}
}

func TestAlertManager_ProcessAndNetworkMetrics(t *testing.T) {
	am := NewAlertManager()
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Rules: []config.AlertRule{
				{Name: "TooManyProcesses", Metric: "processes_total", Operator: ">", Threshold: 200},
				{Name: "NetworkErrors", Metric: "network_errors", Operator: ">", Threshold: 0},
				{Name: "NetworkDrops", Metric: "network_drops", Operator: ">", Threshold: 0},
			},
			Cooldown: 0,
		},
	}
	metrics := []ServerMetrics{{
		ServerName: "srv1",
		Timestamp:  time.Now(),
		Load:       &LoadStats{RunningProcesses: 2, TotalProcesses: 250},
		Network: []NetworkStats{
			{Interface: "eth0", ErrorsRecv: 1, DropsSent: 2},
		},
	}}
	firings := am.Evaluate(metrics, cfg)
	if len(firings) != 3 {
		t.Fatalf("expected 3 firings, got %d", len(firings))
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

func TestAlertManager_NetworkProbeMetrics(t *testing.T) {
	tlsDays := 5.0
	metrics := []ServerMetrics{{
		ServerName: "web-01",
		Connectivity: ConnectivityStats{
			DNS: []DNSResult{{
				Host:      "example.com",
				Type:      "A",
				OK:        false,
				LatencyMs: 120,
			}},
			Traceroute: []TracerouteResult{{
				Host: "example.com",
				OK:   true,
				Hops: 24,
			}},
			TLS: []TLSResult{{
				Host:            "example.com",
				Port:            443,
				OK:              true,
				CertExpiresDays: &tlsDays,
			}},
		},
	}}
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Cooldown: 1,
			Rules: []config.AlertRule{
				{Name: "DNSFailed", Metric: "dns_failed", Operator: "==", Threshold: 1},
				{Name: "TraceTooLong", Metric: "traceroute_hops", Operator: ">", Threshold: 20},
				{Name: "TLSExpires", Metric: "tls_cert_expires_days", Operator: "<", Threshold: 10},
			},
		},
	}

	firings := NewAlertManager().Evaluate(metrics, cfg)
	if len(firings) != 3 {
		t.Fatalf("firings len = %d, want 3: %#v", len(firings), firings)
	}
}

func TestAlertManager_BoardMetrics(t *testing.T) {
	temp := 82.5
	rssi := -78.0
	metrics := []ServerMetrics{{
		ServerName: "pi-01",
		Board: &BoardInfo{
			TemperatureC:    &temp,
			UnderVoltageNow: true,
			ThrottledNow:    true,
			WiFiRSSIDbm:     &rssi,
		},
	}}
	cfg := &config.Config{
		Alerts: config.AlertsConfig{
			Cooldown: 0,
			Rules: []config.AlertRule{
				{Name: "PiHot", Metric: "board_temperature", Operator: ">", Threshold: 80},
				{Name: "PiUndervoltage", Metric: "board_under_voltage", Operator: "==", Threshold: 1},
				{Name: "PiThrottled", Metric: "board_throttled", Operator: "==", Threshold: 1},
				{Name: "PiWiFiWeak", Metric: "board_wifi_rssi", Operator: "<", Threshold: -75},
			},
		},
	}

	firings := NewAlertManager().Evaluate(metrics, cfg)
	if len(firings) != 4 {
		t.Fatalf("firings len = %d, want 4: %#v", len(firings), firings)
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
