// Package config handles loading and validating the WatchSSH configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// AuthType specifies how to authenticate with an SSH server.
type AuthType string

const (
	// AuthTypeKey uses a private key file (default).
	AuthTypeKey AuthType = "key"
	// AuthTypePassword uses a plaintext password.
	AuthTypePassword AuthType = "password"
	// AuthTypeAgent delegates to the SSH agent via SSH_AUTH_SOCK.
	AuthTypeAgent AuthType = "agent"
)

// Auth holds authentication configuration for a server.
type Auth struct {
	Type       AuthType `yaml:"type"`
	KeyFile    string   `yaml:"key_file"`
	Passphrase string   `yaml:"passphrase"`
	Password   string   `yaml:"password"`
}

// PingCheck configures a ping connectivity test run from the monitoring machine.
type PingCheck struct {
	Enabled bool `yaml:"enabled"`
	Count   int  `yaml:"count"`   // number of ICMP packets (default 3)
	Timeout int  `yaml:"timeout"` // wait seconds before giving up (default 5)
}

// PortCheck configures a TCP reachability test run from the monitoring machine.
type PortCheck struct {
	Port    int `yaml:"port"`
	Timeout int `yaml:"timeout"` // dial timeout in seconds (default 5)
}

// HTTPCheck configures an HTTP health check run from the monitoring machine.
type HTTPCheck struct {
	URL            string `yaml:"url"`
	ExpectedStatus int    `yaml:"expected_status"` // default 200
	Timeout        int    `yaml:"timeout"`         // seconds (default 10)
}

// DockerConfig enables optional Docker container observability on Linux hosts.
// When enabled, WatchSSH runs `docker ps` and `docker stats --no-stream` to
// discover running containers and collect their resource usage. This feature
// is Linux-only; enabling it on other platforms results in a clear capability
// flag ("unsupported") rather than an error.
type DockerConfig struct {
	// Enabled controls whether Docker metrics are collected. Default: false.
	Enabled bool `yaml:"enabled"`
}

// CustomCheck configures an arbitrary SSH command check.
type CustomCheck struct {
	Name string `yaml:"name"`
	// Command is run on the remote host via SSH.
	Command string `yaml:"command"`
	// ExpectedOutput, if set, requires the command output to contain this string.
	ExpectedOutput string `yaml:"expected_output"`
}

// Checks holds all optional connectivity and custom checks for a server.
type Checks struct {
	Ping   PingCheck     `yaml:"ping"`
	Ports  []PortCheck   `yaml:"ports"`
	HTTP   []HTTPCheck   `yaml:"http"`
	Custom []CustomCheck `yaml:"custom"`
}

// Server holds connection details for one monitored host.
type Server struct {
	Name     string `yaml:"name"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Auth     Auth   `yaml:"auth"`

	// InsecureIgnoreHostKey disables host key verification.
	// WARNING: this is dangerous and should only be used for testing.
	// Default: false (strict host key checking is active).
	InsecureIgnoreHostKey bool `yaml:"insecure_ignore_host_key"`

	// Local, when true, runs all monitoring commands locally (no SSH).
	// Use this to monitor the machine running WatchSSH itself.
	Local  bool     `yaml:"local"`
	Tags   []string `yaml:"tags,omitempty"`
	Checks Checks   `yaml:"checks"`

	// Docker enables optional Docker container metrics collection.
	// Only effective on Linux targets; ignored on other platforms.
	Docker DockerConfig `yaml:"docker"`
}

// Output configures how collected metrics are presented.
type Output struct {
	// Type is one of "console" or "json".
	Type string `yaml:"type"`
	// File is an optional file path for JSON output; stdout is used when empty.
	File string `yaml:"file"`
}

// WebConfig configures the built-in HTTP monitoring dashboard.
type WebConfig struct {
	Enabled bool   `yaml:"enabled"`
	Listen  string `yaml:"listen"` // TCP address, default ":8080"
}

// EmailConfig holds SMTP settings for alert delivery.
type EmailConfig struct {
	SMTPHost string   `yaml:"smtp_host"`
	SMTPPort int      `yaml:"smtp_port"` // default 587
	Username string   `yaml:"username"`
	Password string   `yaml:"password"`
	From     string   `yaml:"from"`
	To       []string `yaml:"to"`
	// TLSMode controls transport security: "none", "starttls" (default), "tls".
	TLSMode string `yaml:"tls_mode"`
}

// AlertRule defines a threshold-based alert condition.
type AlertRule struct {
	Name string `yaml:"name"`
	// Metric: cpu_usage, mem_usage, swap_usage, load1, load5, load15,
	//         disk_usage, ping_latency, ping_failed, port_closed,
	//         http_failed, custom_failed.
	//         cert_expires_days.
	Metric string `yaml:"metric"`
	// Operator: ">", "<", ">=", "<=", "==", "!=".
	Operator  string  `yaml:"operator"`
	Threshold float64 `yaml:"threshold"`
	// MountPoint filters disk_usage to a specific mount (empty = any disk).
	MountPoint string `yaml:"mount_point"`
	// Port filters port_closed to a specific port number (0 = any port).
	Port int `yaml:"port"`
	// Servers limits the rule to named servers (empty = all servers).
	Servers []string `yaml:"servers"`
}

// AlertsConfig holds all alerting configuration.
type AlertsConfig struct {
	Rules  []AlertRule        `yaml:"rules"`
	Email  *EmailConfig       `yaml:"email"`
	Action *AlertActionConfig `yaml:"action"`
	// Cooldown is the minimum number of seconds between repeated alerts for
	// the same rule on the same server. Default: 3600.
	Cooldown int `yaml:"cooldown"`
}

// AlertActionConfig configures a guarded local command that runs when alerts fire.
type AlertActionConfig struct {
	// Command is executed directly (no shell) when one or more alerts fire.
	Command string `yaml:"command"`
	// AllowedExecutables is a required allowlist (e.g. ["systemctl", "/usr/bin/logger"]).
	AllowedExecutables []string `yaml:"allowed_executables"`
	// Timeout is the maximum runtime in seconds (default 10).
	Timeout int `yaml:"timeout"`
}

// Config is the root configuration structure.
type Config struct {
	// KnownHostsPath overrides the default ~/.ssh/known_hosts file.
	// Host key verification uses this file when StrictHostKeyChecking is true (default).
	KnownHostsPath string `yaml:"known_hosts_path"`

	// StrictHostKeyChecking enables strict host key verification (default: true).
	// When true, connections to hosts not in known_hosts will fail with a clear
	// error. Set insecure_ignore_host_key: true on a per-server basis instead
	// of disabling this globally.
	StrictHostKeyChecking *bool `yaml:"strict_host_key_checking"`

	Servers []Server `yaml:"servers"`
	// Interval is the polling period in seconds (default 60).
	Interval int `yaml:"interval"`
	// Timeout is the per-command SSH timeout in seconds (default 30).
	Timeout int `yaml:"timeout"`
	// Workers is the maximum number of servers polled concurrently.
	// Default: 0, which means one worker per server (unbounded).
	// Set to a positive integer to cap concurrency (e.g. 10).
	Workers int          `yaml:"workers"`
	Output  Output       `yaml:"output"`
	Web     WebConfig    `yaml:"web"`
	Alerts  AlertsConfig `yaml:"alerts"`
}

// IsStrictHostKeyChecking returns true unless explicitly set to false.
func (c *Config) IsStrictHostKeyChecking() bool {
	if c.StrictHostKeyChecking == nil {
		return true // secure by default
	}
	return *c.StrictHostKeyChecking
}

// LoadOrDefault reads the YAML configuration at path. If the file does not
// exist it returns a default configuration with the web dashboard enabled on
// :8080 so the user can configure WatchSSH interactively without first
// creating a config file manually.
func LoadOrDefault(path string) (*Config, error) {
	expanded := expandTilde(path)
	if _, err := os.Stat(expanded); os.IsNotExist(err) {
		cfg := &Config{}
		applyDefaults(cfg)
		cfg.Web.Enabled = true
		return cfg, nil
	}
	return Load(path)
}

// Save marshals cfg to YAML and writes it to path, creating or truncating the
// file. Permissions are set to 0600 to protect sensitive credentials.
func Save(cfg *Config, path string) error {
	path = expandTilde(path)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// Load reads and validates a YAML configuration file at the given path.
func Load(path string) (*Config, error) {
	path = expandTilde(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	applyDefaults(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Interval <= 0 {
		cfg.Interval = 60
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30
	}
	if cfg.Output.Type == "" {
		cfg.Output.Type = "console"
	}
	if cfg.Web.Listen == "" {
		cfg.Web.Listen = ":8080"
	}
	if cfg.Alerts.Cooldown <= 0 {
		cfg.Alerts.Cooldown = 3600
	}
	if cfg.Alerts.Action != nil && cfg.Alerts.Action.Timeout <= 0 {
		cfg.Alerts.Action.Timeout = 10
	}
	if cfg.Alerts.Email != nil {
		if cfg.Alerts.Email.SMTPPort == 0 {
			cfg.Alerts.Email.SMTPPort = 587
		}
		if cfg.Alerts.Email.TLSMode == "" {
			cfg.Alerts.Email.TLSMode = "starttls"
		}
	}

	for i := range cfg.Servers {
		srv := &cfg.Servers[i]
		if srv.Local {
			if srv.Name == "" {
				srv.Name = "localhost"
			}
		} else {
			if srv.Port == 0 {
				srv.Port = 22
			}
			if srv.Name == "" {
				srv.Name = srv.Host
			}
		}
		// Check defaults
		if srv.Checks.Ping.Count == 0 {
			srv.Checks.Ping.Count = 3
		}
		if srv.Checks.Ping.Timeout == 0 {
			srv.Checks.Ping.Timeout = 5
		}
		for j := range srv.Checks.Ports {
			if srv.Checks.Ports[j].Timeout == 0 {
				srv.Checks.Ports[j].Timeout = 5
			}
		}
		for j := range srv.Checks.HTTP {
			if srv.Checks.HTTP[j].ExpectedStatus == 0 {
				srv.Checks.HTTP[j].ExpectedStatus = 200
			}
			if srv.Checks.HTTP[j].Timeout == 0 {
				srv.Checks.HTTP[j].Timeout = 10
			}
		}
	}
}

func validate(cfg *Config) error {
	for i, srv := range cfg.Servers {
		if srv.Local {
			continue // local servers don't need host/username
		}
		if srv.Host == "" {
			return fmt.Errorf("server[%d] (%q): host is required", i, srv.Name)
		}
		if srv.Username == "" {
			return fmt.Errorf("server[%d] (%q): username is required", i, srv.Name)
		}
	}
	if cfg.Alerts.Action != nil {
		if strings.TrimSpace(cfg.Alerts.Action.Command) == "" {
			return fmt.Errorf("alerts.action.command is required when alerts.action is set")
		}
		if len(cfg.Alerts.Action.AllowedExecutables) == 0 {
			return fmt.Errorf("alerts.action.allowed_executables must include at least one executable")
		}
		parts := strings.Fields(cfg.Alerts.Action.Command)
		if len(parts) == 0 {
			return fmt.Errorf("alerts.action.command is invalid")
		}
		if !isAllowedExecutable(parts[0], cfg.Alerts.Action.AllowedExecutables) {
			return fmt.Errorf("alerts.action.command executable %q is not in alerts.action.allowed_executables", parts[0])
		}
	}
	return nil
}

func isAllowedExecutable(executable string, allowed []string) bool {
	exeBase := filepath.Base(executable)
	for _, entry := range allowed {
		v := strings.TrimSpace(entry)
		if v == "" {
			continue
		}
		if v == executable || filepath.Base(v) == exeBase {
			return true
		}
	}
	return false
}

// expandTilde replaces a leading "~" with the user's home directory.
func expandTilde(path string) string {
	if len(path) > 0 && path[0] == '~' {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}
