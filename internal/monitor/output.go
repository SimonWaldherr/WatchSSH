package monitor

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// OutputWriter accepts a slice of ServerMetrics and presents or stores them.
type OutputWriter interface {
	Write(metrics []ServerMetrics) error
}

// ---------------------------------------------------------------------------
// ConsoleWriter
// ---------------------------------------------------------------------------

// ConsoleWriter renders metrics as a human-readable box-drawn table on stdout.
type ConsoleWriter struct{}

const boxWidth = 70

func (w *ConsoleWriter) Write(metrics []ServerMetrics) error {
	for _, m := range metrics {
		fmt.Print(renderServerMetrics(m))
	}
	return nil
}

func renderServerMetrics(m ServerMetrics) string {
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
	line(fmt.Sprintf("Host   : %s   Uptime: %s", hostname, uptime))
	divider()

	// Load & CPU
	if m.Load != nil {
		line(fmt.Sprintf("Load   : %.2f  %.2f  %.2f  (1/5/15 min)",
			m.Load.Load1, m.Load.Load5, m.Load.Load15))
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
	divider()

	// Disks
	if len(m.Disks) > 0 {
		line("Disks  :")
		for _, d := range m.Disks {
			bar := usageBar(d.UsagePercent, 20)
			line(fmt.Sprintf("  %-16s %-12s %s %s / %s (%.0f%%)",
				truncate(d.Device, 16), truncate(d.MountPoint, 12),
				bar,
				formatBytes(d.UsedBytes), formatBytes(d.TotalBytes),
				d.UsagePercent))
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
		for _, n := range m.Network {
			if n.BytesRecv == 0 && n.BytesSent == 0 {
				continue
			}
			line(fmt.Sprintf("  %-10s  rx %s  tx %s",
				truncate(n.Interface, 10),
				formatBytes(n.BytesRecv), formatBytes(n.BytesSent)))
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
