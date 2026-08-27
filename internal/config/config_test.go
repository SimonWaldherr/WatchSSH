package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
	"golang.org/x/crypto/bcrypt"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("writing temp config: %v", err)
	}
	return path
}

func TestLoad_Defaults(t *testing.T) {
	path := writeConfig(t, `
servers:
  - host: "192.168.1.1"
    username: "root"
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Interval != 60 {
		t.Errorf("Interval = %d, want 60", cfg.Interval)
	}
	if cfg.Timeout != 30 {
		t.Errorf("Timeout = %d, want 30", cfg.Timeout)
	}
	if cfg.Output.Type != "console" {
		t.Errorf("Output.Type = %q, want %q", cfg.Output.Type, "console")
	}
	if cfg.Storage.Type != "none" {
		t.Errorf("Storage.Type = %q, want none", cfg.Storage.Type)
	}
	if cfg.Servers[0].Port != 22 {
		t.Errorf("Port = %d, want 22", cfg.Servers[0].Port)
	}
	if cfg.Servers[0].Name != "192.168.1.1" {
		t.Errorf("Name = %q, want %q", cfg.Servers[0].Name, "192.168.1.1")
	}
}

func TestLoad_WebAuth(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	path := writeConfig(t, "web:\n  auth:\n    username: ops\n    password_hash: "+string(hash)+"\nservers:\n  - host: 192.0.2.10\n    username: monitor\n")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Web.Auth == nil || cfg.Web.Auth.Username != "ops" {
		t.Fatalf("Web.Auth = %#v, want configured credentials", cfg.Web.Auth)
	}
}

func TestLoad_WebAuthRejectsInvalidHash(t *testing.T) {
	path := writeConfig(t, `
web:
  auth:
    username: ops
    password_hash: not-a-bcrypt-hash
servers:
  - host: 192.0.2.10
    username: monitor
`)
	if _, err := config.Load(path); err == nil {
		t.Fatal("expected invalid web auth hash to be rejected")
	}
}

func TestLoad_RejectsUnauthenticatedPublicDashboard(t *testing.T) {
	path := writeConfig(t, `
web:
  enabled: true
  listen: ":8080"
`)
	_, err := config.Load(path)
	if err == nil || !strings.Contains(err.Error(), "publicly reachable") {
		t.Fatalf("Load() error = %v, want unauthenticated public-dashboard rejection", err)
	}
}

func TestLoad_AllowsUnauthenticatedLoopbackDashboard(t *testing.T) {
	path := writeConfig(t, `
web:
  enabled: true
  listen: "127.0.0.1:8080"
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Web.Listen != "127.0.0.1:8080" {
		t.Fatalf("Web.Listen = %q", cfg.Web.Listen)
	}
}

func TestLoad_AllowsExplicitUnauthenticatedPublicDashboard(t *testing.T) {
	path := writeConfig(t, `
web:
  enabled: true
  listen: ":8080"
  allow_unauthenticated_public: true
`)
	if _, err := config.Load(path); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadOrDefaultUsesLoopbackDashboard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new-config.yaml")
	cfg, err := config.LoadOrDefault(path)
	if err != nil {
		t.Fatalf("LoadOrDefault() error = %v", err)
	}
	if !cfg.Web.Enabled || cfg.Web.Listen != "127.0.0.1:8080" {
		t.Fatalf("default web config = %#v, want enabled loopback dashboard", cfg.Web)
	}
}

func TestSave_RejectsUnauthenticatedPublicDashboard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	err := config.Save(&config.Config{Web: config.WebConfig{Enabled: true, Listen: ":8080"}}, path)
	if err == nil || !strings.Contains(err.Error(), "publicly reachable") {
		t.Fatalf("Save() error = %v, want unauthenticated public-dashboard rejection", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("config file was written despite validation failure: %v", statErr)
	}
}

func TestLoad_VaultCredentialSource(t *testing.T) {
	path := writeConfig(t, `
secrets:
  vault:
    address: https://vault.example.test
    token_env: VAULT_TOKEN
    kv_version: 2
servers:
  - host: 192.0.2.10
    username: monitor
    auth:
      type: keyboard-interactive
      password_source:
        vault_kv:
          mount: infrastructure
          path: watchssh/app-01
          field: ssh_password
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Secrets.Vault == nil || cfg.Servers[0].Auth.PasswordSource.VaultKV == nil {
		t.Fatalf("Vault credential source was not loaded: %#v", cfg)
	}
}

func TestLoad_ExplicitValues(t *testing.T) {
	path := writeConfig(t, `
interval: 120
timeout: 15
output:
  type: json
  file: /tmp/out.json
storage:
  type: tinysql
  path: /tmp/watchssh.tinysql
servers:
  - name: "web-01"
    host: "10.0.0.1"
    port: 2222
    username: "admin"
    auth:
      type: key
      key_file: "~/.ssh/id_ed25519"
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Interval != 120 {
		t.Errorf("Interval = %d, want 120", cfg.Interval)
	}
	if cfg.Timeout != 15 {
		t.Errorf("Timeout = %d, want 15", cfg.Timeout)
	}
	if cfg.Output.Type != "json" {
		t.Errorf("Output.Type = %q, want json", cfg.Output.Type)
	}
	if cfg.Storage.Type != "tinysql" {
		t.Errorf("Storage.Type = %q, want tinysql", cfg.Storage.Type)
	}
	if cfg.Storage.Path != "/tmp/watchssh.tinysql" {
		t.Errorf("Storage.Path = %q, want /tmp/watchssh.tinysql", cfg.Storage.Path)
	}
	srv := cfg.Servers[0]
	if srv.Name != "web-01" {
		t.Errorf("Name = %q, want web-01", srv.Name)
	}
	if srv.Port != 2222 {
		t.Errorf("Port = %d, want 2222", srv.Port)
	}
	if srv.Auth.Type != config.AuthTypeKey {
		t.Errorf("Auth.Type = %q, want key", srv.Auth.Type)
	}
}

func TestLoad_MissingHost(t *testing.T) {
	path := writeConfig(t, `
servers:
  - username: "root"
`)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for missing host, got nil")
	}
}

func TestLoad_MissingUsername(t *testing.T) {
	path := writeConfig(t, `
servers:
  - host: "10.0.0.1"
`)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for missing username, got nil")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := config.Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeConfig(t, "key: [unclosed array\n")
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestLoad_TinySQLStorageDefaultPath(t *testing.T) {
	path := writeConfig(t, `
storage:
  type: tinysql
servers:
  - host: "10.0.0.1"
    username: "admin"
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Storage.Path != "watchssh.tinysql" {
		t.Errorf("Storage.Path = %q, want watchssh.tinysql", cfg.Storage.Path)
	}
	if cfg.Storage.RetentionDays != 30 {
		t.Errorf("Storage.RetentionDays = %d, want 30", cfg.Storage.RetentionDays)
	}
}

func TestLoad_InvalidStorageType(t *testing.T) {
	path := writeConfig(t, `
storage:
  type: postgres
servers:
  - host: "10.0.0.1"
    username: "admin"
`)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for invalid storage type")
	}
}

func TestLoad_InvalidStorageRetention(t *testing.T) {
	path := writeConfig(t, `
storage:
  type: tinysql
  retention_days: -1
servers:
  - host: "10.0.0.1"
    username: "admin"
`)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for invalid retention_days")
	}
}

func TestLoad_MultipleServers(t *testing.T) {
	path := writeConfig(t, `
servers:
  - host: "10.0.0.1"
    username: "admin"
  - host: "10.0.0.2"
    username: "root"
    port: 2022
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Servers) != 2 {
		t.Fatalf("got %d servers, want 2", len(cfg.Servers))
	}
	if cfg.Servers[1].Port != 2022 {
		t.Errorf("Servers[1].Port = %d, want 2022", cfg.Servers[1].Port)
	}
}

func TestLoad_AlertActionDefaults(t *testing.T) {
	path := writeConfig(t, `
servers:
  - host: "10.0.0.1"
    username: "admin"
alerts:
  action:
    command: "echo remediation"
    allowed_executables:
      - "echo"
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Alerts.Action == nil {
		t.Fatal("Alerts.Action is nil")
	}
	if cfg.Alerts.Action.Timeout != 10 {
		t.Errorf("Alerts.Action.Timeout = %d, want 10", cfg.Alerts.Action.Timeout)
	}
}

func TestLoad_AlertActionRequiresAllowlistedExecutable(t *testing.T) {
	path := writeConfig(t, `
servers:
  - host: "10.0.0.1"
    username: "admin"
alerts:
  action:
    command: "echo remediation"
    allowed_executables:
      - "logger"
`)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for command executable not in allowlist")
	}
}

func TestLoad_AlertRouteProtocolDefaults(t *testing.T) {
	path := writeConfig(t, `
servers:
  - host: "10.0.0.1"
    username: "admin"
alerts:
  routes:
    - name: irc-ops
      irc:
        address: irc.example.test:6697
        nick: watchssh
        channel: "#ops"
    - name: syslog
      syslog:
        address: syslog.example.test:514
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	irc := cfg.Alerts.Routes[0].IRC
	if irc == nil || irc.Username != "watchssh" || irc.RealName != "WatchSSH" || irc.Timeout != 10 {
		t.Fatalf("IRC defaults = %#v", irc)
	}
	syslog := cfg.Alerts.Routes[1].Syslog
	if syslog == nil || syslog.Network != "udp" || syslog.AppName != "watchssh" || syslog.Timeout != 10 {
		t.Fatalf("Syslog defaults = %#v", syslog)
	}
}

func TestLoad_AlertRouteRejectsMultipleTargets(t *testing.T) {
	path := writeConfig(t, `
servers:
  - host: "10.0.0.1"
    username: "admin"
alerts:
  routes:
    - name: invalid
      webhook:
        url: https://example.test/watchssh
      syslog:
        address: syslog.example.test:514
`)
	if _, err := config.Load(path); err == nil {
		t.Fatal("expected alert route with multiple targets to be rejected")
	}
}

func TestLoad_RemediationDefaults(t *testing.T) {
	path := writeConfig(t, `
servers:
  - name: web-01
    local: true
alerts:
  remediations:
    - name: restart-web
      enabled: true
      rules: [WebUnavailable]
      command: /etc/init.d/nginx restart
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	remediation := cfg.Alerts.Remediations[0]
	if remediation.Timeout != 30 || remediation.Cooldown != 300 || remediation.MaxAttempts != 3 || remediation.Window != 3600 {
		t.Fatalf("remediation defaults = %#v", remediation)
	}
}

func TestLoad_RemediationRejectsUnknownTarget(t *testing.T) {
	path := writeConfig(t, `
servers:
  - name: web-01
    local: true
alerts:
  remediations:
    - name: restart-web
      enabled: true
      targets: [missing]
      command: /etc/init.d/nginx restart
`)
	if _, err := config.Load(path); err == nil {
		t.Fatal("expected remediation target to be validated")
	}
}

func TestLoad_WatchdogDefaults(t *testing.T) {
	path := writeConfig(t, `
servers:
  - name: web-01
    local: true
alerts:
  remediations:
    - name: restart-web
      enabled: true
      mode: watchdog
      command: /etc/init.d/nginx restart
  watchdog:
    enabled: true
    base_url: http://127.0.0.1:1234/v1
    model: local-model
    allowed_remediations: [restart-web]
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	watchdog := cfg.Alerts.Watchdog
	if watchdog == nil || watchdog.Timeout != 20 || watchdog.Cooldown != 300 || watchdog.MaxInputBytes != 65536 || watchdog.MaxTokens != 300 || watchdog.ResponseFormat != "json_schema" {
		t.Fatalf("watchdog defaults = %#v", watchdog)
	}
}

func TestLoad_RejectsDependencyCycle(t *testing.T) {
	path := writeConfig(t, `
servers:
  - name: api
    local: true
    depends_on: [database]
  - name: database
    local: true
    depends_on: [api]
`)
	if _, err := config.Load(path); err == nil {
		t.Fatal("expected dependency cycle to be rejected")
	}
}

func TestLoad_WatchdogRejectsAlertModeRemediation(t *testing.T) {
	path := writeConfig(t, `
servers:
  - name: web-01
    local: true
alerts:
  remediations:
    - name: restart-web
      enabled: true
      command: /etc/init.d/nginx restart
  watchdog:
    enabled: true
    base_url: http://127.0.0.1:1234/v1
    model: local-model
    allowed_remediations: [restart-web]
`)
	if _, err := config.Load(path); err == nil {
		t.Fatal("expected watchdog to require remediation mode: watchdog")
	}
}

func TestLoad_ProxyJumpKeepaliveAndTargetPortProbe(t *testing.T) {
	path := writeConfig(t, `
servers:
  - name: app-01
    host: 10.20.0.10
    username: monitor
    keepalive_interval: 20
    proxy_jump:
      host: bastion.example.test
      username: jump-monitor
      auth:
        type: agent
    checks:
      ports:
        - host: db.internal
          port: 5432
          source: target
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	srv := cfg.Servers[0]
	if srv.ProxyJump == nil || srv.ProxyJump.Port != 22 || srv.ProxyJump.Username != "jump-monitor" {
		t.Fatalf("proxy jump defaults = %#v", srv.ProxyJump)
	}
	if srv.KeepaliveCountMax != 3 {
		t.Fatalf("keepalive count max = %d, want 3", srv.KeepaliveCountMax)
	}
	port := srv.Checks.Ports[0]
	if port.Source != "target" || port.Host != "db.internal" || port.Timeout != 5 {
		t.Fatalf("target port probe = %#v", port)
	}
}

func TestLoad_RejectsInvalidTargetPortSource(t *testing.T) {
	path := writeConfig(t, `
servers:
  - name: app-01
    host: 10.20.0.10
    username: monitor
    checks:
      ports:
        - port: 443
          source: somewhere-else
`)
	if _, err := config.Load(path); err == nil {
		t.Fatal("expected invalid port probe source to be rejected")
	}
}

func TestLoad_StandardToolProbeDefaults(t *testing.T) {
	path := writeConfig(t, `
servers:
  - name: app-01
    host: 10.20.0.10
    username: monitor
    checks:
      service:
        - unit: nginx.service
      process:
        - pattern: "nginx: worker"
      listening:
        - port: 443
      journal:
        - unit: sshd.service
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	srv := cfg.Servers[0]
	if srv.Checks.Service[0].Timeout != 5 {
		t.Fatalf("service check timeout = %d, want 5", srv.Checks.Service[0].Timeout)
	}
	if srv.Checks.Process[0].MinCount != 1 || srv.Checks.Process[0].Timeout != 5 {
		t.Fatalf("process check defaults = %#v", srv.Checks.Process[0])
	}
	if srv.Checks.Listening[0].Protocol != "tcp" || srv.Checks.Listening[0].Timeout != 5 {
		t.Fatalf("listening check defaults = %#v", srv.Checks.Listening[0])
	}
	if srv.Checks.Journal[0].Priority != "err" || srv.Checks.Journal[0].SinceMinutes != 10 || srv.Checks.Journal[0].Timeout != 5 {
		t.Fatalf("journal check defaults = %#v", srv.Checks.Journal[0])
	}
}

func TestLoad_PortableUnixProbeDefaults(t *testing.T) {
	path := writeConfig(t, `
servers:
  - name: app-01
    host: 10.20.0.10
    username: monitor
    checks:
      file:
        - path: /run/app.pid
      directory:
        - path: /var/cache/app
      log:
        - path: /var/log/app.log
          pattern: ERROR
      command:
        - command: docker
      hash:
        - path: /etc/application/config.yaml
          expected_digest: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
      certificate_file:
        - path: /etc/letsencrypt/live/application/fullchain.pem
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	checks := cfg.Servers[0].Checks
	if checks.File[0].Name != "/run/app.pid" || checks.File[0].Timeout != 5 {
		t.Fatalf("file defaults = %#v", checks.File[0])
	}
	if checks.Directory[0].Name != "/var/cache/app" || checks.Directory[0].Timeout != 10 {
		t.Fatalf("directory defaults = %#v", checks.Directory[0])
	}
	if checks.Log[0].Name != "/var/log/app.log" || checks.Log[0].Lines != 200 || checks.Log[0].Timeout != 5 {
		t.Fatalf("log defaults = %#v", checks.Log[0])
	}
	if checks.Command[0].Name != "docker" || checks.Command[0].Timeout != 5 {
		t.Fatalf("command defaults = %#v", checks.Command[0])
	}
	if checks.Hash[0].Name != "/etc/application/config.yaml" || checks.Hash[0].Algorithm != "sha256" || checks.Hash[0].Timeout != 10 {
		t.Fatalf("hash defaults = %#v", checks.Hash[0])
	}
	if checks.CertFile[0].Name != "/etc/letsencrypt/live/application/fullchain.pem" || checks.CertFile[0].WarnDays != 30 || checks.CertFile[0].Timeout != 5 {
		t.Fatalf("certificate file defaults = %#v", checks.CertFile[0])
	}
}

func TestLoad_PIDOfProcessProbeAndRecoveryVerification(t *testing.T) {
	path := writeConfig(t, `
servers:
  - name: web-01
    host: 10.20.0.10
    username: monitor
    checks:
      process:
        - pidof: apache2
alerts:
  remediations:
    - name: restart-apache
      enabled: true
      command: /etc/init.d/apache2 restart
      verify_command: pidof apache2 >/dev/null
      verify_delay: 2
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	process := cfg.Servers[0].Checks.Process[0]
	if process.Name != "apache2" || process.PIDOf != "apache2" || process.Timeout != 5 {
		t.Fatalf("pidof process = %#v", process)
	}
	recovery := cfg.Alerts.Remediations[0]
	if recovery.VerifyTimeout != recovery.Timeout || recovery.VerifyDelay != 2 {
		t.Fatalf("recovery verification defaults = %#v", recovery)
	}
}

func TestLoad_RejectsInvalidRecoveryVerification(t *testing.T) {
	cases := []string{
		`
servers:
  - host: 10.20.0.10
    username: monitor
alerts:
  remediations:
    - name: restart-app
      command: service app restart
      verify_timeout: 5
`,
		`
servers:
  - host: 10.20.0.10
    username: monitor
alerts:
  remediations:
    - name: restart-app
      command: service app restart
      verify_command: pidof app
      verify_delay: -1
`,
	}
	for i, yamlDoc := range cases {
		if _, err := config.Load(writeConfig(t, yamlDoc)); err == nil {
			t.Fatalf("case %d: expected invalid recovery verification configuration", i)
		}
	}
}

func TestLoad_ScheduledArtifactJob(t *testing.T) {
	path := writeConfig(t, `
servers:
  - name: hetzner-osm
    host: 192.0.2.10
    username: uploader
jobs:
  - name: update-bavaria-osm
    enabled: true
    schedule: "15 3 * * 1"
    working_directory: /Users/ops/osm
    command: ./refresh-osm.sh
    uploads:
      - server: hetzner-osm
        source: /Users/ops/osm/out/bavaria.osm.pbf
        destination: /srv/osm/bavaria.osm.pbf
        create_directories: true
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	job := cfg.Jobs[0]
	if job.Timeout != 3600 || !job.Enabled || job.Uploads[0].Server != "hetzner-osm" || !job.Uploads[0].CreateDirectories {
		t.Fatalf("scheduled job = %#v", job)
	}
}

func TestLoad_RejectsInvalidScheduledArtifactJob(t *testing.T) {
	cases := []string{
		`
servers:
  - name: local
    local: true
jobs:
  - name: upload
    schedule: bad schedule
    command: true
`,
		`
servers:
  - name: local
    local: true
jobs:
  - name: upload
    schedule: "0 3 * * *"
    uploads:
      - server: local
        source: result.osm.pbf
        destination: /srv/result.osm.pbf
`,
	}
	for i, yamlDoc := range cases {
		if _, err := config.Load(writeConfig(t, yamlDoc)); err == nil {
			t.Fatalf("case %d: expected invalid scheduled job", i)
		}
	}
}

func TestLoad_RejectsRelativeUnixProbePaths(t *testing.T) {
	path := writeConfig(t, `
servers:
  - name: app-01
    host: 10.20.0.10
    username: monitor
    checks:
      file:
        - path: tmp/app.pid
`)
	if _, err := config.Load(path); err == nil {
		t.Fatal("expected relative Unix probe path to be rejected")
	}
}

func TestLoad_RejectsInvalidPortableUnixProbeLimits(t *testing.T) {
	cases := []string{
		`
servers:
  - name: app-01
    host: 10.20.0.10
    username: monitor
    checks:
      file:
        - path: /run/app.pid
          timeout: -1
`,
		`
servers:
  - name: app-01
    host: 10.20.0.10
    username: monitor
    checks:
      directory:
        - path: /var/cache/app
          timeout: -1
`,
		`
servers:
  - name: app-01
    host: 10.20.0.10
    username: monitor
    checks:
      log:
        - path: /var/log/app.log
          pattern: ERROR
          timeout: -1
`,
		`
servers:
  - name: app-01
    host: 10.20.0.10
    username: monitor
    checks:
      command:
        - command: ""
`,
		`
servers:
  - name: app-01
    host: 10.20.0.10
    username: monitor
    checks:
      hash:
        - path: /etc/application.conf
          expected_digest: not-a-digest
`,
		`
servers:
  - name: app-01
    host: 10.20.0.10
    username: monitor
    checks:
      certificate_file:
        - path: relative.pem
`,
	}
	for i, yamlDoc := range cases {
		path := writeConfig(t, yamlDoc)
		if _, err := config.Load(path); err == nil {
			t.Fatalf("case %d: expected invalid portable Unix probe limits to be rejected", i)
		}
	}
}

func TestLoad_RejectsStandardToolProbesMissingRequiredFields(t *testing.T) {
	cases := []string{
		`
servers:
  - name: app-01
    host: 10.20.0.10
    username: monitor
    checks:
      service:
        - timeout: 5
`,
		`
servers:
  - name: app-01
    host: 10.20.0.10
    username: monitor
    checks:
      process:
        - min_count: 1
`,
		`
servers:
  - name: app-01
    host: 10.20.0.10
    username: monitor
    checks:
      listening:
        - port: 0
`,
	}
	for i, yamlDoc := range cases {
		path := writeConfig(t, yamlDoc)
		if _, err := config.Load(path); err == nil {
			t.Fatalf("case %d: expected invalid probe config to be rejected", i)
		}
	}
}
