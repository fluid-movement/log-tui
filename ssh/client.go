package ssh

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/fluid-movement/log-tui/clog"
	"github.com/fluid-movement/log-tui/config"
	gosshconfig "github.com/kevinburke/ssh_config"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// ConnectFailReason describes why an SSH connection attempt failed.
type ConnectFailReason int

const (
	FailUnreachable    ConnectFailReason = iota
	FailAuthFailed                       // if passphrase key: "start ssh-agent, run ssh-add"
	FailHostKeyChanged                   // show error + known_hosts file/line; never auto-fix
	FailHostKeyUnknown                   // prompt: "Add host key to known_hosts? [y/N]"
	FailTimeout
)

// ConnectError is the typed error returned by Connect.
type ConnectError struct {
	Host      config.Host
	Reason    ConnectFailReason
	Err       error
	ExtraInfo string // e.g. "Run: ssh-add ~/.ssh/your_key" or known_hosts location
}

func (e *ConnectError) Error() string {
	return fmt.Sprintf("connect to %s: %v", e.Host.Name, e.Err)
}

func (e *ConnectError) Unwrap() error { return e.Err }

// HostStatus indicates the live state of a card's connection.
type HostStatus int

const (
	StatusConnecting HostStatus = iota
	StatusConnected
	StatusReconnecting
	StatusError
	StatusUnresolvable
)

// Client wraps a live SSH connection to one host.
type Client struct {
	Host   config.Host
	conn   *gossh.Client
	cancel context.CancelFunc
}

// Close tears down the SSH connection.
func (c *Client) Close() {
	if c.cancel != nil {
		c.cancel()
	}
	if c.conn != nil {
		c.conn.Close()
	}
}

// SSHClient returns the underlying gossh.Client for creating sessions.
func (c *Client) SSHClient() *gossh.Client {
	return c.conn
}

// knownHostsPathOverride is used in tests to point to a temp known_hosts file.
var knownHostsPathOverride string

// Connect establishes an SSH connection to host using ssh-agent then identity files.
func Connect(ctx context.Context, host config.Host) (*Client, error) {
	authMethods, err := buildAuthMethods(host)
	if err != nil {
		return nil, &ConnectError{Host: host, Reason: FailAuthFailed, Err: err}
	}

	home, _ := os.UserHomeDir()
	khPath := filepath.Join(home, ".ssh", "known_hosts")
	if knownHostsPathOverride != "" {
		khPath = knownHostsPathOverride
	}

	hostKeyCallback, err := knownhosts.New(khPath)
	var unknownHostErr *knownhosts.KeyError
	if err != nil && !os.IsNotExist(err) {
		return nil, &ConnectError{Host: host, Reason: FailUnreachable, Err: fmt.Errorf("known_hosts: %w", err)}
	}
	if os.IsNotExist(err) {
		// No known_hosts file yet — all hosts will be unknown.
		hostKeyCallback = func(hostname string, remote net.Addr, key gossh.PublicKey) error {
			return &knownhosts.KeyError{Want: nil}
		}
	}

	cfg := &gossh.ClientConfig{
		User: host.User,
		Auth: authMethods,
		HostKeyCallback: func(hostname string, remote net.Addr, key gossh.PublicKey) error {
			err := hostKeyCallback(hostname, remote, key)
			if err == nil {
				return nil
			}
			if errors.As(err, &unknownHostErr) {
				if len(unknownHostErr.Want) == 0 {
					// Host truly unknown
					return &ConnectError{
						Host:   host,
						Reason: FailHostKeyUnknown,
						Err:    fmt.Errorf("unknown host key for %s", hostname),
					}
				}
				// Key changed
				return &ConnectError{
					Host:      host,
					Reason:    FailHostKeyChanged,
					Err:       fmt.Errorf("host key changed for %s", hostname),
					ExtraInfo: khPath,
				}
			}
			return err
		},
		Timeout: 10 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", host.Hostname, port(host))
	clog.Debug("dialing", "host", host.Name, "addr", addr, "user", cfg.User)

	connCtx, cancel := context.WithCancel(ctx)
	_ = connCtx

	conn, err := gossh.Dial("tcp", addr, cfg)
	if err != nil {
		cancel()
		var ce *ConnectError
		if errors.As(err, &ce) {
			return nil, ce
		}
		reason := FailUnreachable
		if isAuthError(err) {
			reason = FailAuthFailed
		}
		return nil, &ConnectError{Host: host, Reason: reason, Err: err}
	}

	clog.Debug("connected", "host", host.Name)
	return &Client{Host: host, conn: conn, cancel: cancel}, nil
}

func port(h config.Host) int {
	if h.Port == 0 {
		return 22
	}
	return h.Port
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	return contains(err.Error(), "unable to authenticate") ||
		contains(err.Error(), "no supported methods remain")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// buildAuthMethods returns a single publickey AuthMethod with all available signers.
// Agent signers and file signers are combined into one PublicKeys call so the
// golang SSH client tries them all before giving up — using two separate AuthMethods
// causes it to stop after the first method fails without falling through to the second.
func buildAuthMethods(host config.Host) ([]gossh.AuthMethod, error) {
	var signers []gossh.Signer

	// 1. SSH agent keys.
	sock := os.Getenv("SSH_AUTH_SOCK")
	clog.Debug("ssh-agent", "host", host.Name, "SSH_AUTH_SOCK", sock)
	if sock != "" {
		conn, err := net.Dial("unix", sock)
		if err != nil {
			clog.Debug("ssh-agent dial failed", "host", host.Name, "err", err)
		} else {
			agentClient := agent.NewClient(conn)
			agentSigners, err := agentClient.Signers()
			for _, s := range agentSigners {
				clog.Debug("ssh-agent key", "host", host.Name, "type", s.PublicKey().Type(), "fingerprint", gossh.FingerprintSHA256(s.PublicKey()))
			}
			clog.Debug("ssh-agent signers", "host", host.Name, "count", len(agentSigners), "err", err)
			signers = append(signers, agentSigners...)
		}
	}

	// 2. Identity files: stored → live ssh_config → well-known defaults.
	home, err := os.UserHomeDir()
	if err != nil {
		if len(signers) == 0 {
			return nil, fmt.Errorf("no auth methods available for host %s", host.Name)
		}
		return []gossh.AuthMethod{gossh.PublicKeys(signers...)}, nil
	}
	keyPaths := host.IdentityFiles
	clog.Debug("stored IdentityFiles", "host", host.Name, "paths", keyPaths)
	if len(keyPaths) == 0 {
		rawPaths := gosshconfig.GetAll(host.Name, "IdentityFile")
		clog.Debug("ssh_config IdentityFile", "host", host.Name, "paths", rawPaths)
		for _, p := range rawPaths {
			if p != "" {
				keyPaths = append(keyPaths, expandSSHPath(p, home))
			}
		}
	}
	if len(keyPaths) == 0 {
		keyPaths = []string{
			filepath.Join(home, ".ssh", "id_ed25519"),
			filepath.Join(home, ".ssh", "id_rsa"),
			filepath.Join(home, ".ssh", "id_ecdsa"),
		}
		clog.Debug("falling back to default key paths", "host", host.Name, "paths", keyPaths)
	}
	for _, keyPath := range keyPaths {
		signer, err := loadKey(keyPath, host)
		if err != nil {
			clog.Debug("skipping key", "path", keyPath, "err", err)
			continue
		}
		clog.Debug("loaded key", "path", keyPath, "type", signer.PublicKey().Type(), "fingerprint", gossh.FingerprintSHA256(signer.PublicKey()))
		signers = append(signers, signer)
	}

	if len(signers) == 0 {
		return nil, fmt.Errorf("no auth methods available for host %s", host.Name)
	}
	clog.Debug("auth signers ready", "host", host.Name, "count", len(signers))
	return []gossh.AuthMethod{gossh.PublicKeys(signers...)}, nil
}

func loadKey(path string, host config.Host) (gossh.Signer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	signer, err := gossh.ParsePrivateKey(data)
	if err != nil {
		// passphrase-protected key
		return nil, &ConnectError{
			Host:      host,
			Reason:    FailAuthFailed,
			Err:       err,
			ExtraInfo: fmt.Sprintf("Key requires passphrase. Run: ssh-add %s", path),
		}
	}
	return signer, nil
}

// AddKnownHost appends a host key to ~/.ssh/known_hosts.
func AddKnownHost(hostname string, remote net.Addr, key gossh.PublicKey) error {
	home, _ := os.UserHomeDir()
	khPath := filepath.Join(home, ".ssh", "known_hosts")
	f, err := os.OpenFile(khPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
	_, err = fmt.Fprintln(f, line)
	return err
}

func expandSSHPath(path, home string) string {
	if len(path) >= 2 && path[:2] == "~/" {
		return home + path[1:]
	}
	return path
}
