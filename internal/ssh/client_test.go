package ssh

import (
	"bufio"
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
	gossh "golang.org/x/crypto/ssh"
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

func TestCopyWithContext(t *testing.T) {
	var destination bytes.Buffer
	written, err := copyWithContext(context.Background(), &destination, bytes.NewBufferString("artifact"))
	if err != nil || written != 8 || destination.String() != "artifact" {
		t.Fatalf("copy = %d, %q, %v", written, destination.String(), err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = copyWithContext(ctx, &destination, bytes.NewBufferString("ignored"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled copy error = %v", err)
	}
}

func TestUploadTemporaryPathKeepsDestinationAsPrefix(t *testing.T) {
	destination := "/srv/osm/bavaria.osm.pbf"
	temporary := uploadTemporaryPath(destination)
	if len(temporary) <= len(destination) || temporary[:len(destination)] != destination || temporary[len(temporary)-8:] != ".partial" {
		t.Fatalf("temporary upload path = %q", temporary)
	}
}

func TestShellQuote(t *testing.T) {
	for input, want := range map[string]string{
		"/srv/osm/bavaria.osm.pbf": "'/srv/osm/bavaria.osm.pbf'",
		"/srv/it's-ready.pbf":      "'/srv/it'\"'\"'s-ready.pbf'",
	} {
		if got := shellQuote(input); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestReadSCPResponse(t *testing.T) {
	if err := readSCPResponse(bufio.NewReader(bytes.NewBuffer([]byte{0}))); err != nil {
		t.Fatalf("success response error = %v", err)
	}
	if err := readSCPResponse(bufio.NewReader(bytes.NewBufferString("\x01permission denied\n"))); err == nil || err.Error() != "SCP receiver: permission denied" {
		t.Fatalf("error response = %v", err)
	}
	if err := readSCPResponse(bufio.NewReader(bytes.NewBuffer([]byte{3}))); err == nil {
		t.Fatal("invalid response succeeded")
	}
}

func TestUploadUsesSCPOverExistingConnection(t *testing.T) {
	address, events, stop := startSCPTestServer(t)
	defer stop()
	connection, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	clientConn, channels, requests, err := gossh.NewClientConn(connection, address, &gossh.ClientConfig{
		User:            "watchssh-test",
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &sshClient{conn: gossh.NewClient(clientConn, channels, requests)}
	defer client.Close()

	written, err := client.Upload(context.Background(), bytes.NewBufferString("artifact"), 8, "/srv/osm/bavaria.osm.pbf", true)
	if err != nil || written != 8 {
		t.Fatalf("Upload() = %d, %v", written, err)
	}

	seen := make([]scpTestEvent, 0, 3)
	deadline := time.After(2 * time.Second)
	for len(seen) < 3 {
		select {
		case event := <-events:
			if event.err != nil {
				t.Fatalf("SCP test server: %v", event.err)
			}
			seen = append(seen, event)
		case <-deadline:
			t.Fatalf("received %d SCP commands, want 3", len(seen))
		}
	}
	if seen[0].command != "mkdir -p '/srv/osm'" {
		t.Errorf("mkdir command = %q", seen[0].command)
	}
	if !strings.HasPrefix(seen[1].command, "scp -t '/srv/osm/bavaria.osm.pbf.watchssh-") || !strings.HasSuffix(seen[1].command, ".partial'") {
		t.Errorf("SCP command = %q", seen[1].command)
	}
	if seen[1].contents != "artifact" {
		t.Errorf("SCP contents = %q", seen[1].contents)
	}
	if !strings.HasPrefix(seen[2].command, "mv -f '/srv/osm/bavaria.osm.pbf.watchssh-") || !strings.HasSuffix(seen[2].command, ".partial' '/srv/osm/bavaria.osm.pbf'") {
		t.Errorf("publish command = %q", seen[2].command)
	}
}

type scpTestEvent struct {
	command  string
	contents string
	err      error
}

func startSCPTestServer(t *testing.T) (string, <-chan scpTestEvent, func()) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	serverConfig := &gossh.ServerConfig{NoClientAuth: true}
	serverConfig.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan scpTestEvent, 3)
	done := make(chan struct{})
	go func() {
		defer close(done)
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		serverConn, channels, requests, handshakeErr := gossh.NewServerConn(connection, serverConfig)
		if handshakeErr != nil {
			events <- scpTestEvent{err: handshakeErr}
			return
		}
		defer serverConn.Close()
		go gossh.DiscardRequests(requests)
		for newChannel := range channels {
			go serveSCPTestChannel(newChannel, events)
		}
	}()
	return listener.Addr().String(), events, func() {
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("SCP test server did not stop")
		}
	}
}

func serveSCPTestChannel(newChannel gossh.NewChannel, events chan<- scpTestEvent) {
	channel, requests, err := newChannel.Accept()
	if err != nil {
		events <- scpTestEvent{err: err}
		return
	}
	defer channel.Close()
	for request := range requests {
		if request.Type != "exec" {
			_ = request.Reply(false, nil)
			continue
		}
		command, err := decodeSSHString(request.Payload)
		if err != nil {
			events <- scpTestEvent{err: err}
			return
		}
		if err := request.Reply(true, nil); err != nil {
			events <- scpTestEvent{err: err}
			return
		}
		if strings.HasPrefix(command, "scp -t ") {
			contents, receiveErr := receiveSCPTestFile(channel)
			events <- scpTestEvent{command: command, contents: contents, err: receiveErr}
			return
		}
		events <- scpTestEvent{command: command}
		_, _ = channel.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
		return
	}
}

func receiveSCPTestFile(channel gossh.Channel) (string, error) {
	if _, err := channel.Write([]byte{0}); err != nil {
		return "", err
	}
	reader := bufio.NewReader(channel)
	header, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	parts := strings.Fields(strings.TrimSpace(header))
	if len(parts) != 3 || parts[0] != "C0644" {
		return "", fmt.Errorf("unexpected SCP header %q", header)
	}
	size, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || size < 0 {
		return "", fmt.Errorf("invalid SCP size %q", parts[1])
	}
	if _, err := channel.Write([]byte{0}); err != nil {
		return "", err
	}
	contents := make([]byte, size)
	if _, err := io.ReadFull(reader, contents); err != nil {
		return "", err
	}
	marker, err := reader.ReadByte()
	if err != nil || marker != 0 {
		return "", fmt.Errorf("SCP completion marker = %d, %v", marker, err)
	}
	if _, err := channel.Write([]byte{0}); err != nil {
		return "", err
	}
	_, _ = io.Copy(io.Discard, reader)
	_, _ = channel.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
	return string(contents), nil
}

func decodeSSHString(payload []byte) (string, error) {
	if len(payload) < 4 {
		return "", fmt.Errorf("SSH request payload is too short")
	}
	length := int(binary.BigEndian.Uint32(payload[:4]))
	if len(payload) != length+4 {
		return "", fmt.Errorf("SSH request payload has invalid string length")
	}
	return string(payload[4:]), nil
}
