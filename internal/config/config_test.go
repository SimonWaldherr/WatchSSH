package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
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
	if cfg.Servers[0].Port != 22 {
		t.Errorf("Port = %d, want 22", cfg.Servers[0].Port)
	}
	if cfg.Servers[0].Name != "192.168.1.1" {
		t.Errorf("Name = %q, want %q", cfg.Servers[0].Name, "192.168.1.1")
	}
}

func TestLoad_ExplicitValues(t *testing.T) {
	path := writeConfig(t, `
interval: 120
timeout: 15
output:
  type: json
  file: /tmp/out.json
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
