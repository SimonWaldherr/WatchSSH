// Package config handles loading and validating the WatchSSH configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

// AuthType specifies how to authenticate with an SSH server.
type AuthType string

const (
	// AuthTypeKey uses a private key file (default).
	AuthTypeKey AuthType = "key"
	// AuthTypePassword uses a password supplied inline or from a secret source.
	AuthTypePassword AuthType = "password"
	// AuthTypeKeyboardInteractive answers SSH keyboard-interactive prompts with a password.
	AuthTypeKeyboardInteractive AuthType = "keyboard-interactive"
	// AuthTypeAgent delegates to the SSH agent via SSH_AUTH_SOCK.
	AuthTypeAgent AuthType = "agent"
)

// SecretSource resolves sensitive material without storing it in config.yaml.
// Exactly one source may be set. File values have one trailing line ending
// removed; environment and Vault values are used verbatim.
type SecretSource struct {
	Env     string         `yaml:"env"`
	File    string         `yaml:"file"`
	VaultKV *VaultKVSource `yaml:"vault_kv"`
}

// VaultKVSource identifies one field in a HashiCorp Vault KV secret.
// Version defaults to the global Vault configuration (and then KV v2).
type VaultKVSource struct {
	Mount   string `yaml:"mount"`
	Path    string `yaml:"path"`
	Field   string `yaml:"field"`
	Version int    `yaml:"version"`
}

// VaultConfig configures the shared HashiCorp Vault connection used by
// VaultKV secret sources. Tokens are supplied from a file or environment
// variable, never as plaintext YAML.
type VaultConfig struct {
	Address   string `yaml:"address"`
	TokenEnv  string `yaml:"token_env"`
	TokenFile string `yaml:"token_file"`
	Namespace string `yaml:"namespace"`
	KVVersion int    `yaml:"kv_version"` // default 2; supports 1 and 2
}

// SecretsConfig configures secret backends shared by server authentication.
type SecretsConfig struct {
	Vault *VaultConfig `yaml:"vault"`
}

// Auth holds authentication configuration for a server.
type Auth struct {
	Type             AuthType     `yaml:"type"`
	KeyFile          string       `yaml:"key_file"`
	PrivateKey       SecretSource `yaml:"private_key"`
	Passphrase       string       `yaml:"passphrase"`
	PassphraseSource SecretSource `yaml:"passphrase_source"`
	CertificateFile  string       `yaml:"certificate_file"`
	Certificate      SecretSource `yaml:"certificate"`
	Password         string       `yaml:"password"`
	PasswordSource   SecretSource `yaml:"password_source"`
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

// BannerCheck verifies a greeting sent immediately after a TCP connection,
// for example SSH ("SSH-"), SMTP ("220"), or Redis ("+PONG").
type BannerCheck struct {
	Name           string `yaml:"name"`
	Host           string `yaml:"host"`
	Port           int    `yaml:"port"`
	ExpectedPrefix string `yaml:"expected_prefix"`
	Timeout        int    `yaml:"timeout"` // seconds (default 5)
}

// HTTPCheck configures an HTTP health check run from the monitoring machine.
type HTTPCheck struct {
	URL            string `yaml:"url"`
	Method         string `yaml:"method"`          // GET (default), HEAD, or another HTTP method without a request body
	ExpectedStatus int    `yaml:"expected_status"` // default 200
	ExpectedBody   string `yaml:"expected_body"`   // optional substring required in the response body
	Timeout        int    `yaml:"timeout"`         // seconds (default 10)
}

// DNSCheck configures a DNS lookup probe run from the monitoring machine.
type DNSCheck struct {
	Name           string `yaml:"name"`
	Host           string `yaml:"host"`
	Type           string `yaml:"type"`            // A, AAAA, CNAME, MX, TXT (default A)
	Server         string `yaml:"server"`          // optional resolver host or host:port
	ExpectedAnswer string `yaml:"expected_answer"` // optional substring match
	Timeout        int    `yaml:"timeout"`         // seconds (default 5)
}

// TracerouteCheck configures a traceroute probe run from the monitoring machine.
type TracerouteCheck struct {
	Name    string `yaml:"name"`
	Host    string `yaml:"host"`
	MaxHops int    `yaml:"max_hops"` // default 30
	Timeout int    `yaml:"timeout"`  // seconds (default 10)
}

// TLSCheck configures a TLS certificate probe run from the monitoring machine.
type TLSCheck struct {
	Name       string `yaml:"name"`
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`        // default 443
	ServerName string `yaml:"server_name"` // defaults to Host
	Timeout    int    `yaml:"timeout"`     // seconds (default 5)
}

// NTPCheck configures an SNTP time probe run from the monitoring machine.
// MaxOffsetMs is optional; when set, larger clock offsets make the probe fail.
type NTPCheck struct {
	Name        string  `yaml:"name"`
	Host        string  `yaml:"host"`
	Port        int     `yaml:"port"`          // default 123
	MaxOffsetMs float64 `yaml:"max_offset_ms"` // 0 disables offset validation
	Timeout     int     `yaml:"timeout"`       // seconds (default 5)
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
	Ping   PingCheck         `yaml:"ping"`
	Ports  []PortCheck       `yaml:"ports"`
	Banner []BannerCheck     `yaml:"banner"`
	HTTP   []HTTPCheck       `yaml:"http"`
	DNS    []DNSCheck        `yaml:"dns"`
	Trace  []TracerouteCheck `yaml:"traceroute"`
	TLS    []TLSCheck        `yaml:"tls"`
	NTP    []NTPCheck        `yaml:"ntp"`
	Custom []CustomCheck     `yaml:"custom"`
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

// StorageConfig configures optional persistence for metric and alert history.
type StorageConfig struct {
	// Type is one of "none" or "tinysql". "none" keeps WatchSSH stateless.
	Type string `yaml:"type"`
	// Path is the file path used by the tinySQL backend.
	Path string `yaml:"path"`
	// RetentionDays removes history records older than this many days. 0 disables age-based retention.
	RetentionDays int `yaml:"retention_days"`
	// MaxSizeMB trims oldest history records after the database file grows beyond this size. 0 disables size-based retention.
	MaxSizeMB int `yaml:"max_size_mb"`
}

// WebAuthConfig protects the dashboard, APIs, and Prometheus endpoint with
// HTTP Basic Authentication. PasswordHash must be a bcrypt hash, never a
// plaintext password. Liveness and readiness endpoints remain unauthenticated.
type WebAuthConfig struct {
	Username     string `yaml:"username"`
	PasswordHash string `yaml:"password_hash"`
}

// WebConfig configures the built-in HTTP monitoring dashboard.
type WebConfig struct {
	Enabled bool           `yaml:"enabled"`
	Listen  string         `yaml:"listen"` // TCP address, default ":8080"
	Auth    *WebAuthConfig `yaml:"auth"`
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
	//         disk_usage, ping_latency, ping_loss, ping_failed, port_closed,
	//         port_latency, banner_failed, banner_latency, http_failed,
	//         http_latency, dns_failed, dns_latency, traceroute_failed,
	//         traceroute_hops, tls_failed, tls_latency, ntp_failed,
	//         ntp_latency, ntp_offset, custom_failed.
	//         cert_expires_days, tls_cert_expires_days,
	//         board_temperature, board_under_voltage, board_throttled,
	//         board_wifi_rssi.
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

// WebhookConfig defines one outbound HTTP alert integration. HeaderEnv maps
// header names to environment-variable names so API tokens do not need to be
// stored in YAML. BodyTemplate is an optional Go text/template; it receives
// Route, Alerts, Summary, and FiredAt, and provides a json helper function.
type WebhookConfig struct {
	URL          string            `yaml:"url"`
	URLEnv       string            `yaml:"url_env"`
	Method       string            `yaml:"method"` // default POST
	Headers      map[string]string `yaml:"headers"`
	HeaderEnv    map[string]string `yaml:"header_env"`
	BodyTemplate string            `yaml:"body_template"`
	Timeout      int               `yaml:"timeout"` // seconds, default 10
}

// IRCConfig sends concise alert messages to an IRC channel. PasswordEnv is
// optional and is used for server passwords without storing them in YAML.
type IRCConfig struct {
	Address     string `yaml:"address"`
	TLS         bool   `yaml:"tls"`
	Nick        string `yaml:"nick"`
	Username    string `yaml:"username"`
	RealName    string `yaml:"real_name"`
	PasswordEnv string `yaml:"password_env"`
	Channel     string `yaml:"channel"`
	Timeout     int    `yaml:"timeout"` // seconds, default 10
}

// SyslogConfig sends RFC 5424 alert records to a local or central syslog
// receiver over UDP or TCP.
type SyslogConfig struct {
	Address string `yaml:"address"`
	Network string `yaml:"network"` // udp (default) or tcp
	AppName string `yaml:"app_name"`
	Timeout int    `yaml:"timeout"` // seconds, default 10
}

// AlertRoute sends matching alert firings to one protocol target. All match fields are
// optional; an empty route matches every firing. Continue permits later routes
// to receive the same firing, enabling explicit fan-out or escalation chains.
type AlertRoute struct {
	Name     string        `yaml:"name"`
	Rules    []string      `yaml:"rules"`
	Metrics  []string      `yaml:"metrics"`
	Servers  []string      `yaml:"servers"`
	Continue bool          `yaml:"continue"`
	Webhook  WebhookConfig `yaml:"webhook"`
	IRC      *IRCConfig    `yaml:"irc"`
	Syslog   *SyslogConfig `yaml:"syslog"`
}

// AlertsConfig holds all alerting configuration.
type AlertsConfig struct {
	Rules  []AlertRule        `yaml:"rules"`
	Routes []AlertRoute       `yaml:"routes"`
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
	Workers int           `yaml:"workers"`
	Output  Output        `yaml:"output"`
	Storage StorageConfig `yaml:"storage"`
	Web     WebConfig     `yaml:"web"`
	Secrets SecretsConfig `yaml:"secrets"`
	Alerts  AlertsConfig  `yaml:"alerts"`
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
	if cfg.Storage.Type == "" {
		cfg.Storage.Type = "none"
	}
	if cfg.Storage.Type == "tinysql" && cfg.Storage.Path == "" {
		cfg.Storage.Path = "watchssh.tinysql"
	}
	if cfg.Storage.Type == "tinysql" && cfg.Storage.RetentionDays == 0 {
		cfg.Storage.RetentionDays = 30
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
	for i := range cfg.Alerts.Routes {
		if cfg.Alerts.Routes[i].Webhook.Method == "" {
			cfg.Alerts.Routes[i].Webhook.Method = "POST"
		}
		if cfg.Alerts.Routes[i].Webhook.Timeout == 0 {
			cfg.Alerts.Routes[i].Webhook.Timeout = 10
		}
		if cfg.Alerts.Routes[i].IRC != nil {
			irc := cfg.Alerts.Routes[i].IRC
			if irc.Timeout == 0 {
				irc.Timeout = 10
			}
			if irc.Username == "" {
				irc.Username = irc.Nick
			}
			if irc.RealName == "" {
				irc.RealName = "WatchSSH"
			}
		}
		if cfg.Alerts.Routes[i].Syslog != nil {
			syslog := cfg.Alerts.Routes[i].Syslog
			if syslog.Timeout == 0 {
				syslog.Timeout = 10
			}
			if syslog.Network == "" {
				syslog.Network = "udp"
			}
			if syslog.AppName == "" {
				syslog.AppName = "watchssh"
			}
		}
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
		for j := range srv.Checks.Banner {
			if srv.Checks.Banner[j].Timeout == 0 {
				srv.Checks.Banner[j].Timeout = 5
			}
		}
		for j := range srv.Checks.HTTP {
			if srv.Checks.HTTP[j].Method == "" {
				srv.Checks.HTTP[j].Method = "GET"
			}
			if srv.Checks.HTTP[j].ExpectedStatus == 0 {
				srv.Checks.HTTP[j].ExpectedStatus = 200
			}
			if srv.Checks.HTTP[j].Timeout == 0 {
				srv.Checks.HTTP[j].Timeout = 10
			}
		}
		for j := range srv.Checks.DNS {
			if srv.Checks.DNS[j].Type == "" {
				srv.Checks.DNS[j].Type = "A"
			}
			if srv.Checks.DNS[j].Timeout == 0 {
				srv.Checks.DNS[j].Timeout = 5
			}
		}
		for j := range srv.Checks.Trace {
			if srv.Checks.Trace[j].MaxHops == 0 {
				srv.Checks.Trace[j].MaxHops = 30
			}
			if srv.Checks.Trace[j].Timeout == 0 {
				srv.Checks.Trace[j].Timeout = 10
			}
		}
		for j := range srv.Checks.TLS {
			if srv.Checks.TLS[j].Port == 0 {
				srv.Checks.TLS[j].Port = 443
			}
			if srv.Checks.TLS[j].Timeout == 0 {
				srv.Checks.TLS[j].Timeout = 5
			}
		}
		for j := range srv.Checks.NTP {
			if srv.Checks.NTP[j].Port == 0 {
				srv.Checks.NTP[j].Port = 123
			}
			if srv.Checks.NTP[j].Timeout == 0 {
				srv.Checks.NTP[j].Timeout = 5
			}
		}
	}
}

func validate(cfg *Config) error {
	switch cfg.Storage.Type {
	case "", "none", "tinysql":
	default:
		return fmt.Errorf("storage.type must be one of none or tinysql")
	}
	if cfg.Storage.Type == "tinysql" && strings.TrimSpace(cfg.Storage.Path) == "" {
		return fmt.Errorf("storage.path is required when storage.type is tinysql")
	}
	if cfg.Storage.RetentionDays < 0 {
		return fmt.Errorf("storage.retention_days must be >= 0")
	}
	if cfg.Storage.MaxSizeMB < 0 {
		return fmt.Errorf("storage.max_size_mb must be >= 0")
	}
	if cfg.Web.Auth != nil {
		if strings.TrimSpace(cfg.Web.Auth.Username) == "" {
			return fmt.Errorf("web.auth.username is required when web.auth is set")
		}
		if strings.TrimSpace(cfg.Web.Auth.PasswordHash) == "" {
			return fmt.Errorf("web.auth.password_hash is required when web.auth is set")
		}
		if _, err := bcrypt.Cost([]byte(cfg.Web.Auth.PasswordHash)); err != nil {
			return fmt.Errorf("web.auth.password_hash must be a valid bcrypt hash: %w", err)
		}
	}
	if err := validateVaultConfig(cfg.Secrets.Vault); err != nil {
		return err
	}
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
		if err := validateAuth(srv.Auth, cfg.Secrets.Vault); err != nil {
			return fmt.Errorf("server[%d] (%q): auth: %w", i, srv.Name, err)
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
	if err := validateAlertRoutes(cfg.Alerts.Routes); err != nil {
		return err
	}
	return nil
}

func validateAlertRoutes(routes []AlertRoute) error {
	names := make(map[string]struct{}, len(routes))
	for i, route := range routes {
		name := strings.TrimSpace(route.Name)
		if name == "" {
			return fmt.Errorf("alerts.routes[%d].name is required", i)
		}
		if _, exists := names[name]; exists {
			return fmt.Errorf("alerts.routes[%d].name %q is duplicated", i, name)
		}
		names[name] = struct{}{}
		targets := 0
		if strings.TrimSpace(route.Webhook.URL) != "" || strings.TrimSpace(route.Webhook.URLEnv) != "" {
			targets++
			if (strings.TrimSpace(route.Webhook.URL) == "") == (strings.TrimSpace(route.Webhook.URLEnv) == "") {
				return fmt.Errorf("alerts.routes[%d].webhook requires exactly one of url or url_env", i)
			}
		}
		if route.IRC != nil {
			targets++
			if strings.TrimSpace(route.IRC.Address) == "" || strings.TrimSpace(route.IRC.Nick) == "" || strings.TrimSpace(route.IRC.Channel) == "" {
				return fmt.Errorf("alerts.routes[%d].irc requires address, nick, and channel", i)
			}
		}
		if route.Syslog != nil {
			targets++
			if strings.TrimSpace(route.Syslog.Address) == "" {
				return fmt.Errorf("alerts.routes[%d].syslog.address is required", i)
			}
			if route.Syslog.Network != "" && route.Syslog.Network != "udp" && route.Syslog.Network != "tcp" {
				return fmt.Errorf("alerts.routes[%d].syslog.network must be udp or tcp", i)
			}
		}
		if targets != 1 {
			return fmt.Errorf("alerts.routes[%d] must configure exactly one of webhook, irc, or syslog", i)
		}
		if route.Webhook.Timeout < 0 || (route.IRC != nil && route.IRC.Timeout < 0) || (route.Syslog != nil && route.Syslog.Timeout < 0) {
			return fmt.Errorf("alerts.routes[%d] target timeout must be >= 0", i)
		}
		for header, env := range route.Webhook.HeaderEnv {
			if strings.TrimSpace(header) == "" || strings.TrimSpace(env) == "" {
				return fmt.Errorf("alerts.routes[%d].webhook.header_env requires header names and environment variables", i)
			}
		}
	}
	return nil
}

func validateVaultConfig(vault *VaultConfig) error {
	if vault == nil {
		return nil
	}
	if strings.TrimSpace(vault.Address) == "" {
		return fmt.Errorf("secrets.vault.address is required when secrets.vault is set")
	}
	if (strings.TrimSpace(vault.TokenEnv) == "") == (strings.TrimSpace(vault.TokenFile) == "") {
		return fmt.Errorf("secrets.vault requires exactly one of token_env or token_file")
	}
	if vault.KVVersion != 0 && vault.KVVersion != 1 && vault.KVVersion != 2 {
		return fmt.Errorf("secrets.vault.kv_version must be 1 or 2")
	}
	return nil
}

func validateAuth(auth Auth, vault *VaultConfig) error {
	switch auth.Type {
	case "", AuthTypeKey, AuthTypePassword, AuthTypeKeyboardInteractive, AuthTypeAgent:
	default:
		return fmt.Errorf("unsupported type %q", auth.Type)
	}
	if err := validateSecretSource("private_key", auth.PrivateKey, vault); err != nil {
		return err
	}
	if err := validateSecretSource("passphrase_source", auth.PassphraseSource, vault); err != nil {
		return err
	}
	if err := validateSecretSource("certificate", auth.Certificate, vault); err != nil {
		return err
	}
	if err := validateSecretSource("password_source", auth.PasswordSource, vault); err != nil {
		return err
	}
	if auth.Type == AuthTypeAgent && (auth.KeyFile != "" || secretSourceConfigured(auth.PrivateKey) || auth.CertificateFile != "" || secretSourceConfigured(auth.Certificate) || auth.Password != "" || secretSourceConfigured(auth.PasswordSource)) {
		return fmt.Errorf("agent authentication cannot combine key, certificate, or password credentials")
	}
	if (auth.Type == AuthTypePassword || auth.Type == AuthTypeKeyboardInteractive) && strings.TrimSpace(auth.Password) == "" && !secretSourceConfigured(auth.PasswordSource) {
		return fmt.Errorf("password or password_source is required for %s authentication", auth.Type)
	}
	if auth.Password != "" && secretSourceConfigured(auth.PasswordSource) {
		return fmt.Errorf("password and password_source cannot both be configured")
	}
	if auth.Passphrase != "" && secretSourceConfigured(auth.PassphraseSource) {
		return fmt.Errorf("passphrase and passphrase_source cannot both be configured")
	}
	if (auth.Type == "" || auth.Type == AuthTypeKey) && auth.KeyFile != "" && secretSourceConfigured(auth.PrivateKey) {
		return fmt.Errorf("key_file and private_key cannot both be configured")
	}
	if auth.CertificateFile != "" && secretSourceConfigured(auth.Certificate) {
		return fmt.Errorf("certificate_file and certificate cannot both be configured")
	}
	return nil
}

func validateSecretSource(name string, source SecretSource, vault *VaultConfig) error {
	count := 0
	if strings.TrimSpace(source.Env) != "" {
		count++
	}
	if strings.TrimSpace(source.File) != "" {
		count++
	}
	if source.VaultKV != nil {
		count++
		if vault == nil {
			return fmt.Errorf("%s.vault_kv requires secrets.vault", name)
		}
		if strings.TrimSpace(source.VaultKV.Mount) == "" || strings.TrimSpace(source.VaultKV.Path) == "" || strings.TrimSpace(source.VaultKV.Field) == "" {
			return fmt.Errorf("%s.vault_kv requires mount, path, and field", name)
		}
		if source.VaultKV.Version != 0 && source.VaultKV.Version != 1 && source.VaultKV.Version != 2 {
			return fmt.Errorf("%s.vault_kv.version must be 1 or 2", name)
		}
	}
	if count > 1 {
		return fmt.Errorf("%s must configure only one of env, file, or vault_kv", name)
	}
	return nil
}

func secretSourceConfigured(source SecretSource) bool {
	return strings.TrimSpace(source.Env) != "" || strings.TrimSpace(source.File) != "" || source.VaultKV != nil
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
