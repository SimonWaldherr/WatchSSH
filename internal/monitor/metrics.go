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
	Inodes    []InodeStats   `json:"inodes,omitempty"`
	Network   []NetworkStats `json:"network"`
	Processes []ProcessInfo  `json:"processes"`
	// FileDescriptors is populated when the platform exposes kernel-wide counters.
	FileDescriptors *FileDescriptorStats `json:"file_descriptors,omitempty"`
	Users           []LoggedInUser       `json:"logged_in_users,omitempty"`
	// Containers is populated on Linux hosts when docker.enabled is true.
	Containers   []ContainerInfo     `json:"containers,omitempty"`
	Board        *BoardInfo          `json:"board,omitempty"`
	Connectivity ConnectivityStats   `json:"connectivity"`
	CustomChecks []CustomCheckResult `json:"custom_checks,omitempty"`
	// StandardTools contains non-sensitive availability facts for common POSIX,
	// Linux, and operational tools. It is populated only when tool_inventory is enabled.
	StandardTools map[string]bool `json:"standard_tools,omitempty"`
	// Audit is an optional, bounded inventory of accounts and installed packages.
	Audit *AuditResult `json:"audit,omitempty"`

	// Capabilities maps each metric name to its availability status:
	// "ok", "unsupported", "unavailable", or "error".
	Capabilities map[string]string `json:"capabilities,omitempty"`
	// MetricErrors maps metric names to human-readable error messages.
	MetricErrors map[string]string `json:"metric_errors,omitempty"`
}

// AuditResult is collected only after explicit per-server opt-in. It never
// includes credentials, hashes, home directory contents, or package metadata.
type AuditResult struct {
	Users        []AuditUser       `json:"users,omitempty"`
	Packages     []string          `json:"packages,omitempty"`
	PackageTool  string            `json:"package_tool,omitempty"`
	UsersCut     bool              `json:"users_truncated,omitempty"`
	PackagesCut  bool              `json:"packages_truncated,omitempty"`
	Capabilities map[string]string `json:"capabilities,omitempty"`
}

type AuditUser struct {
	Name string `json:"name"`
	UID  int    `json:"uid"`
}

// SystemInfo contains static system information.
type SystemInfo struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Kernel   string `json:"kernel"`
	Arch     string `json:"arch"`
	CPUCores int    `json:"cpu_cores,omitempty"`
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
	Device             string  `json:"device"`
	MountPoint         string  `json:"mount_point"`
	TotalBytes         int64   `json:"total_bytes"`
	UsedBytes          int64   `json:"used_bytes"`
	FreeBytes          int64   `json:"free_bytes"`
	UsagePercent       float64 `json:"usage_percent"`
	InodesTotal        int64   `json:"inodes_total,omitempty"`
	InodesUsed         int64   `json:"inodes_used,omitempty"`
	InodesFree         int64   `json:"inodes_free,omitempty"`
	InodesUsagePercent float64 `json:"inodes_usage_percent,omitempty"`
}

// InodeStats contains inode usage for one mount point.
type InodeStats struct {
	Device       string  `json:"device"`
	MountPoint   string  `json:"mount_point"`
	TotalInodes  int64   `json:"total_inodes"`
	UsedInodes   int64   `json:"used_inodes"`
	FreeInodes   int64   `json:"free_inodes"`
	UsagePercent float64 `json:"usage_percent"`
}

// NetworkStats contains cumulative byte/packet counters for one interface.
type NetworkStats struct {
	Interface   string `json:"interface"`
	BytesRecv   int64  `json:"bytes_recv"`
	BytesSent   int64  `json:"bytes_sent"`
	PacketsRecv int64  `json:"packets_recv"`
	PacketsSent int64  `json:"packets_sent"`
	ErrorsRecv  int64  `json:"errors_recv,omitempty"`
	ErrorsSent  int64  `json:"errors_sent,omitempty"`
	DropsRecv   int64  `json:"drops_recv,omitempty"`
	DropsSent   int64  `json:"drops_sent,omitempty"`
}

// LoadStats contains system load averages and uptime.
type LoadStats struct {
	Load1            float64 `json:"load1"`
	Load5            float64 `json:"load5"`
	Load15           float64 `json:"load15"`
	UptimeSeconds    float64 `json:"uptime_seconds"`
	RunningProcesses int     `json:"running_processes,omitempty"`
	TotalProcesses   int     `json:"total_processes,omitempty"`
	LastPID          int     `json:"last_pid,omitempty"`
}

// LoggedInUser represents one active login session.
type LoggedInUser struct {
	User      string `json:"user"`
	TTY       string `json:"tty"`
	LoginTime string `json:"login_time"`
	Host      string `json:"host,omitempty"`
}

// ProcessInfo represents a single running process.
type ProcessInfo struct {
	PID            int     `json:"pid"`
	User           string  `json:"user"`
	CPUPercent     float64 `json:"cpu_percent"`
	MemPercent     float64 `json:"mem_percent"`
	RSSBytes       int64   `json:"rss_bytes"`
	DiskReadBytes  int64   `json:"disk_read_bytes,omitempty"`
	DiskWriteBytes int64   `json:"disk_write_bytes,omitempty"`
	State          string  `json:"state"`
	Command        string  `json:"command"`
}

// ConnectivityStats contains results of external connectivity checks.
type ConnectivityStats struct {
	// PingEnabled is true when a ping check was configured and attempted.
	PingEnabled bool               `json:"ping_enabled"`
	PingOK      bool               `json:"ping_ok"`
	PingLatency float64            `json:"ping_latency_ms"`
	PingLoss    float64            `json:"ping_loss_percent"`
	Ports       []PortResult       `json:"ports,omitempty"`
	Banner      []BannerResult     `json:"banner,omitempty"`
	HTTP        []HTTPResult       `json:"http,omitempty"`
	DNS         []DNSResult        `json:"dns,omitempty"`
	Traceroute  []TracerouteResult `json:"traceroute,omitempty"`
	TLS         []TLSResult        `json:"tls,omitempty"`
	NTP         []NTPResult        `json:"ntp,omitempty"`
}

// PortResult holds the outcome of a single TCP port check.
type PortResult struct {
	Host      string  `json:"host,omitempty"`
	Port      int     `json:"port"`
	Source    string  `json:"source,omitempty"` // monitor or target
	Open      bool    `json:"open"`
	LatencyMs float64 `json:"latency_ms"`
	Error     string  `json:"error,omitempty"`
}

// BannerResult holds the outcome of a TCP greeting/banner probe.
type BannerResult struct {
	Name      string  `json:"name,omitempty"`
	Host      string  `json:"host"`
	Port      int     `json:"port"`
	Banner    string  `json:"banner,omitempty"`
	OK        bool    `json:"ok"`
	LatencyMs float64 `json:"latency_ms"`
	Error     string  `json:"error,omitempty"`
}

// HTTPResult holds the outcome of a single HTTP health check.
type HTTPResult struct {
	URL             string   `json:"url"`
	Method          string   `json:"method,omitempty"`
	StatusCode      int      `json:"status_code"`
	OK              bool     `json:"ok"`
	LatencyMs       float64  `json:"latency_ms"`
	CertExpiresDays *float64 `json:"cert_expires_days,omitempty"`
	Error           string   `json:"error,omitempty"`
}

// DNSResult holds the outcome of a single DNS probe.
type DNSResult struct {
	Name      string   `json:"name,omitempty"`
	Host      string   `json:"host"`
	Type      string   `json:"type"`
	Server    string   `json:"server,omitempty"`
	Answers   []string `json:"answers,omitempty"`
	OK        bool     `json:"ok"`
	LatencyMs float64  `json:"latency_ms"`
	Error     string   `json:"error,omitempty"`
}

// TracerouteResult holds the outcome of a single traceroute probe.
type TracerouteResult struct {
	Name      string  `json:"name,omitempty"`
	Host      string  `json:"host"`
	OK        bool    `json:"ok"`
	Hops      int     `json:"hops"`
	LatencyMs float64 `json:"latency_ms"`
	Error     string  `json:"error,omitempty"`
}

// TLSResult holds the outcome of a single TLS certificate probe.
type TLSResult struct {
	Name            string   `json:"name,omitempty"`
	Host            string   `json:"host"`
	Port            int      `json:"port"`
	ServerName      string   `json:"server_name,omitempty"`
	OK              bool     `json:"ok"`
	LatencyMs       float64  `json:"latency_ms"`
	CertExpiresDays *float64 `json:"cert_expires_days,omitempty"`
	Issuer          string   `json:"issuer,omitempty"`
	Subject         string   `json:"subject,omitempty"`
	Error           string   `json:"error,omitempty"`
}

// NTPResult holds the outcome of an SNTP clock probe.
type NTPResult struct {
	Name      string  `json:"name,omitempty"`
	Host      string  `json:"host"`
	Port      int     `json:"port"`
	OK        bool    `json:"ok"`
	LatencyMs float64 `json:"latency_ms"`
	OffsetMs  float64 `json:"offset_ms"`
	Stratum   int     `json:"stratum"`
	Error     string  `json:"error,omitempty"`
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

// FileDescriptorStats contains kernel-wide file descriptor usage.
type FileDescriptorStats struct {
	Allocated    int64   `json:"allocated"`
	Unused       int64   `json:"unused"`
	Max          int64   `json:"max"`
	UsagePercent float64 `json:"usage_percent"`
}

// BoardInfo contains optional Raspberry Pi / SBC diagnostics.
type BoardInfo struct {
	Model            string   `json:"model,omitempty"`
	TemperatureC     *float64 `json:"temperature_c,omitempty"`
	CPUFrequencyMHz  *float64 `json:"cpu_frequency_mhz,omitempty"`
	ThrottledHex     string   `json:"throttled_hex,omitempty"`
	UnderVoltageNow  bool     `json:"under_voltage_now,omitempty"`
	ThrottledNow     bool     `json:"throttled_now,omitempty"`
	UnderVoltageSeen bool     `json:"under_voltage_seen,omitempty"`
	ThrottledSeen    bool     `json:"throttled_seen,omitempty"`
	WiFiInterface    string   `json:"wifi_interface,omitempty"`
	WiFiRSSIDbm      *float64 `json:"wifi_rssi_dbm,omitempty"`
}

// Firing represents an alert rule that has been triggered for a server.
type Firing struct {
	RuleName string    `json:"rule_name"`
	Metric   string    `json:"metric"`
	Server   string    `json:"server"`
	Value    float64   `json:"value"`
	Message  string    `json:"message"`
	FiredAt  time.Time `json:"fired_at"`
	// Remediations records automatic actions attempted for this firing.
	Remediations []RemediationResult `json:"remediations,omitempty"`
	// Watchdog contains the optional AI advisor analysis for this firing.
	Watchdog *WatchdogResult `json:"watchdog,omitempty"`
}

// RemediationResult records one automatic command attempt after an alert.
// Output is capped before it is persisted or exposed through the API.
type RemediationResult struct {
	Name       string    `json:"name"`
	Target     string    `json:"target"`
	StartedAt  time.Time `json:"started_at"`
	DurationMs float64   `json:"duration_ms"`
	Status     string    `json:"status"` // succeeded, failed, skipped_cooldown, skipped_rate_limit
	Output     string    `json:"output,omitempty"`
	Error      string    `json:"error,omitempty"`
}

// WatchdogResult records a bounded OpenAI-compatible advisor analysis. Model
// suggestions can only name configured runbooks and always require an operator
// to review and perform the corresponding action.
type WatchdogResult struct {
	Model                   string    `json:"model"`
	StartedAt               time.Time `json:"started_at"`
	DurationMs              float64   `json:"duration_ms"`
	Status                  string    `json:"status"` // analyzed, failed, skipped_cooldown
	Severity                string    `json:"severity,omitempty"`
	Summary                 string    `json:"summary,omitempty"`
	RecommendedRemediations []string  `json:"recommended_remediations,omitempty"`
	RejectedRemediations    []string  `json:"rejected_remediations,omitempty"`
	Error                   string    `json:"error,omitempty"`
}
