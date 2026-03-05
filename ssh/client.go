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

// Connect establishes an SSH connection to host using ssh-agent then identity files.
func Connect(ctx context.Context, host config.Host) (*Client, error) {
	authMethods, err := buildAuthMethods(host)
	if err != nil {
		return nil, &ConnectError{Host: host, Reason: FailAuthFailed, Err: err}
	}

	home, _ := os.UserHomeDir()
	khPath := filepath.Join(home, ".ssh", "known_hosts")

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

// buildAuthMethods returns auth methods: ssh-agent first, then identity files.
func buildAuthMethods(host config.Host) ([]gossh.AuthMethod, error) {
	var methods []gossh.AuthMethod

	// 1. ssh-agent via SSH_AUTH_SOCK
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		conn, err := net.Dial("unix", sock)
		if err == nil {
			agentClient := agent.NewClient(conn)
			methods = append(methods, gossh.PublicKeysCallback(agentClient.Signers))
		}
	}

	// 2. Identity files from ~/.ssh/config or defaults
	home, err := os.UserHomeDir()
	if err != nil {
		return methods, nil
	}
	defaults := []string{
		filepath.Join(home, ".ssh", "id_ed25519"),
		filepath.Join(home, ".ssh", "id_rsa"),
		filepath.Join(home, ".ssh", "id_ecdsa"),
	}
	for _, keyPath := range defaults {
		signer, err := loadKey(keyPath, host)
		if err != nil {
			clog.Debug("skipping key", "path", keyPath, "err", err)
			continue
		}
		methods = append(methods, gossh.PublicKeys(signer))
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no auth methods available for host %s", host.Name)
	}
	return methods, nil
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
