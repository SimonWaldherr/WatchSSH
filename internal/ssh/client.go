// Package ssh provides an SSH client wrapper for executing remote commands.
package ssh

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/SimonWaldherr/WatchSSH/internal/config"
)

// Client is the interface returned by New; it abstracts over concrete SSH
// client implementations (plain and agent-backed).
type Client interface {
	// Run executes cmd on the remote host and returns combined stdout+stderr.
	// It respects ctx cancellation.
	Run(ctx context.Context, cmd string) (string, error)
	// DialTCP opens a TCP connection from the SSH target with direct-tcpip.
	// It does not invoke a remote shell or require netcat on the target.
	DialTCP(ctx context.Context, host string, port int) (time.Duration, error)
	// Close releases the underlying connection.
	Close() error
}

// sshClient wraps a raw *gossh.Client.
type sshClient struct {
	conn          *gossh.Client
	cleanup       []io.Closer
	stopKeepalive chan struct{}
	closeOnce     sync.Once
}

// New dials the server described by srv and returns a ready Client.
// The caller must call Close() when done.
//
// Host key verification is performed against the known_hosts file by default.
// If the host is unknown or the key has changed, an error is returned with a
// clear explanation. To skip verification for a specific server, set
// insecure_ignore_host_key: true in the server's config — and be aware of the
// security implications (MITM risk).
func New(ctx context.Context, srv config.Server, globalCfg *config.Config, timeout time.Duration) (Client, error) {
	targetCfg, targetAgent, err := buildClientConfig(ctx, srv, globalCfg, timeout)
	if err != nil {
		return nil, err
	}
	addr := net.JoinHostPort(srv.Host, fmt.Sprintf("%d", srv.Port))
	cleanup := make([]io.Closer, 0, 3)
	if targetAgent != nil {
		cleanup = append(cleanup, targetAgent)
	}

	var targetConn net.Conn
	if srv.ProxyJump == nil {
		targetConn, err = dialTCP(ctx, addr, timeout)
	} else {
		jumpSrv := config.Server{Host: srv.ProxyJump.Host, Port: srv.ProxyJump.Port, Username: srv.ProxyJump.Username, Auth: srv.ProxyJump.Auth, InsecureIgnoreHostKey: srv.ProxyJump.InsecureIgnoreHostKey}
		jumpCfg, jumpAgent, configErr := buildClientConfig(ctx, jumpSrv, globalCfg, timeout)
		if configErr != nil {
			closeAll(cleanup)
			return nil, fmt.Errorf("building proxy jump configuration: %w", configErr)
		}
		if jumpAgent != nil {
			cleanup = append(cleanup, jumpAgent)
		}
		jumpAddr := net.JoinHostPort(jumpSrv.Host, fmt.Sprintf("%d", jumpSrv.Port))
		jumpConn, dialErr := dialTCP(ctx, jumpAddr, timeout)
		if dialErr != nil {
			closeAll(cleanup)
			return nil, fmt.Errorf("proxy jump TCP connect to %s: %w", jumpAddr, dialErr)
		}
		jumpClient, handshakeErr := newSSHClient(jumpConn, jumpAddr, jumpCfg)
		if handshakeErr != nil {
			closeAll(append(cleanup, jumpConn))
			return nil, fmt.Errorf("proxy jump SSH handshake with %s: %w", jumpAddr, handshakeErr)
		}
		cleanup = append(cleanup, jumpClient)
		targetConn, err = dialSSHChannel(ctx, jumpClient, addr)
	}
	if err != nil {
		closeAll(cleanup)
		return nil, fmt.Errorf("TCP connect to %s: %w", addr, err)
	}
	targetClient, err := newSSHClient(targetConn, addr, targetCfg)
	if err != nil {
		closeAll(append(cleanup, targetConn))
		return nil, fmt.Errorf("SSH handshake with %s: %w", addr, err)
	}
	client := &sshClient{conn: targetClient, cleanup: cleanup}
	if srv.KeepaliveInterval > 0 {
		client.startKeepalives(time.Duration(srv.KeepaliveInterval)*time.Second, srv.KeepaliveCountMax)
	}
	return client, nil
}

func buildClientConfig(ctx context.Context, srv config.Server, globalCfg *config.Config, timeout time.Duration) (*gossh.ClientConfig, net.Conn, error) {
	authMethods, agentConn, err := buildAuthMethods(ctx, srv.Auth, globalCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("building auth methods: %w", err)
	}
	hostKeyCallback, err := buildHostKeyCallback(srv, globalCfg)
	if err != nil {
		if agentConn != nil {
			_ = agentConn.Close()
		}
		return nil, nil, fmt.Errorf("host key setup: %w", err)
	}
	return &gossh.ClientConfig{User: srv.Username, Auth: authMethods, HostKeyCallback: hostKeyCallback, Timeout: timeout}, agentConn, nil
}

func dialTCP(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error) {
	return (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", addr)
}

func newSSHClient(conn net.Conn, addr string, cfg *gossh.ClientConfig) (*gossh.Client, error) {
	sshConn, chans, reqs, err := gossh.NewClientConn(conn, addr, cfg)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return gossh.NewClient(sshConn, chans, reqs), nil
}

func dialSSHChannel(ctx context.Context, client *gossh.Client, addr string) (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}
	resultCh := make(chan result, 1)
	go func() { conn, err := client.Dial("tcp", addr); resultCh <- result{conn, err} }()
	select {
	case result := <-resultCh:
		return result.conn, result.err
	case <-ctx.Done():
		go func() {
			if result := <-resultCh; result.conn != nil {
				_ = result.conn.Close()
			}
		}()
		return nil, ctx.Err()
	}
}

// Run executes cmd on the remote host and returns combined stdout+stderr.
// It respects ctx cancellation: when the context is done the session is
// closed immediately and ctx.Err() is returned.
func (c *sshClient) Run(ctx context.Context, cmd string) (string, error) {
	sess, err := c.conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("creating SSH session: %w", err)
	}

	type result struct {
		out string
		err error
	}
	ch := make(chan result, 1)

	go func() {
		out, err := sess.CombinedOutput(cmd)
		ch <- result{string(out), err}
	}()

	select {
	case <-ctx.Done():
		_ = sess.Close()
		return "", ctx.Err()
	case r := <-ch:
		_ = sess.Close()
		return r.out, r.err
	}
}

// DialTCP opens a connection from the authenticated SSH target with
// direct-tcpip. It is the agentless alternative to running nc remotely.
func (c *sshClient) DialTCP(ctx context.Context, host string, port int) (time.Duration, error) {
	startedAt := time.Now()
	conn, err := dialSSHChannel(ctx, c.conn, net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	if err != nil {
		return time.Since(startedAt), err
	}
	_ = conn.Close()
	return time.Since(startedAt), nil
}

// Close releases the underlying SSH connection.
func (c *sshClient) Close() error {
	var err error
	c.closeOnce.Do(func() {
		if c.stopKeepalive != nil {
			close(c.stopKeepalive)
		}
		err = c.conn.Close()
		closeAll(c.cleanup)
	})
	return err
}

func (c *sshClient) startKeepalives(interval time.Duration, maxFailures int) {
	if maxFailures <= 0 {
		maxFailures = 3
	}
	c.stopKeepalive = make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		failures := 0
		for {
			select {
			case <-c.stopKeepalive:
				return
			case <-ticker.C:
				_, _, err := c.conn.SendRequest("keepalive@openssh.com", true, nil)
				if err == nil {
					failures = 0
					continue
				}
				failures++
				if failures >= maxFailures {
					_ = c.Close()
					return
				}
			}
		}
	}()
}

func closeAll(closers []io.Closer) {
	for i := len(closers) - 1; i >= 0; i-- {
		_ = closers[i].Close()
	}
}

// buildAuthMethods constructs the SSH auth methods plus an optional agent
// net.Conn that must be closed when the SSH session ends.
func buildAuthMethods(ctx context.Context, auth config.Auth, globalCfg *config.Config) ([]gossh.AuthMethod, net.Conn, error) {
	switch auth.Type {
	case config.AuthTypePassword:
		password, err := resolveAuthSecret(ctx, "password", auth.Password, auth.PasswordSource, globalCfg)
		if err != nil {
			return nil, nil, err
		}
		return []gossh.AuthMethod{gossh.Password(password)}, nil, nil

	case config.AuthTypeKeyboardInteractive:
		password, err := resolveAuthSecret(ctx, "password", auth.Password, auth.PasswordSource, globalCfg)
		if err != nil {
			return nil, nil, err
		}
		return []gossh.AuthMethod{gossh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i := range answers {
				answers[i] = password
			}
			return answers, nil
		})}, nil, nil

	case config.AuthTypeAgent:
		sock := os.Getenv("SSH_AUTH_SOCK")
		if sock == "" {
			return nil, nil, fmt.Errorf("SSH_AUTH_SOCK is not set")
		}
		agentConn, err := net.Dial("unix", sock)
		if err != nil {
			return nil, nil, fmt.Errorf("connecting to SSH agent: %w", err)
		}
		ag := agent.NewClient(agentConn)
		return []gossh.AuthMethod{gossh.PublicKeysCallback(ag.Signers)}, agentConn, nil

	default: // AuthTypeKey or empty
		var keyData []byte
		if auth.KeyFile != "" {
			keyFile := expandTilde(auth.KeyFile)
			var err error
			keyData, err = os.ReadFile(keyFile)
			if err != nil {
				return nil, nil, fmt.Errorf("reading private key %s: %w", keyFile, err)
			}
		} else if secretSourceConfigured(auth.PrivateKey) {
			keyValue, err := resolveAuthSecret(ctx, "private key", "", auth.PrivateKey, globalCfg)
			if err != nil {
				return nil, nil, err
			}
			keyData = []byte(keyValue)
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, nil, fmt.Errorf("getting home directory: %w", err)
			}
			keyFile := filepath.Join(home, ".ssh", "id_rsa")
			var readErr error
			keyData, readErr = os.ReadFile(keyFile)
			if readErr != nil {
				return nil, nil, fmt.Errorf("reading private key %s: %w", keyFile, readErr)
			}
		}

		var signer gossh.Signer
		passphrase, err := resolveAuthSecret(ctx, "private key passphrase", auth.Passphrase, auth.PassphraseSource, globalCfg)
		if err != nil {
			return nil, nil, err
		}
		if passphrase != "" {
			signer, err = gossh.ParsePrivateKeyWithPassphrase(keyData, []byte(passphrase))
		} else {
			signer, err = gossh.ParsePrivateKey(keyData)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("parsing private key: %w", err)
		}

		certificate, err := resolveCertificate(ctx, auth, globalCfg)
		if err != nil {
			return nil, nil, err
		}
		if len(certificate) > 0 {
			publicKey, _, _, _, parseErr := gossh.ParseAuthorizedKey(certificate)
			if parseErr != nil {
				return nil, nil, fmt.Errorf("parsing SSH certificate: %w", parseErr)
			}
			cert, ok := publicKey.(*gossh.Certificate)
			if !ok {
				return nil, nil, fmt.Errorf("certificate is not an SSH user certificate")
			}
			signer, err = gossh.NewCertSigner(cert, signer)
			if err != nil {
				return nil, nil, fmt.Errorf("combining SSH certificate with private key: %w", err)
			}
		}

		return []gossh.AuthMethod{gossh.PublicKeys(signer)}, nil, nil
	}
}

func resolveCertificate(ctx context.Context, auth config.Auth, globalCfg *config.Config) ([]byte, error) {
	if auth.CertificateFile != "" {
		data, err := os.ReadFile(expandTilde(auth.CertificateFile))
		if err != nil {
			return nil, fmt.Errorf("reading SSH certificate %s: %w", auth.CertificateFile, err)
		}
		return data, nil
	}
	if !secretSourceConfigured(auth.Certificate) {
		return nil, nil
	}
	certificate, err := resolveAuthSecret(ctx, "SSH certificate", "", auth.Certificate, globalCfg)
	if err != nil {
		return nil, err
	}
	return []byte(certificate), nil
}

func resolveAuthSecret(ctx context.Context, name, inline string, source config.SecretSource, globalCfg *config.Config) (string, error) {
	if inline != "" && secretSourceConfigured(source) {
		return "", fmt.Errorf("%s cannot combine an inline value with a secret source", name)
	}
	if inline != "" {
		return inline, nil
	}
	if source.Env != "" {
		value, ok := os.LookupEnv(source.Env)
		if !ok {
			return "", fmt.Errorf("%s environment variable %q is not set", name, source.Env)
		}
		return value, nil
	}
	if source.File != "" {
		data, err := os.ReadFile(expandTilde(source.File))
		if err != nil {
			return "", fmt.Errorf("reading %s from %s: %w", name, source.File, err)
		}
		return trimSingleLineEnding(string(data)), nil
	}
	if source.VaultKV != nil {
		if globalCfg == nil || globalCfg.Secrets.Vault == nil {
			return "", fmt.Errorf("%s Vault KV source requires secrets.vault configuration", name)
		}
		return readVaultKV(ctx, *globalCfg.Secrets.Vault, *source.VaultKV)
	}
	return "", nil
}

func readVaultKV(ctx context.Context, vault config.VaultConfig, source config.VaultKVSource) (string, error) {
	token, err := vaultToken(vault)
	if err != nil {
		return "", err
	}
	version := source.Version
	if version == 0 {
		version = vault.KVVersion
	}
	if version == 0 {
		version = 2
	}
	endpoint, err := vaultEndpoint(vault.Address, source, version)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("creating Vault KV request: %w", err)
	}
	req.Header.Set("X-Vault-Token", token)
	if vault.Namespace != "" {
		req.Header.Set("X-Vault-Namespace", vault.Namespace)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting Vault KV secret: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("Vault KV request returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var payload struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", fmt.Errorf("decoding Vault KV response: %w", err)
	}
	var data map[string]any
	if err := json.Unmarshal(payload.Data, &data); err != nil {
		return "", fmt.Errorf("decoding Vault KV data: %w", err)
	}
	if version == 2 {
		nested, ok := data["data"].(map[string]any)
		if !ok {
			return "", fmt.Errorf("Vault KV v2 response does not contain data.data")
		}
		data = nested
	}
	value, ok := data[source.Field]
	if !ok {
		return "", fmt.Errorf("Vault KV field %q was not found", source.Field)
	}
	stringValue, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("Vault KV field %q is not a string", source.Field)
	}
	return stringValue, nil
}

func vaultToken(vault config.VaultConfig) (string, error) {
	if vault.TokenEnv != "" {
		token, ok := os.LookupEnv(vault.TokenEnv)
		if !ok || token == "" {
			return "", fmt.Errorf("Vault token environment variable %q is not set", vault.TokenEnv)
		}
		return token, nil
	}
	data, err := os.ReadFile(expandTilde(vault.TokenFile))
	if err != nil {
		return "", fmt.Errorf("reading Vault token file %s: %w", vault.TokenFile, err)
	}
	token := trimSingleLineEnding(string(data))
	if token == "" {
		return "", fmt.Errorf("Vault token file %s is empty", vault.TokenFile)
	}
	return token, nil
}

func vaultEndpoint(address string, source config.VaultKVSource, version int) (string, error) {
	base, err := url.Parse(strings.TrimRight(address, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("invalid Vault address %q", address)
	}
	segments := []string{"v1", source.Mount}
	if version == 2 {
		segments = append(segments, "data")
	}
	segments = append(segments, strings.Split(strings.Trim(source.Path, "/"), "/")...)
	encoded := make([]string, 0, len(segments))
	for _, segment := range segments {
		encoded = append(encoded, url.PathEscape(segment))
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/" + strings.Join(encoded, "/")
	return base.String(), nil
}

func trimSingleLineEnding(value string) string {
	value = strings.TrimSuffix(value, "\n")
	return strings.TrimSuffix(value, "\r")
}

func secretSourceConfigured(source config.SecretSource) bool {
	return source.Env != "" || source.File != "" || source.VaultKV != nil
}

// buildHostKeyCallback returns a host key callback.
//
// Security model:
//   - By default (strict mode), connections to unknown or changed hosts fail.
//   - The insecure_ignore_host_key flag on a per-server basis disables checking
//     for that server only. Never use this in production.
//   - If known_hosts is missing in strict mode, the connection fails with a
//     clear error instructing the user to verify the host key out of band.
//
// globalCfg may be nil (e.g. in unit tests); in that case strict mode is
// applied and known_hosts is loaded from the default ~/.ssh/known_hosts path.
func buildHostKeyCallback(srv config.Server, globalCfg *config.Config) (gossh.HostKeyCallback, error) {
	// Per-server override takes precedence.
	if srv.InsecureIgnoreHostKey {
		return gossh.InsecureIgnoreHostKey(), nil //nolint:gosec // explicit per-server opt-in
	}

	// When strict host key checking is explicitly disabled globally, allow
	// insecure connections (but warn the caller via the error return so they
	// can surface a log message; we don't do it here to keep the function pure).
	if globalCfg != nil && !globalCfg.IsStrictHostKeyChecking() {
		return gossh.InsecureIgnoreHostKey(), nil //nolint:gosec // explicit global opt-out
	}

	// Strict mode: require a known_hosts file.
	knownHostsPath := ""
	if globalCfg != nil && globalCfg.KnownHostsPath != "" {
		knownHostsPath = expandTilde(globalCfg.KnownHostsPath)
	}
	if knownHostsPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolving home directory: %w", err)
		}
		knownHostsPath = filepath.Join(home, ".ssh", "known_hosts")
	}

	cb, err := knownhosts.New(knownHostsPath)
	if err != nil {
		// Do NOT silently fall back to InsecureIgnoreHostKey.
		// Return a clear, actionable error message.
		return nil, fmt.Errorf(
			"cannot load known_hosts from %q: %w\n"+
				"  → Add the host key with: ssh-keyscan -H %s >> %s\n"+
				"  → Verify the fingerprint out-of-band before adding it.\n"+
				"  → Alternatively, set insecure_ignore_host_key: true for testing only.",
			knownHostsPath, err, srv.Host, knownHostsPath,
		)
	}
	return cb, nil
}

func expandTilde(path string) string {
	if len(path) > 0 && path[0] == '~' {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}
