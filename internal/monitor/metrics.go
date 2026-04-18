// Package monitor collects metrics from remote servers over SSH.
package monitor

import "time"

// schemaVersion identifies the JSON output schema. Increment on breaking changes.
const schemaVersion = "2"

// ServerMetrics is a point-in-time snapshot of all metrics for one server.
type ServerMetrics struct {
	ServerName    string    `json:"server_name"`
	Host          string    `json:"host"`
	Timestamp     time.Time `json:"timestamp"`
	SchemaVersion string    `json:"schema_version"`
	// Platform is the detected remote OS family (e.g. "Linux", "Darwin", "FreeBSD").
	Platform string `json:"platform,omitempty"`
	// Error is non-empty when the collection failed (e.g. SSH connection refused).
	Error  string     `json:"error,omitempty"`
	System SystemInfo `json:"system"`

	// Pointer fields are null in JSON when the metric is unavailable or unsupported.
	// Check Capabilities for the reason ("ok", "unsupported", "unavailable", "error").
	CPU    *CPUStats    `json:"cpu"`    // null on first poll or if unsupported
	Memory *MemoryStats `json:"memory"` // null on collection error
	Swap   *SwapStats   `json:"swap"`   // null if platform has no swap
	Load   *LoadStats   `json:"load"`   // null if unsupported

	Disks     []DiskStats    `json:"disks"`
	Network   []NetworkStats `json:"network"`
	Processes []ProcessInfo  `json:"processes"`
	// Containers is populated on Linux hosts when docker.enabled is true.
	Containers   []ContainerInfo     `json:"containers,omitempty"`
	Connectivity ConnectivityStats   `json:"connectivity"`
	CustomChecks []CustomCheckResult `json:"custom_checks,omitempty"`

	// Capabilities maps each metric name to its availability status:
	// "ok", "unsupported", "unavailable", or "error".
	Capabilities map[string]string `json:"capabilities,omitempty"`
	// MetricErrors maps metric names to human-readable error messages.
	MetricErrors map[string]string `json:"metric_errors,omitempty"`
}

// SystemInfo contains static system information.
type SystemInfo struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Kernel   string `json:"kernel"`
	Arch     string `json:"arch"`
}

// CPUStats contains CPU utilisation data derived from a two-sample delta.
// IOWaitPercent is 0 on platforms that do not report I/O wait separately.
type CPUStats struct {
	UsagePercent  float64 `json:"usage_percent"`
	UserPercent   float64 `json:"user_percent"`
	SystemPercent float64 `json:"system_percent"`
	IdlePercent   float64 `json:"idle_percent"`
	IOWaitPercent float64 `json:"iowait_percent"`
}

// MemoryStats contains physical RAM utilisation.
// Swap is reported separately in SwapStats.
type MemoryStats struct {
	TotalBytes     int64   `json:"total_bytes"`
	UsedBytes      int64   `json:"used_bytes"`
	FreeBytes      int64   `json:"free_bytes"`
	AvailableBytes int64   `json:"available_bytes"`
	UsagePercent   float64 `json:"usage_percent"`
}

// SwapStats contains swap/virtual-memory utilisation.
type SwapStats struct {
	TotalBytes int64   `json:"total_bytes"`
	UsedBytes  int64   `json:"used_bytes"`
	FreeBytes  int64   `json:"free_bytes"`
	Percent    float64 `json:"percent"`
}

// DiskStats contains usage for one mount point.
type DiskStats struct {
	Device       string  `json:"device"`
	MountPoint   string  `json:"mount_point"`
	TotalBytes   int64   `json:"total_bytes"`
	UsedBytes    int64   `json:"used_bytes"`
	FreeBytes    int64   `json:"free_bytes"`
	UsagePercent float64 `json:"usage_percent"`
}

// NetworkStats contains cumulative byte/packet counters for one interface.
type NetworkStats struct {
	Interface   string `json:"interface"`
	BytesRecv   int64  `json:"bytes_recv"`
	BytesSent   int64  `json:"bytes_sent"`
	PacketsRecv int64  `json:"packets_recv"`
	PacketsSent int64  `json:"packets_sent"`
}

// LoadStats contains system load averages and uptime.
type LoadStats struct {
	Load1         float64 `json:"load1"`
	Load5         float64 `json:"load5"`
	Load15        float64 `json:"load15"`
	UptimeSeconds float64 `json:"uptime_seconds"`
}

// ProcessInfo represents a single running process.
type ProcessInfo struct {
	PID        int     `json:"pid"`
	User       string  `json:"user"`
	CPUPercent float64 `json:"cpu_percent"`
	MemPercent float64 `json:"mem_percent"`
	RSSBytes   int64   `json:"rss_bytes"`
	State      string  `json:"state"`
	Command    string  `json:"command"`
}

// ConnectivityStats contains results of external connectivity checks.
type ConnectivityStats struct {
	// PingEnabled is true when a ping check was configured and attempted.
	PingEnabled bool         `json:"ping_enabled"`
	PingOK      bool         `json:"ping_ok"`
	PingLatency float64      `json:"ping_latency_ms"`
	Ports       []PortResult `json:"ports,omitempty"`
	HTTP        []HTTPResult `json:"http,omitempty"`
}

// PortResult holds the outcome of a single TCP port check.
type PortResult struct {
	Port int  `json:"port"`
	Open bool `json:"open"`
}

// HTTPResult holds the outcome of a single HTTP health check.
type HTTPResult struct {
	URL             string   `json:"url"`
	StatusCode      int      `json:"status_code"`
	OK              bool     `json:"ok"`
	LatencyMs       float64  `json:"latency_ms"`
	CertExpiresDays *float64 `json:"cert_expires_days,omitempty"`
}

// CustomCheckResult holds the outcome of a custom SSH command check.
type CustomCheckResult struct {
	Name   string `json:"name"`
	Output string `json:"output"`
	OK     bool   `json:"ok"`
}

// ContainerInfo represents a single running Docker container and its resource usage.
// This field is only populated on Linux hosts when docker.enabled is true in the
// server configuration and the Docker daemon is reachable.
type ContainerInfo struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Image           string  `json:"image"`
	Status          string  `json:"status"`
	CPUPercent      float64 `json:"cpu_percent"`
	MemUsedBytes    int64   `json:"mem_used_bytes"`
	MemLimitBytes   int64   `json:"mem_limit_bytes"`
	MemPercent      float64 `json:"mem_percent"`
	NetRxBytes      int64   `json:"net_rx_bytes"`
	NetTxBytes      int64   `json:"net_tx_bytes"`
	BlockReadBytes  int64   `json:"block_read_bytes"`
	BlockWriteBytes int64   `json:"block_write_bytes"`
}

// Firing represents an alert rule that has been triggered for a server.
type Firing struct {
	RuleName string    `json:"rule_name"`
	Metric   string    `json:"metric"`
	Server   string    `json:"server"`
	Value    float64   `json:"value"`
	Message  string    `json:"message"`
	FiredAt  time.Time `json:"fired_at"`
}
