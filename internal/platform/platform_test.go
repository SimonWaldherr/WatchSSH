package platform

import (
	"math"
	"strconv"
	"testing"
	"time"
)

// TestParseSysctlBootTime verifies a valid BSD boottime is parsed to a positive uptime.
func TestParseSysctlBootTime(t *testing.T) {
	// Use a boot time 1 hour ago
	bootSec := time.Now().Unix() - 3600
	input := "{ sec = " + itoa64(bootSec) + ", usec = 0 }"
	uptime, err := parseSysctlBootTime(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uptime <= 0 {
		t.Errorf("expected uptime > 0, got %f", uptime)
	}
	// Should be approximately 3600s
	if math.Abs(uptime-3600) > 5 {
		t.Errorf("expected uptime ~3600, got %f", uptime)
	}
}

func TestParseSysctlBootTime_Invalid(t *testing.T) {
	_, err := parseSysctlBootTime("not valid")
	if err == nil {
		t.Error("expected error for invalid input")
	}
}

// TestParseSysctlLoadAvg verifies BSD loadavg parsing.
func TestParseSysctlLoadAvg(t *testing.T) {
	la, err := parseSysctlLoadAvg("{ 0.52 0.48 0.41 }")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if la.Load1 != 0.52 {
		t.Errorf("Load1: expected 0.52, got %f", la.Load1)
	}
	if la.Load5 != 0.48 {
		t.Errorf("Load5: expected 0.48, got %f", la.Load5)
	}
	if la.Load15 != 0.41 {
		t.Errorf("Load15: expected 0.41, got %f", la.Load15)
	}
}

func TestParseSysctlLoadAvg_Invalid(t *testing.T) {
	_, err := parseSysctlLoadAvg("{ 0.52 }")
	if err == nil {
		t.Error("expected error for too few fields")
	}
}

// TestParseDFOutput verifies POSIX df -kP parsing.
func TestParseDFOutput(t *testing.T) {
	input := `Filesystem     1024-blocks      Used Available Capacity Mounted on
/dev/sda1           102400     44040     58360      44% /
/dev/sdb1           204800     10240    194560       5% /data`

	disks, err := parseDFOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(disks) != 2 {
		t.Fatalf("expected 2 disks, got %d", len(disks))
	}

	d := disks[0]
	if d.Device != "/dev/sda1" {
		t.Errorf("Device: expected /dev/sda1, got %s", d.Device)
	}
	if d.MountPoint != "/" {
		t.Errorf("MountPoint: expected /, got %s", d.MountPoint)
	}
	if d.TotalBytes != 102400*1024 {
		t.Errorf("TotalBytes: expected %d, got %d", 102400*1024, d.TotalBytes)
	}
	if d.UsedBytes != 44040*1024 {
		t.Errorf("UsedBytes: expected %d, got %d", 44040*1024, d.UsedBytes)
	}
	if d.FreeBytes != 58360*1024 {
		t.Errorf("FreeBytes: expected %d, got %d", 58360*1024, d.FreeBytes)
	}
	if d.UsagePercent != 44.0 {
		t.Errorf("UsagePercent: expected 44.0, got %f", d.UsagePercent)
	}
}

// TestParsePSAux verifies BSD ps aux parsing.
func TestParsePSAux(t *testing.T) {
	input := `USER       PID %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND
root         1  0.0  0.1   8936  6780 ?        Ss   10:00   0:01 /sbin/init
user      1234  2.5  1.2  45678  8192 pts/0    S    10:01   0:02 bash -i`

	procs, err := parsePSAux(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(procs) != 2 {
		t.Fatalf("expected 2 procs, got %d", len(procs))
	}

	p := procs[1]
	if p.PID != 1234 {
		t.Errorf("PID: expected 1234, got %d", p.PID)
	}
	if p.User != "user" {
		t.Errorf("User: expected user, got %s", p.User)
	}
	if p.CPUPercent != 2.5 {
		t.Errorf("CPUPercent: expected 2.5, got %f", p.CPUPercent)
	}
	if p.RSSBytes != 8192*1024 {
		t.Errorf("RSSBytes: expected %d, got %d", 8192*1024, p.RSSBytes)
	}
}

// TestParseLinuxMemInfo verifies /proc/meminfo parsing.
func TestParseLinuxMemInfo(t *testing.T) {
	input := `MemTotal:       16384000 kB
MemFree:         2048000 kB
MemAvailable:    8192000 kB
Buffers:          512000 kB
Cached:          4096000 kB
SwapCached:            0 kB
SwapTotal:       4096000 kB
SwapFree:        3584000 kB`

	mem, swap, err := parseLinuxMemInfo(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mem.TotalBytes != 16384000*1024 {
		t.Errorf("TotalBytes: expected %d, got %d", int64(16384000*1024), mem.TotalBytes)
	}
	if mem.AvailableBytes != 8192000*1024 {
		t.Errorf("AvailableBytes: expected %d, got %d", int64(8192000*1024), mem.AvailableBytes)
	}
	// used = total - free - buffers - cached = 16384000 - 2048000 - 512000 - 4096000 = 9728000 kB
	expectedUsed := int64((16384000 - 2048000 - 512000 - 4096000) * 1024)
	if mem.UsedBytes != expectedUsed {
		t.Errorf("UsedBytes: expected %d, got %d", expectedUsed, mem.UsedBytes)
	}

	if swap == nil {
		t.Fatal("expected swap stats, got nil")
	}
	if swap.TotalBytes != 4096000*1024 {
		t.Errorf("Swap TotalBytes: expected %d, got %d", int64(4096000*1024), swap.TotalBytes)
	}
	expectedSwapUsed := int64((4096000 - 3584000) * 1024)
	if swap.UsedBytes != expectedSwapUsed {
		t.Errorf("Swap UsedBytes: expected %d, got %d", expectedSwapUsed, swap.UsedBytes)
	}
}

// TestParseLinuxNetDev verifies /proc/net/dev parsing.
func TestParseLinuxNetDev(t *testing.T) {
	input := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo:    1234      10    0    0    0     0          0         0     1234      10    0    0    0     0       0          0
  eth0: 56789012    4567    0    0    0     0          0         0 12345678    3456    0    0    0     0       0          0`

	nets, err := parseLinuxNetDev(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(nets) != 2 {
		t.Fatalf("expected 2 interfaces, got %d", len(nets))
	}

	eth0 := nets[1]
	if eth0.Interface != "eth0" {
		t.Errorf("Interface: expected eth0, got %s", eth0.Interface)
	}
	if eth0.BytesRecv != 56789012 {
		t.Errorf("BytesRecv: expected 56789012, got %d", eth0.BytesRecv)
	}
	if eth0.BytesSent != 12345678 {
		t.Errorf("BytesSent: expected 12345678, got %d", eth0.BytesSent)
	}
}

// TestParseLinuxProcStat_CalcCPU verifies CPU delta calculation.
func TestParseLinuxProcStat_CalcCPU(t *testing.T) {
	// Two /proc/stat samples ~1s apart
	stat1 := `cpu  1000 50 200 8000 100 10 20 0 0 0`
	stat2 := `cpu  1050 50 210 8100 105 10 20 0 0 0`

	s1, err := parseLinuxProcStat(stat1)
	if err != nil {
		t.Fatalf("parse sample1: %v", err)
	}
	s2, err := parseLinuxProcStat(stat2)
	if err != nil {
		t.Fatalf("parse sample2: %v", err)
	}

	cpu := calcLinuxCPU(s1, s2)
	if cpu == nil {
		t.Fatal("expected CPUStats, got nil")
	}

	// delta total = (1050+50+210+8100+105+10+20) - (1000+50+200+8000+100+10+20) = 9545 - 9380 = 165
	// user delta = (1050+50) - (1000+50) = 50
	// user% = 100 * 50 / 165 ≈ 30.3%
	if cpu.UserPercent < 25 || cpu.UserPercent > 40 {
		t.Errorf("UserPercent out of range: %f", cpu.UserPercent)
	}
	if cpu.IdlePercent < 50 || cpu.IdlePercent > 70 {
		t.Errorf("IdlePercent out of range: %f", cpu.IdlePercent)
	}
	if cpu.IOWaitPercent < 0 || cpu.IOWaitPercent > 10 {
		t.Errorf("IOWaitPercent out of range: %f", cpu.IOWaitPercent)
	}
}

// TestParseDarwinVMStat verifies macOS vm_stat parsing.
func TestParseDarwinVMStat(t *testing.T) {
	input := `Mach Virtual Memory Statistics: (page size of 4096 bytes)
Pages free:                               97376.
Pages active:                           2139204.
Pages inactive:                          561332.
Pages speculative:                        119038.
Pages wired down:                         487980.`

	physMem := int64(16 * 1024 * 1024 * 1024) // 16 GiB
	mem, err := parseDarwinVMStat(input, physMem)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mem.TotalBytes != physMem {
		t.Errorf("TotalBytes: expected %d, got %d", physMem, mem.TotalBytes)
	}
	// used = (active + wired) * pageSize = (2139204 + 487980) * 4096
	expectedUsed := int64((2139204 + 487980) * 4096)
	if mem.UsedBytes != expectedUsed {
		t.Errorf("UsedBytes: expected %d, got %d", expectedUsed, mem.UsedBytes)
	}
	if mem.UsagePercent <= 0 || mem.UsagePercent > 100 {
		t.Errorf("UsagePercent out of range: %f", mem.UsagePercent)
	}
}

// TestParseDarwinTopCPU verifies that the LAST CPU usage line is used.
func TestParseDarwinTopCPU(t *testing.T) {
	// Simulate `top -l 2` output with two CPU lines (first sample ignored, second used)
	input := `Processes: 300 total, 2 running
CPU usage: 10.00% user, 5.00% sys, 85.00% idle
Load Avg: 1.23, 1.10, 0.95
PhysMem: 4096M used
---- second sample ----
Processes: 300 total, 2 running
CPU usage: 5.91% user, 6.54% sys, 87.53% idle
Load Avg: 1.23, 1.10, 0.95`

	cpu, err := parseDarwinTopCPU(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should use the second (last) CPU line
	if math.Abs(cpu.UserPercent-5.91) > 0.01 {
		t.Errorf("UserPercent: expected 5.91, got %f", cpu.UserPercent)
	}
	if math.Abs(cpu.SystemPercent-6.54) > 0.01 {
		t.Errorf("SystemPercent: expected 6.54, got %f", cpu.SystemPercent)
	}
	if math.Abs(cpu.IdlePercent-87.53) > 0.01 {
		t.Errorf("IdlePercent: expected 87.53, got %f", cpu.IdlePercent)
	}
	if math.Abs(cpu.UsagePercent-(5.91+6.54)) > 0.01 {
		t.Errorf("UsagePercent: expected %f, got %f", 5.91+6.54, cpu.UsagePercent)
	}
}

// TestParseDarwinSwapUsage verifies sysctl vm.swapusage parsing.
func TestParseDarwinSwapUsage(t *testing.T) {
	input := "total = 1024.00M  used = 34.50M  free = 989.50M  (encrypted)"
	sw, err := parseDarwinSwapUsage(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sw == nil {
		t.Fatal("expected SwapStats, got nil")
	}
	expectedTotal := int64(1024 * 1024 * 1024)
	if sw.TotalBytes != expectedTotal {
		t.Errorf("TotalBytes: expected %d, got %d", expectedTotal, sw.TotalBytes)
	}
	expectedUsed := int64(34.5 * 1024 * 1024)
	if math.Abs(float64(sw.UsedBytes-expectedUsed)) > 1024 {
		t.Errorf("UsedBytes: expected ~%d, got %d", expectedUsed, sw.UsedBytes)
	}
}

func TestParseDarwinSwapUsage_Zero(t *testing.T) {
	input := "total = 0B  used = 0B  free = 0B"
	sw, err := parseDarwinSwapUsage(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sw != nil {
		t.Errorf("expected nil for zero swap, got %+v", sw)
	}
}

// TestParseDarwinNetstat verifies netstat -ibn parsing.
func TestParseDarwinNetstat(t *testing.T) {
	input := `Name  Mtu   Network       Address            Ipkts Ierrs     Ibytes    Opkts Oerrs     Obytes  Coll
lo0   16384 <Link#1>                         12345     0    1234567    12345     0    1234567     0
en0   1500  <Link#2>    a8:00:11:22:33:44    56789     0   56789012    45678     0   45678901     0
en0   1500  192.168.1   192.168.1.100        56789     -          -    45678     -          -     -`

	nets, err := parseDarwinNetstat(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only Link# entries should be included
	if len(nets) != 2 {
		t.Fatalf("expected 2 network entries, got %d", len(nets))
	}
	en0 := nets[1]
	if en0.Interface != "en0" {
		t.Errorf("Interface: expected en0, got %s", en0.Interface)
	}
	if en0.BytesRecv != 56789012 {
		t.Errorf("BytesRecv: expected 56789012, got %d", en0.BytesRecv)
	}
	if en0.BytesSent != 45678901 {
		t.Errorf("BytesSent: expected 45678901, got %d", en0.BytesSent)
	}
}

// TestParseFreeBSDCPTime verifies FreeBSD kern.cp_time parsing and CPU calc.
func TestParseFreeBSDCPTime(t *testing.T) {
	sample1 := "218340 0 167239 0 6855742"
	sample2 := "218500 0 167400 0 6856000"

	s1, err := parseFreeBSDCPTime(sample1)
	if err != nil {
		t.Fatalf("parse sample1: %v", err)
	}
	s2, err := parseFreeBSDCPTime(sample2)
	if err != nil {
		t.Fatalf("parse sample2: %v", err)
	}

	if s1.user != 218340 {
		t.Errorf("s1.user: expected 218340, got %d", s1.user)
	}
	if s1.idle != 6855742 {
		t.Errorf("s1.idle: expected 6855742, got %d", s1.idle)
	}

	cpu := calcFreeBSDCPU(s1, s2)
	if cpu == nil {
		t.Fatal("expected CPUStats, got nil")
	}
	if cpu.UsagePercent < 0 || cpu.UsagePercent > 100 {
		t.Errorf("UsagePercent out of range: %f", cpu.UsagePercent)
	}
	if cpu.IdlePercent < 0 || cpu.IdlePercent > 100 {
		t.Errorf("IdlePercent out of range: %f", cpu.IdlePercent)
	}
}

// TestParseFreeBSDSwapinfo verifies swapinfo -k parsing.
func TestParseFreeBSDSwapinfo(t *testing.T) {
	input := `Device          1K-blocks     Used    Avail Capacity  Type
/dev/ada0p2       2097152      512  2096640     0%    -`

	sw, err := parseFreeBSDSwapinfo(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sw == nil {
		t.Fatal("expected SwapStats, got nil")
	}
	if sw.TotalBytes != 2097152*1024 {
		t.Errorf("TotalBytes: expected %d, got %d", int64(2097152*1024), sw.TotalBytes)
	}
	if sw.UsedBytes != 512*1024 {
		t.Errorf("UsedBytes: expected %d, got %d", int64(512*1024), sw.UsedBytes)
	}
}

func TestParseFreeBSDSwapinfo_NoSwap(t *testing.T) {
	sw, err := parseFreeBSDSwapinfo("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sw != nil {
		t.Errorf("expected nil for empty input, got %+v", sw)
	}
}

// TestParseOpenBSDCPTime verifies OpenBSD kern.cp_time parsing.
func TestParseOpenBSDCPTime(t *testing.T) {
	input := "kern.cp_time=user=217735,nice=0,sys=167138,spin=0,intr=14268,idle=6855742"
	s, err := parseOpenBSDCPTime(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.user != 217735 {
		t.Errorf("user: expected 217735, got %d", s.user)
	}
	if s.sys != 167138 {
		t.Errorf("sys: expected 167138, got %d", s.sys)
	}
	if s.idle != 6855742 {
		t.Errorf("idle: expected 6855742, got %d", s.idle)
	}

	// Test CPU calc with two identical samples → 0% usage
	cpu := calcOpenBSDCPU(s, s)
	if cpu.UsagePercent != 0 {
		t.Errorf("UsagePercent for identical samples: expected 0, got %f", cpu.UsagePercent)
	}
}

// TestParseOpenBSDSwapctl verifies swapctl -s -k parsing.
func TestParseOpenBSDSwapctl(t *testing.T) {
	input := "total: 1048576 1K-blocks allocated, 512 used, 1048064 available"
	sw, err := parseOpenBSDSwapctl(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sw == nil {
		t.Fatal("expected SwapStats, got nil")
	}
	if sw.TotalBytes != 1048576*1024 {
		t.Errorf("TotalBytes: expected %d, got %d", int64(1048576*1024), sw.TotalBytes)
	}
	if sw.UsedBytes != 512*1024 {
		t.Errorf("UsedBytes: expected %d, got %d", int64(512*1024), sw.UsedBytes)
	}
	if sw.FreeBytes != 1048064*1024 {
		t.Errorf("FreeBytes: expected %d, got %d", int64(1048064*1024), sw.FreeBytes)
	}
}

func TestParseOpenBSDSwapctl_Empty(t *testing.T) {
	sw, err := parseOpenBSDSwapctl("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sw != nil {
		t.Errorf("expected nil for empty input, got %+v", sw)
	}
}

// itoa64 converts int64 to string for test helper use.
func itoa64(v int64) string {
	return strconv.FormatInt(v, 10)
}

// ─── Docker collector tests ──────────────────────────────────────────────────

// TestParseDockerPS verifies docker ps tab-delimited output parsing.
func TestParseDockerPS(t *testing.T) {
	input := "abc123def456\tnginx\tnginx:1.25\tUp 2 hours\n" +
		"def789abc012\tredis\tredis:7.0\tUp 3 days\n"

	containers, err := parseDockerPS(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(containers))
	}
	c := containers[0]
	if c.ID != "abc123def456" {
		t.Errorf("ID: expected abc123def456, got %s", c.ID)
	}
	if c.Name != "nginx" {
		t.Errorf("Name: expected nginx, got %s", c.Name)
	}
	if c.Image != "nginx:1.25" {
		t.Errorf("Image: expected nginx:1.25, got %s", c.Image)
	}
	if c.Status != "Up 2 hours" {
		t.Errorf("Status: expected 'Up 2 hours', got %s", c.Status)
	}
}

func TestParseDockerPS_Empty(t *testing.T) {
	containers, err := parseDockerPS("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(containers) != 0 {
		t.Errorf("expected 0 containers for empty input, got %d", len(containers))
	}
}

// TestParseDockerStats verifies docker stats tab-delimited output parsing.
func TestParseDockerStats(t *testing.T) {
	// IDCPUPercMemUsageNetIOBlockIO
	input := "abc123def456\t5.23%\t123MiB / 1GiB\t1.5kB / 2.3kB\t10MB / 5MB\n"

	stats, err := parseDockerStats(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	st, ok := stats["abc123def456"]
	if !ok {
		t.Fatal("expected entry for abc123def456")
	}
	if st.CPUPercent != 5.23 {
		t.Errorf("CPUPercent: expected 5.23, got %f", st.CPUPercent)
	}

	expectedMemUsed := int64(123 * 1024 * 1024)
	if st.MemUsedBytes != expectedMemUsed {
		t.Errorf("MemUsedBytes: expected %d, got %d", expectedMemUsed, st.MemUsedBytes)
	}

	expectedMemLimit := int64(1 * 1024 * 1024 * 1024)
	if st.MemLimitBytes != expectedMemLimit {
		t.Errorf("MemLimitBytes: expected %d, got %d", expectedMemLimit, st.MemLimitBytes)
	}

	if st.MemPercent <= 0 || st.MemPercent > 100 {
		t.Errorf("MemPercent out of range: %f", st.MemPercent)
	}
}

// TestParseDockerSize verifies human-readable size parsing.
func TestParseDockerSize(t *testing.T) {
	cases := []struct {
		input string
		want  int64
	}{
		{"0B", 0},
		{"1B", 1},
		{"1kB", 1000},
		{"1.5kB", 1500},
		{"1MB", 1_000_000},
		{"1GiB", 1024 * 1024 * 1024},
		{"256MiB", 256 * 1024 * 1024},
		{"--", 0},
		{"", 0},
	}
	for _, tc := range cases {
		got := parseDockerSize(tc.input)
		if got != tc.want {
			t.Errorf("parseDockerSize(%q): expected %d, got %d", tc.input, tc.want, got)
		}
	}
}

// TestCollectDocker_NonLinux verifies that collecting Docker on a non-Linux
// platform sets the capability to "unsupported" without error.
func TestCollectDocker_NonLinux(t *testing.T) {
	snap := &Snapshot{}
	CollectDocker(nil, nil, Darwin, snap)

	if snap.Caps["containers"] != string(CapUnsupported) {
		t.Errorf("expected containers capability=unsupported for Darwin, got %q", snap.Caps["containers"])
	}
}
