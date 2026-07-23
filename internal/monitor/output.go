package monitor

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OutputWriter accepts a slice of ServerMetrics and presents or stores them.
type OutputWriter interface {
	Write(metrics []ServerMetrics) error
}

// ---------------------------------------------------------------------------
// ConsoleWriter
// ---------------------------------------------------------------------------

// ConsoleWriter renders metrics as a human-readable box-drawn table. On an
// interactive terminal it refreshes the preceding snapshot in place; pipes and
// files keep conventional append-only output for logs and automation.
type ConsoleWriter struct {
	out          io.Writer
	interactive  bool
	previousRows int
	width        int
	mu           sync.Mutex
}

// NewConsoleWriter uses stdout and detects whether it is an interactive
// terminal. Tests and embedding callers may construct ConsoleWriter directly.
func NewConsoleWriter() *ConsoleWriter {
	interactive := stdoutIsTerminal()
	return &ConsoleWriter{out: os.Stdout, interactive: interactive, width: consoleWidth(interactive)}
}

const (
	minimumConsoleWidth = 70
	defaultConsoleWidth = 100
	maximumConsoleWidth = 180
)

func (w *ConsoleWriter) Write(metrics []ServerMetrics) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.out == nil {
		w.out = os.Stdout
		w.interactive = stdoutIsTerminal()
	}
	if w.width == 0 || w.interactive {
		w.width = consoleWidth(w.interactive)
	}
	var snapshot strings.Builder
	for _, m := range metrics {
		snapshot.WriteString(renderServerMetrics(m, w.width))
	}
	output := snapshot.String()
	if w.interactive && w.previousRows > 0 {
		// Move to the first row of the previous snapshot and erase only the
		// display region below it, leaving earlier shell output intact.
		fmt.Fprintf(w.out, "\x1b[%dA\x1b[J", w.previousRows)
	}
	if _, err := io.WriteString(w.out, output); err != nil {
		return err
	}
	if w.interactive {
		w.previousRows = strings.Count(output, "\n")
	}
	return nil
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// consoleWidth uses COLUMNS when the shell exposes it. This works across the
// supported platforms without adding a terminal-specific dependency. Keep a
// sensible bounded fallback for serial terminals and redirected output.
func consoleWidth(interactive bool) int {
	if !interactive {
		return defaultConsoleWidth
	}
	columns, err := strconv.Atoi(os.Getenv("COLUMNS"))
	if err != nil || columns <= 0 {
		return defaultConsoleWidth
	}
	width := columns - 4 // leave a small terminal margin
	if width < minimumConsoleWidth {
		return minimumConsoleWidth
	}
	if width > maximumConsoleWidth {
		return maximumConsoleWidth
	}
	return width
}

func renderServerMetrics(m ServerMetrics, boxWidth int) string {
	var sb strings.Builder
	sep := strings.Repeat("─", boxWidth)

	line := func(text string) {
		runes := []rune(text)
		inner := boxWidth - 2
		if len(runes) > inner {
			runes = runes[:inner]
		}
		padded := string(runes) + strings.Repeat(" ", inner-len(runes))
		sb.WriteString("│ " + padded + " │\n")
	}
	divider := func() { sb.WriteString("├" + sep + "┤\n") }

	sb.WriteString("┌" + sep + "┐\n")
	if m.Host != "" {
		line(fmt.Sprintf("Server : %s  (%s)", m.ServerName, m.Host))
	} else {
		line(fmt.Sprintf("Server : %s  (local)", m.ServerName))
	}
	line(fmt.Sprintf("Time   : %s", m.Timestamp.Format(time.RFC3339)))
	if m.Platform != "" {
		line(fmt.Sprintf("OS     : %s", m.Platform))
	}
	divider()

	if m.Error != "" {
		line(fmt.Sprintf("ERROR  : %s", m.Error))
		sb.WriteString("└" + sep + "┘\n\n")
		return sb.String()
	}

	// System info
	if m.System.OS != "" || m.System.Kernel != "" {
		line(fmt.Sprintf("OS     : %s  Kernel: %s  Arch: %s",
			nvl(m.System.OS, "n/a"), nvl(m.System.Kernel, "n/a"), nvl(m.System.Arch, "n/a")))
	}
	hostname := nvl(m.System.Hostname, "n/a")
	uptime := "n/a"
	if m.Load != nil {
		uptime = formatUptime(m.Load.UptimeSeconds)
	}
	if m.System.CPUCores > 0 {
		line(fmt.Sprintf("Host   : %s   Cores: %d   Uptime: %s", hostname, m.System.CPUCores, uptime))
	} else {
		line(fmt.Sprintf("Host   : %s   Uptime: %s", hostname, uptime))
	}
	divider()

	// Load & CPU
	if m.Load != nil {
		if m.Load.TotalProcesses > 0 {
			line(fmt.Sprintf("Load   : %.2f  %.2f  %.2f  (run %d/%d, last PID %d)",
				m.Load.Load1, m.Load.Load5, m.Load.Load15,
				m.Load.RunningProcesses, m.Load.TotalProcesses, m.Load.LastPID))
		} else {
			line(fmt.Sprintf("Load   : %.2f  %.2f  %.2f  (1/5/15 min)",
				m.Load.Load1, m.Load.Load5, m.Load.Load15))
		}
	} else {
		line("Load   : n/a")
	}
	if m.CPU != nil {
		line(fmt.Sprintf("CPU    : %.1f%%  (user %.1f%%  sys %.1f%%  iowait %.1f%%  idle %.1f%%)",
			m.CPU.UsagePercent, m.CPU.UserPercent, m.CPU.SystemPercent,
			m.CPU.IOWaitPercent, m.CPU.IdlePercent))
	} else {
		line("CPU    : n/a")
	}
	divider()

	// Memory
	if m.Memory != nil {
		line(fmt.Sprintf("RAM    : %s / %s  (%.1f%%)",
			formatBytes(m.Memory.UsedBytes), formatBytes(m.Memory.TotalBytes), m.Memory.UsagePercent))
	} else {
		line("RAM    : n/a")
	}
	if m.Swap != nil {
		line(fmt.Sprintf("Swap   : %s / %s  (%.1f%%)",
			formatBytes(m.Swap.UsedBytes), formatBytes(m.Swap.TotalBytes), m.Swap.Percent))
	}
	if m.FileDescriptors != nil {
		line(fmt.Sprintf("FDs    : %d / %d  (%.1f%%, %d unused)",
			fileDescriptorsInUse(*m.FileDescriptors), m.FileDescriptors.Max,
			m.FileDescriptors.UsagePercent, m.FileDescriptors.Unused))
	}
	divider()

	// Disks
	if len(m.Disks) > 0 {
		line("Disks  :")
		barWidth := 20
		if boxWidth >= 120 {
			barWidth = 30
		}
		for _, d := range m.Disks {
			bar := usageBar(d.UsagePercent, barWidth)
			line(fmt.Sprintf("  %-20s %-20s %s %s / %s (%.0f%%)",
				truncate(d.Device, 20), truncate(d.MountPoint, 20),
				bar,
				formatBytes(d.UsedBytes), formatBytes(d.TotalBytes),
				d.UsagePercent))
			if d.InodesTotal > 0 {
				line(fmt.Sprintf("  %-20s %-20s inodes %d / %d (%.0f%%)",
					"", "", d.InodesUsed, d.InodesTotal, d.InodesUsagePercent))
			}
		}
		divider()
	}

	// Inodes
	if len(m.Inodes) > 0 {
		line("Inodes :")
		for _, i := range m.Inodes {
			line(fmt.Sprintf("  %-16s %-12s %d / %d (%.0f%%)",
				truncate(i.Device, 16), truncate(i.MountPoint, 12),
				i.UsedInodes, i.TotalInodes, i.UsagePercent))
		}
		divider()
	}

	// Network
	if len(m.Network) > 0 {
		line("Network:")
		entries := make([]string, 0, len(m.Network))
		for _, n := range m.Network {
			if n.BytesRecv == 0 && n.BytesSent == 0 {
				continue
			}
			extras := ""
			errs := n.ErrorsRecv + n.ErrorsSent
			drops := n.DropsRecv + n.DropsSent
			if errs > 0 || drops > 0 {
				extras = fmt.Sprintf("  err %d  drop %d", errs, drops)
			}
			entries = append(entries, fmt.Sprintf("%-10s rx %-9s tx %-9s%s",
				truncate(n.Interface, 10),
				formatBytes(n.BytesRecv), formatBytes(n.BytesSent), extras))
		}
		for len(entries) > 0 {
			if boxWidth >= 120 && len(entries) > 1 {
				line("  " + entries[0] + "    " + entries[1])
				entries = entries[2:]
			} else {
				line("  " + entries[0])
				entries = entries[1:]
			}
		}
		divider()
	}

	// Logged-in users
	if len(m.Users) > 0 {
		line("Users  :")
		for _, u := range m.Users {
			remote := ""
			if u.Host != "" {
				remote = " from " + u.Host
			}
			line(fmt.Sprintf("  %-12s %-10s %s%s",
				truncate(u.User, 12), truncate(u.TTY, 10), truncate(u.LoginTime, 24), remote))
		}
		divider()
	}

	// Connectivity
	cs := m.Connectivity
	hasConn := cs.PingEnabled || len(cs.Ports) > 0 || len(cs.HTTP) > 0
	if hasConn {
		line("Checks :")
		if cs.PingEnabled {
			if cs.PingOK {
				line(fmt.Sprintf("  Ping        OK  (%.1f ms)", cs.PingLatency))
			} else {
				line("  Ping        FAILED")
			}
		}
		for _, p := range cs.Ports {
			status := "OPEN"
			if !p.Open {
				status = "CLOSED"
			}
			line(fmt.Sprintf("  Port %-5d  %s", p.Port, status))
		}
		for _, h := range cs.HTTP {
			status := "OK"
			if !h.OK {
				status = fmt.Sprintf("FAILED (HTTP %d)", h.StatusCode)
			}
			line(fmt.Sprintf("  HTTP  %s  →  %s  (%.0f ms)", truncate(h.URL, 30), status, h.LatencyMs))
		}
		divider()
	}

	// Custom checks
	if len(m.CustomChecks) > 0 {
		line("Custom :")
		for _, cc := range m.CustomChecks {
			status := "OK"
			if !cc.OK {
				status = "FAILED"
			}
			line(fmt.Sprintf("  %-20s  %s", truncate(cc.Name, 20), status))
		}
		divider()
	}

	// Processes
	if len(m.Processes) > 0 {
		line(fmt.Sprintf("%-8s %-12s %5s %5s  %s", "PID", "USER", "CPU%", "MEM%", "COMMAND"))
		for _, p := range m.Processes {
			line(fmt.Sprintf("%-8d %-12s %5.1f %5.1f  %s",
				p.PID, truncate(p.User, 12), p.CPUPercent, p.MemPercent,
				truncate(p.Command, 30)))
		}
	}

	// Containers displays Docker container stats when collected (Linux only).
	if len(m.Containers) > 0 {
		divider()
		line("Containers (Docker):")
		line(fmt.Sprintf("  %-12s %-20s %6s %-21s %s", "ID", "NAME", "CPU%", "MEM", "IMAGE"))
		for _, c := range m.Containers {
			memStr := fmt.Sprintf("%s/%s", formatBytes(c.MemUsedBytes), formatBytes(c.MemLimitBytes))
			line(fmt.Sprintf("  %-12s %-20s %5.1f%% %-21s %s",
				truncate(c.ID, 12), truncate(c.Name, 20), c.CPUPercent,
				memStr, truncate(c.Image, 20)))
		}
	}

	sb.WriteString("└" + sep + "┘\n\n")
	return sb.String()
}

// nvl returns s if non-empty, otherwise fallback.
func nvl(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// usageBar renders a simple ASCII progress bar of the given width.
func usageBar(pct float64, width int) string {
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}

// formatBytes converts a byte count to a human-readable string (e.g. "4.2 GiB").
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// formatUptime converts seconds to a human-readable duration string.
func formatUptime(sec float64) string {
	d := time.Duration(sec) * time.Second
	days := int(d.Hours()) / 24
	d -= time.Duration(days*24) * time.Hour
	if days > 0 {
		return fmt.Sprintf("%dd %s", days, d.Round(time.Second))
	}
	return d.Round(time.Second).String()
}

func fileDescriptorsInUse(fd FileDescriptorStats) int64 {
	used := fd.Allocated - fd.Unused
	if used < 0 {
		return 0
	}
	return used
}

// truncate shortens s to at most n runes, appending "…" if truncated.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// ---------------------------------------------------------------------------
// JSONWriter
// ---------------------------------------------------------------------------

// JSONWriter serialises metrics as indented JSON. When File is set the output
// is written to that path; otherwise it goes to stdout.
type JSONWriter struct {
	file string
}

func (w *JSONWriter) Write(metrics []ServerMetrics) error {
	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling JSON: %w", err)
	}
	if w.file != "" {
		return os.WriteFile(w.file, data, 0600)
	}
	fmt.Println(string(data))
	return nil
}
