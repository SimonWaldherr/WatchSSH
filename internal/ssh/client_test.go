package ssh

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
)

func TestResolveAuthSecretFromEnvironmentAndFile(t *testing.T) {
	t.Setenv("WATCHSSH_TEST_SECRET", "from-environment")
	value, err := resolveAuthSecret(context.Background(), "password", "", config.SecretSource{Env: "WATCHSSH_TEST_SECRET"}, nil)
	if err != nil || value != "from-environment" {
		t.Fatalf("environment secret = %q, %v", value, err)
	}

	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	value, err = resolveAuthSecret(context.Background(), "password", "", config.SecretSource{File: path}, nil)
	if err != nil || value != "from-file" {
		t.Fatalf("file secret = %q, %v", value, err)
	}
}

func TestResolveAuthSecretFromVaultKVv2(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/team/data/watchssh/app-01" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("X-Vault-Token") != "vault-token" {
			t.Fatalf("Vault token = %q", r.Header.Get("X-Vault-Token"))
		}
		_, _ = w.Write([]byte(`{"data":{"data":{"ssh_password":"vault-password"}}}`))
	}))
	defer server.Close()
	t.Setenv("WATCHSSH_VAULT_TOKEN", "vault-token")

	cfg := &config.Config{Secrets: config.SecretsConfig{Vault: &config.VaultConfig{
		Address:   server.URL,
		TokenEnv:  "WATCHSSH_VAULT_TOKEN",
		KVVersion: 2,
	}}}
	value, err := resolveAuthSecret(context.Background(), "password", "", config.SecretSource{VaultKV: &config.VaultKVSource{
		Mount: "team",
		Path:  "watchssh/app-01",
		Field: "ssh_password",
	}}, cfg)
	if err != nil || value != "vault-password" {
		t.Fatalf("Vault secret = %q, %v", value, err)
	}
}

func TestVaultEndpoint(t *testing.T) {
	endpoint, err := vaultEndpoint("https://vault.example.test", config.VaultKVSource{Mount: "kv", Path: "production/app"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "https://vault.example.test/v1/kv/data/production/app" {
		t.Fatalf("endpoint = %q", endpoint)
	}
}
