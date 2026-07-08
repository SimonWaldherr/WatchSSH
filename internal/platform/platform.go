package platform

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// errWriter is where diagnostic messages are written. Can be replaced in tests.
var errWriter io.Writer = os.Stderr

// Family represents the detected operating system family.
type Family string

const (
	Linux       Family = "Linux"
	Darwin      Family = "Darwin"
	FreeBSD     Family = "FreeBSD"
	OpenBSD     Family = "OpenBSD"
	NetBSD      Family = "NetBSD"
	DragonFly   Family = "DragonFly"
	MidnightBSD Family = "MidnightBSD"
	SunOS       Family = "SunOS"
	Illumos     Family = "illumos"
	AIX         Family = "AIX"
	HPUX        Family = "HP-UX"
	Windows     Family = "Windows"
	Unknown     Family = "Unknown"
)

// CapStatus describes the availability of a collected metric.
type CapStatus string

const (
	CapOK          CapStatus = "ok"
	CapUnsupported CapStatus = "unsupported"
	CapUnavailable CapStatus = "unavailable"
	CapError       CapStatus = "error"
)

// Runner executes a shell command and returns its output.
type Runner interface {
	Run(ctx context.Context, cmd string) (string, error)
}

// Collector collects a Snapshot of system metrics.
type Collector interface {
	Collect(ctx context.Context, r Runner) (*Snapshot, error)
}

// ContainerInfo holds resource usage for a single Docker container.
// Only populated on Linux when Docker collection is enabled.
type ContainerInfo struct {
	ID              string
	Name            string
	Image           string
	Status          string
	CPUPercent      float64
	MemUsedBytes    int64
	MemLimitBytes   int64
	MemPercent      float64
	NetRxBytes      int64
	NetTxBytes      int64
	BlockReadBytes  int64
	BlockWriteBytes int64
}

// Snapshot holds all collected system metrics for one point in time.
type Snapshot struct {
	SystemInfo SystemInfo
	UptimeSecs *float64
	Load       *LoadAvg
	CPU        *CPUStats
	Memory     *MemoryStats
	Swap       *SwapStats
	Disks      []DiskStats
	Inodes     []InodeStats
	Network    []NetworkStats
	Processes  []ProcessInfo
	Users      []LoggedInUser
	Containers []ContainerInfo
	Board      *BoardInfo
	// FileDescriptors is collected on platforms with kernel-wide descriptor counters.
	FileDescriptors *FileDescriptorStats
	Caps            map[string]string // metric name → "ok"|"unsupported"|"unavailable"|"error"
	Errors          map[string]string // metric name → error message
}

func (s *Snapshot) setErr(metric, msg string) {
	if s.Caps == nil {
		s.Caps = map[string]string{}
	}
	if s.Errors == nil {
		s.Errors = map[string]string{}
	}
	s.Caps[metric] = string(CapError)
	s.Errors[metric] = msg
}

func (s *Snapshot) setOK(metric string) {
	if s.Caps == nil {
		s.Caps = map[string]string{}
	}
	s.Caps[metric] = string(CapOK)
}

func (s *Snapshot) setUnsupported(metric string) {
	if s.Caps == nil {
		s.Caps = map[string]string{}
	}
	s.Caps[metric] = string(CapUnsupported)
}

// SystemInfo holds basic OS identification.
type SystemInfo struct {
	OS       string
	Kernel   string
	Arch     string
	Hostname string
	CPUCores int
}

// LoadAvg holds the 1, 5, and 15-minute load averages.
type LoadAvg struct {
	Load1            float64
	Load5            float64
	Load15           float64
	RunningProcesses int
	TotalProcesses   int
	LastPID          int
}

// CPUStats holds CPU usage percentages derived from two samples.
type CPUStats struct {
	UsagePercent  float64
	UserPercent   float64
	SystemPercent float64
	IdlePercent   float64
	IOWaitPercent float64 // 0 if not available on platform
}

// MemoryStats holds physical memory usage.
type MemoryStats struct {
	TotalBytes     int64
	UsedBytes      int64
	FreeBytes      int64
	AvailableBytes int64
	UsagePercent   float64
}

// SwapStats holds swap space usage.
type SwapStats struct {
	TotalBytes int64
	UsedBytes  int64
	FreeBytes  int64
	Percent    float64
}

// DiskStats holds usage for a single mounted filesystem.
type DiskStats struct {
	Device             string
	MountPoint         string
	TotalBytes         int64
	UsedBytes          int64
	FreeBytes          int64
	UsagePercent       float64
	InodesTotal        int64
	InodesUsed         int64
	InodesFree         int64
	InodesUsagePercent float64
}

// InodeStats holds inode usage for a single mounted filesystem.
type InodeStats struct {
	Device       string
	MountPoint   string
	TotalInodes  int64
	UsedInodes   int64
	FreeInodes   int64
	UsagePercent float64
}

// NetworkStats holds cumulative byte/packet counters for one interface.
type NetworkStats struct {
	Interface   string
	BytesRecv   int64
	BytesSent   int64
	PacketsRecv int64
	PacketsSent int64
	ErrorsRecv  int64
	ErrorsSent  int64
	DropsRecv   int64
	DropsSent   int64
}

// FileDescriptorStats holds kernel-wide file descriptor usage.
type FileDescriptorStats struct {
	Allocated    int64
	Unused       int64
	Max          int64
	UsagePercent float64
}

// BoardInfo holds single-board-computer diagnostics when available.
type BoardInfo struct {
	Model            string
	TemperatureC     *float64
	CPUFrequencyMHz  *float64
	ThrottledHex     string
	UnderVoltageNow  bool
	ThrottledNow     bool
	UnderVoltageSeen bool
	ThrottledSeen    bool
	WiFiInterface    string
	WiFiRSSIDbm      *float64
}

// LoggedInUser represents one active login session reported by who(1).
type LoggedInUser struct {
	User      string
	TTY       string
	LoginTime string
	Host      string
}

// ProcessInfo holds resource usage for a single process.
type ProcessInfo struct {
	PID        int
	User       string
	CPUPercent float64
	MemPercent float64
	RSSBytes   int64
	State      string
	Command    string
}

// Detect runs `uname -s` and maps the output to a Family constant.
func Detect(ctx context.Context, r Runner) Family {
	out, err := r.Run(ctx, "uname -s")
	if err != nil {
		winOut, winErr := r.Run(ctx, "cmd /c ver")
		if winErr == nil && strings.Contains(strings.ToLower(winOut), "windows") {
			return Windows
		}
		return Unknown
	}
	switch strings.TrimSpace(out) {
	case "Linux":
		return Linux
	case "Darwin":
		return Darwin
	case "FreeBSD":
		return FreeBSD
	case "OpenBSD":
		return OpenBSD
	case "NetBSD":
		return NetBSD
	case "DragonFly":
		return DragonFly
	case "MidnightBSD":
		return MidnightBSD
	case "SunOS":
		return SunOS
	case "illumos":
		return Illumos
	case "AIX":
		return AIX
	case "HP-UX":
		return HPUX
	default:
		return Unknown
	}
}

// New returns a Collector appropriate for the given OS family.
// Unknown families fall back to linuxCollector as best-effort.
// A warning is logged so administrators can identify unsupported OSes.
func New(f Family) Collector {
	switch f {
	case Darwin:
		return &darwinCollector{}
	case FreeBSD:
		return &freebsdCollector{}
	case OpenBSD:
		return &openbsdCollector{}
	case NetBSD:
		return &netbsdCollector{}
	case DragonFly, MidnightBSD:
		return &freebsdCollector{}
	case SunOS, Illumos, AIX, HPUX:
		return &genericUnixCollector{family: f}
	case Windows:
		return &windowsCollector{}
	case Linux:
		return &linuxCollector{}
	default:
		// Unknown OS family: use the generic Unix collector as a best-effort fallback.
		// Many metrics will fail gracefully and be marked "error" in capabilities.
		// Log so operators can identify the unsupported system.
		logUnknownFamily(f)
		return &genericUnixCollector{family: f}
	}
}

// logUnknownFamily is a variable so it can be replaced in tests.
var logUnknownFamily = func(f Family) {
	// Use fmt.Fprintf to stderr rather than log to avoid a log import in this package.
	fmt.Fprintf(errWriter, "watchssh: unknown OS family %q — falling back to generic Unix collector (some metrics may be unavailable)\n", f)
}
