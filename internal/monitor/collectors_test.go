package monitor_test

import (
	"testing"

	"github.com/SimonWaldherr/WatchSSH/internal/monitor"
)

// ---------------------------------------------------------------------------
// parseMemInfo
// ---------------------------------------------------------------------------

var sampleMemInfo = `MemTotal:       16384000 kB
MemFree:          512000 kB
MemAvailable:    8192000 kB
Buffers:          256000 kB
Cached:          4096000 kB
SwapCached:            0 kB
SwapTotal:       4096000 kB
SwapFree:        3072000 kB
`

func TestParseMemInfo(t *testing.T) {
	m, swap, err := monitor.ParseMemInfo(sampleMemInfo)
	if err != nil {
		t.Fatalf("ParseMemInfo() error = %v", err)
	}
	if m.TotalBytes != 16384000*1024 {
		t.Errorf("TotalBytes = %d, want %d", m.TotalBytes, 16384000*1024)
	}
	if m.AvailableBytes != 8192000*1024 {
		t.Errorf("AvailableBytes = %d, want %d", m.AvailableBytes, 8192000*1024)
	}
	if m.UsagePercent < 0 || m.UsagePercent > 100 {
		t.Errorf("UsagePercent = %f out of range", m.UsagePercent)
	}
	if swap == nil {
		t.Fatal("expected non-nil SwapStats")
	}
	if swap.TotalBytes != 4096000*1024 {
		t.Errorf("SwapTotalBytes = %d, want %d", swap.TotalBytes, 4096000*1024)
	}
	if swap.UsedBytes != (4096000-3072000)*1024 {
		t.Errorf("SwapUsedBytes = %d", swap.UsedBytes)
	}
	if swap.Percent < 0 || swap.Percent > 100 {
		t.Errorf("SwapPercent = %f out of range", swap.Percent)
	}
}

func TestParseMemInfo_Empty(t *testing.T) {
	m, swap, err := monitor.ParseMemInfo("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.TotalBytes != 0 {
		t.Errorf("expected zero TotalBytes")
	}
	if swap != nil {
		t.Errorf("expected nil swap for empty input")
	}
}

// ---------------------------------------------------------------------------
// parseDiskInfo
// ---------------------------------------------------------------------------

var sampleDf = `Filesystem     1B-blocks       Used  Available Use% Mounted on
/dev/sda1      104857600000 45097156608 54452572160  46% /
/dev/sdb1      209715200000 10485760000 199229440000   5% /data
`

func TestParseDiskInfo(t *testing.T) {
	disks, err := monitor.ParseDiskInfo(sampleDf)
	if err != nil {
		t.Fatalf("ParseDiskInfo() error = %v", err)
	}
	if len(disks) != 2 {
		t.Fatalf("got %d disks, want 2", len(disks))
	}
	d := disks[0]
	if d.Device != "/dev/sda1" {
		t.Errorf("Device = %q, want /dev/sda1", d.Device)
	}
	if d.MountPoint != "/" {
		t.Errorf("MountPoint = %q, want /", d.MountPoint)
	}
	if d.TotalBytes != 104857600000 {
		t.Errorf("TotalBytes = %d", d.TotalBytes)
	}
	if d.UsagePercent != 46 {
		t.Errorf("UsagePercent = %f, want 46", d.UsagePercent)
	}
}

func TestParseDiskInfo_HeaderOnly(t *testing.T) {
	disks, err := monitor.ParseDiskInfo("Filesystem 1B-blocks Used Available Use% Mounted on\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(disks) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(disks))
	}
}

// ---------------------------------------------------------------------------
// parseNetDev
// ---------------------------------------------------------------------------

var sampleNetDev = `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 12345678       100    0    0    0     0          0         0 12345678       100    0    0    0     0       0          0
  eth0:  9876543      5000    0    0    0     0          0         0  1234567      4000    0    0    0     0       0          0
`

func TestParseNetDev(t *testing.T) {
	stats, err := monitor.ParseNetDev(sampleNetDev)
	if err != nil {
		t.Fatalf("ParseNetDev() error = %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("got %d interfaces, want 2", len(stats))
	}
	lo := stats[0]
	if lo.Interface != "lo" {
		t.Errorf("Interface = %q, want lo", lo.Interface)
	}
	if lo.BytesRecv != 12345678 {
		t.Errorf("BytesRecv = %d, want 12345678", lo.BytesRecv)
	}
	if lo.BytesSent != 12345678 {
		t.Errorf("BytesSent = %d, want 12345678", lo.BytesSent)
	}
}

// ---------------------------------------------------------------------------
// parseLoadAvg
// ---------------------------------------------------------------------------

func TestParseLoadAvg(t *testing.T) {
	load, err := monitor.ParseLoadAvg("0.52 0.58 0.59 1/432 12345\n")
	if err != nil {
		t.Fatalf("ParseLoadAvg() error = %v", err)
	}
	if load.Load1 != 0.52 {
		t.Errorf("Load1 = %f, want 0.52", load.Load1)
	}
	if load.Load5 != 0.58 {
		t.Errorf("Load5 = %f, want 0.58", load.Load5)
	}
	if load.Load15 != 0.59 {
		t.Errorf("Load15 = %f, want 0.59", load.Load15)
	}
}

func TestParseLoadAvg_Invalid(t *testing.T) {
	_, err := monitor.ParseLoadAvg("only one field")
	if err == nil {
		t.Fatal("expected error for invalid loadavg")
	}
}

// ---------------------------------------------------------------------------
// parseUptime
// ---------------------------------------------------------------------------

func TestParseUptime(t *testing.T) {
	sec, err := monitor.ParseUptime("123456.78 98765.43\n")
	if err != nil {
		t.Fatalf("ParseUptime() error = %v", err)
	}
	if sec != 123456.78 {
		t.Errorf("UptimeSeconds = %f, want 123456.78", sec)
	}
}

func TestParseUptime_Empty(t *testing.T) {
	_, err := monitor.ParseUptime("")
	if err == nil {
		t.Fatal("expected error for empty uptime")
	}
}

// ---------------------------------------------------------------------------
// parseProcStat / calcCPUStats
// ---------------------------------------------------------------------------

var procStat1 = `cpu  100 10 50 740 20 0 5 0 0 0
cpu0 50 5 25 370 10 0 2 0 0 0
`
var procStat2 = `cpu  200 10 60 820 30 0 5 0 0 0
cpu0 100 5 30 410 15 0 2 0 0 0
`

func TestCalcCPUStats(t *testing.T) {
	cpu, err := monitor.CalcCPUFromStatOutputs(procStat1, procStat2)
	if err != nil {
		t.Fatalf("CalcCPUFromStatOutputs() error = %v", err)
	}
	// total diff = (200+10+60+820+30+0+5) - (100+10+50+740+20+0+5) = 1125 - 925 = 200
	// idle diff = 820 - 740 = 80   → idle% = 40
	// iowait diff = 30 - 20 = 10   → iowait% = 5
	// usage% = 100 - 40 - 5 = 55
	if cpu.UsagePercent < 54.9 || cpu.UsagePercent > 55.1 {
		t.Errorf("UsagePercent = %f, want ~55", cpu.UsagePercent)
	}
	if cpu.IdlePercent < 39.9 || cpu.IdlePercent > 40.1 {
		t.Errorf("IdlePercent = %f, want ~40", cpu.IdlePercent)
	}
}

func TestCalcCPUStats_NoCPULine(t *testing.T) {
	_, err := monitor.CalcCPUFromStatOutputs("no cpu line here", procStat2)
	if err == nil {
		t.Fatal("expected error when cpu line is missing")
	}
}

// ---------------------------------------------------------------------------
// parseProcesses
// ---------------------------------------------------------------------------

var samplePS = `USER         PID %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND
root           1  0.1  0.0 167876  9784 ?        Ss   Jan01   0:02 /sbin/init
www-data    1234  2.5  1.2 345678 24576 ?        S    10:00   0:15 /usr/sbin/nginx -g daemon off;
`

func TestParseProcesses(t *testing.T) {
	procs, err := monitor.ParseProcesses(samplePS)
	if err != nil {
		t.Fatalf("ParseProcesses() error = %v", err)
	}
	if len(procs) != 2 {
		t.Fatalf("got %d processes, want 2", len(procs))
	}
	p := procs[1]
	if p.PID != 1234 {
		t.Errorf("PID = %d, want 1234", p.PID)
	}
	if p.User != "www-data" {
		t.Errorf("User = %q, want www-data", p.User)
	}
	if p.CPUPercent != 2.5 {
		t.Errorf("CPUPercent = %f, want 2.5", p.CPUPercent)
	}
}

func TestParseProcesses_Empty(t *testing.T) {
	procs, err := monitor.ParseProcesses("USER PID %CPU %MEM VSZ RSS TTY STAT START TIME COMMAND\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(procs) != 0 {
		t.Errorf("expected empty slice, got %d", len(procs))
	}
}

// ---------------------------------------------------------------------------
// parseSystemInfo
// ---------------------------------------------------------------------------

func TestParseSystemInfo(t *testing.T) {
	info := monitor.ParseSystemInfo("Linux myserver 5.15.0-91-generic x86_64", "myserver")
	if info.OS != "Linux" {
		t.Errorf("OS = %q, want Linux", info.OS)
	}
	if info.Kernel != "5.15.0-91-generic" {
		t.Errorf("Kernel = %q, want 5.15.0-91-generic", info.Kernel)
	}
	if info.Arch != "x86_64" {
		t.Errorf("Arch = %q, want x86_64", info.Arch)
	}
	if info.Hostname != "myserver" {
		t.Errorf("Hostname = %q, want myserver", info.Hostname)
	}
}
