// Package ssh provides an SSH client wrapper for executing remote commands.
package ssh

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
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
	// Close releases the underlying connection.
	Close() error
}

// sshClient wraps a raw *gossh.Client.
type sshClient struct {
	conn *gossh.Client
}

// agentClient wraps an sshClient and additionally closes the agent connection.
type agentClient struct {
	*sshClient
	agentConn net.Conn
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
	authMethods, agentConn, err := buildAuthMethods(srv.Auth)
	if err != nil {
		return nil, fmt.Errorf("building auth methods: %w", err)
	}

	hostKeyCallback, err := buildHostKeyCallback(srv, globalCfg)
	if err != nil {
		if agentConn != nil {
			agentConn.Close()
		}
		return nil, fmt.Errorf("host key setup: %w", err)
	}

	sshCfg := &gossh.ClientConfig{
		User: srv.Username,
		Auth: authMethods,
		// hostKeyCallback is either knownhosts.New (strict, default) or InsecureIgnoreHostKey
		// when explicitly opted in via InsecureIgnoreHostKey:true or StrictHostKeyChecking:false.
		// The insecure path is always guarded by explicit operator consent. //nolint:gosec
		HostKeyCallback: hostKeyCallback,
		Timeout:         timeout,
	}

	addr := net.JoinHostPort(srv.Host, fmt.Sprintf("%d", srv.Port))

	// Use a context-aware dialer so we can cancel early.
	netConn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", addr)
	if err != nil {
		if agentConn != nil {
			agentConn.Close()
		}
		return nil, fmt.Errorf("TCP connect to %s: %w", addr, err)
	}

	sshConn, chans, reqs, err := gossh.NewClientConn(netConn, addr, sshCfg)
	if err != nil {
		netConn.Close()
		if agentConn != nil {
			agentConn.Close()
		}
		return nil, fmt.Errorf("SSH handshake with %s: %w", addr, err)
	}

	base := &sshClient{conn: gossh.NewClient(sshConn, chans, reqs)}
	if agentConn != nil {
		return &agentClient{sshClient: base, agentConn: agentConn}, nil
	}
	return base, nil
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

// Close releases the underlying SSH connection.
func (c *sshClient) Close() error {
	return c.conn.Close()
}

// Close releases both the SSH connection and the agent connection.
func (c *agentClient) Close() error {
	err := c.sshClient.Close()
	_ = c.agentConn.Close()
	return err
}

// buildAuthMethods constructs the SSH auth methods plus an optional agent
// net.Conn that must be closed when the SSH session ends.
func buildAuthMethods(auth config.Auth) ([]gossh.AuthMethod, net.Conn, error) {
	switch auth.Type {
	case config.AuthTypePassword:
		return []gossh.AuthMethod{gossh.Password(auth.Password)}, nil, nil

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
		keyFile := auth.KeyFile
		if keyFile == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, nil, fmt.Errorf("getting home directory: %w", err)
			}
			keyFile = filepath.Join(home, ".ssh", "id_rsa")
		} else {
			keyFile = expandTilde(keyFile)
		}

		keyData, err := os.ReadFile(keyFile)
		if err != nil {
			return nil, nil, fmt.Errorf("reading private key %s: %w", keyFile, err)
		}

		var signer gossh.Signer
		if auth.Passphrase != "" {
			signer, err = gossh.ParsePrivateKeyWithPassphrase(keyData, []byte(auth.Passphrase))
		} else {
			signer, err = gossh.ParsePrivateKey(keyData)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("parsing private key: %w", err)
		}

		return []gossh.AuthMethod{gossh.PublicKeys(signer)}, nil, nil
	}
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
