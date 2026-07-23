package monitor

import (
	"fmt"
	"sort"
	"time"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
)

// AssetRecord is a non-secret inventory row derived from configured targets
// and the latest successful collection. It is safe to expose in the dashboard.
type AssetRecord struct {
	Name         string    `json:"name"`
	Host         string    `json:"host"`
	Platform     string    `json:"platform,omitempty"`
	Architecture string    `json:"architecture,omitempty"`
	Hostname     string    `json:"hostname,omitempty"`
	Tags         []string  `json:"tags,omitempty"`
	DependsOn    []string  `json:"depends_on,omitempty"`
	ProxyJump    bool      `json:"proxy_jump"`
	Local        bool      `json:"local"`
	LastSeen     time.Time `json:"last_seen,omitempty"`
	Tools        []string  `json:"tools,omitempty"`
}

// SecurityFinding describes a configuration or observed security condition.
// It deliberately avoids credentials and remote command output.
type SecurityFinding struct {
	Server   string `json:"server"`
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Summary  string `json:"summary"`
}

// BuildInventory combines target configuration with current, non-secret facts.
func BuildInventory(cfg config.Config, metrics []ServerMetrics) []AssetRecord {
	byName := make(map[string]ServerMetrics, len(metrics))
	for _, metric := range metrics {
		byName[metric.ServerName] = metric
	}
	assets := make([]AssetRecord, 0, len(cfg.Servers))
	for _, server := range cfg.Servers {
		asset := AssetRecord{Name: server.Name, Host: server.Host, Tags: append([]string(nil), server.Tags...), DependsOn: append([]string(nil), server.DependsOn...), ProxyJump: server.ProxyJump != nil, Local: server.Local}
		if server.Local && asset.Host == "" {
			asset.Host = "local"
		}
		if metric, ok := byName[server.Name]; ok {
			asset.Platform = metric.Platform
			asset.Architecture = metric.System.Arch
			asset.Hostname = metric.System.Hostname
			asset.LastSeen = metric.Timestamp
			for tool, available := range metric.StandardTools {
				if available {
					asset.Tools = append(asset.Tools, tool)
				}
			}
			sort.Strings(asset.Tools)
		}
		assets = append(assets, asset)
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].Name < assets[j].Name })
	return assets
}

// BuildSecurityFindings checks SSH posture and observed TLS health without
// needing extra agents or remote scanners.
func BuildSecurityFindings(cfg config.Config, metrics []ServerMetrics) []SecurityFinding {
	findings := make([]SecurityFinding, 0)
	for _, server := range cfg.Servers {
		if !server.Local && (!cfg.IsStrictHostKeyChecking() || server.InsecureIgnoreHostKey) {
			findings = append(findings, SecurityFinding{Server: server.Name, Severity: "warning", Code: "ssh_host_key_verification_disabled", Summary: "SSH host key verification is disabled."})
		}
		if !server.Local && (server.Auth.Type == config.AuthTypePassword || server.Auth.Type == config.AuthTypeKeyboardInteractive) {
			findings = append(findings, SecurityFinding{Server: server.Name, Severity: "info", Code: "ssh_password_authentication", Summary: "SSH password-based authentication is configured; use keys or certificates where practical."})
		}
	}
	for _, metric := range metrics {
		for _, tls := range metric.Connectivity.TLS {
			if !tls.OK {
				findings = append(findings, SecurityFinding{Server: metric.ServerName, Severity: "warning", Code: "tls_probe_failed", Summary: "TLS probe failed: " + tls.Error})
				continue
			}
			if tls.CertExpiresDays != nil && *tls.CertExpiresDays >= 0 && *tls.CertExpiresDays <= 30 {
				findings = append(findings, SecurityFinding{Server: metric.ServerName, Severity: "warning", Code: "tls_certificate_expiring", Summary: "TLS certificate expires in " + formatDays(*tls.CertExpiresDays) + "."})
			}
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return findings[i].Severity < findings[j].Severity
		}
		return findings[i].Server < findings[j].Server
	})
	return findings
}

func formatDays(days float64) string {
	return fmt.Sprintf("%.0f days", days)
}

// SuppressDependentFirings prevents downstream noise while an explicitly
// declared upstream target is unreachable. Suppressed alerts are not routed.
func SuppressDependentFirings(firings []Firing, metrics []ServerMetrics, cfg config.Config) []Firing {
	byName := make(map[string]ServerMetrics, len(metrics))
	for _, metric := range metrics {
		byName[metric.ServerName] = metric
	}
	dependencies := make(map[string][]string, len(cfg.Servers))
	for _, server := range cfg.Servers {
		dependencies[server.Name] = server.DependsOn
	}
	active := make([]Firing, 0, len(firings))
	for _, firing := range firings {
		suppressed := false
		for _, dependency := range dependencies[firing.Server] {
			metric, exists := byName[dependency]
			if !exists || metric.Error != "" || (metric.Connectivity.PingEnabled && !metric.Connectivity.PingOK) {
				suppressed = true
				break
			}
		}
		if !suppressed {
			active = append(active, firing)
		}
	}
	return active
}
