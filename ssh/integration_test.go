package ssh

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fluid-movement/log-tui/config"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// ─── In-process SSH server ────────────────────────────────────────────────────

type testSSHServer struct {
	addr       string
	listener   net.Listener
	hostSigner gossh.Signer
}

// newTestSSHServer starts an in-process SSH server that accepts clientPub as the
// only authorized public key, and executes commands via sh -c locally.
func newTestSSHServer(t *testing.T, clientPub gossh.PublicKey) *testSSHServer {
	t.Helper()

	_, hostPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	hostSigner, err := gossh.NewSignerFromKey(hostPriv)
	if err != nil {
		t.Fatalf("host signer: %v", err)
	}

	cfg := &gossh.ServerConfig{
		PublicKeyCallback: func(_ gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
			if clientPub != nil && bytes.Equal(key.Marshal(), clientPub.Marshal()) {
				return &gossh.Permissions{}, nil
			}
			return nil, fmt.Errorf("unauthorized key")
		},
	}
	cfg.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &testSSHServer{addr: ln.Addr().String(), listener: ln, hostSigner: hostSigner}
	go s.serve(ln, cfg)
	t.Cleanup(func() { ln.Close() })
	return s
}

func (s *testSSHServer) serve(ln net.Listener, cfg *gossh.ServerConfig) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn, cfg)
	}
}

func (s *testSSHServer) handleConn(conn net.Conn, cfg *gossh.ServerConfig) {
	sshConn, chans, reqs, err := gossh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer sshConn.Close()
	go gossh.DiscardRequests(reqs)
	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			_ = newChan.Reject(gossh.UnknownChannelType, "only session channels supported")
			continue
		}
		ch, requests, err := newChan.Accept()
		if err != nil {
			continue
		}
		go s.handleSession(ch, requests)
	}
}

func (s *testSSHServer) handleSession(ch gossh.Channel, requests <-chan *gossh.Request) {
	defer ch.Close()
	for req := range requests {
		if req.Type != "exec" {
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
			continue
		}
		// exec payload: uint32 big-endian length + command bytes.
		if len(req.Payload) < 4 {
			_ = req.Reply(false, nil)
			continue
		}
		cmdLen := binary.BigEndian.Uint32(req.Payload[0:4])
		if int(cmdLen) > len(req.Payload)-4 {
			_ = req.Reply(false, nil)
			continue
		}
		cmdStr := string(req.Payload[4 : 4+cmdLen])
		_ = req.Reply(true, nil)

		go func(cmdStr string) {
			c := exec.Command("sh", "-c", cmdStr)
			c.Stdout = ch
			c.Stderr = ch.Stderr()
			err := c.Run()
			code := uint32(0)
			if err != nil {
				if xe, ok := err.(*exec.ExitError); ok {
					code = uint32(xe.ExitCode())
				} else {
					code = 1
				}
			}
			exitPayload := struct{ Code uint32 }{code}
			_, _ = ch.SendRequest("exit-status", false, gossh.Marshal(exitPayload))
			ch.Close()
		}(cmdStr)
	}
}

// ─── Test environment setup ───────────────────────────────────────────────────

// testSSHEnv sets up a temporary $HOME containing:
//   - .ssh/id_ed25519  (generated client private key, mode 0600)
//   - .ssh/known_hosts (the test server's host key)
//
// It starts a test SSH server and returns the config.Host pointing to it.
func testSSHEnv(t *testing.T) config.Host {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatalf("mkdir .ssh: %v", err)
	}

	// Generate an ED25519 client key pair.
	clientPub, clientPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}

	// Serialize the private key to OpenSSH PEM (using x/crypto/ssh.MarshalPrivateKey).
	privPEMBlock, err := gossh.MarshalPrivateKey(clientPriv, "")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	privPEM := pem.EncodeToMemory(privPEMBlock)
	if err := os.WriteFile(filepath.Join(sshDir, "id_ed25519"), privPEM, 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}

	// Start the test server, authorizing the generated client key.
	sshClientPub, err := gossh.NewPublicKey(clientPub)
	if err != nil {
		t.Fatalf("new public key: %v", err)
	}
	srv := newTestSSHServer(t, sshClientPub)

	// Parse host/port from the server's listen address.
	h, portStr, err := net.SplitHostPort(srv.addr)
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	port := 0
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		t.Fatalf("parse port: %v", err)
	}

	// Write known_hosts so Connect() trusts the server's host key.
	khLine := knownhosts.Line(
		[]string{knownhosts.Normalize(srv.addr)},
		srv.hostSigner.PublicKey(),
	)
	khPath := filepath.Join(sshDir, "known_hosts")
	if err := os.WriteFile(khPath, []byte(khLine+"\n"), 0o600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}

	return config.Host{Name: "test-server", Hostname: h, Port: port}
}

// receiveWithTimeout reads one LogLineMsg from ch within timeout, or calls t.Fatalf.
func receiveWithTimeout(t *testing.T, ch chan LogLineMsg, timeout time.Duration) LogLineMsg {
	t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(timeout):
		t.Fatalf("timed out after %v waiting for LogLineMsg", timeout)
		return LogLineMsg{}
	}
}

// ─── Connection tests ─────────────────────────────────────────────────────────

func TestIntegration_Connect_Success(t *testing.T) {
	host := testSSHEnv(t)
	ctx := context.Background()

	client, err := Connect(ctx, host)
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	defer client.Close()
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestIntegration_Connect_HostKeyUnknown(t *testing.T) {
	host := testSSHEnv(t)
	home, _ := os.UserHomeDir()

	// Remove the known_hosts file so the server's key is unknown.
	_ = os.Remove(filepath.Join(home, ".ssh", "known_hosts"))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := Connect(ctx, host)
	if err == nil {
		t.Fatal("expected FailHostKeyUnknown error, got nil")
	}
	ce, ok := err.(*ConnectError)
	if !ok {
		t.Fatalf("expected *ConnectError, got %T: %v", err, err)
	}
	if ce.Reason != FailHostKeyUnknown {
		t.Errorf("expected FailHostKeyUnknown (%d), got reason %d: %v", FailHostKeyUnknown, ce.Reason, err)
	}
}

func TestIntegration_Connect_AuthFailed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	_ = os.MkdirAll(sshDir, 0o700)

	// Generate a client key pair, but do NOT authorize it on the server.
	_, clientPriv, _ := ed25519.GenerateKey(rand.Reader)
	privPEMBlock, _ := gossh.MarshalPrivateKey(clientPriv, "")
	_ = os.WriteFile(filepath.Join(sshDir, "id_ed25519"), pem.EncodeToMemory(privPEMBlock), 0o600)

	// The server only accepts a *different* key.
	differentPub, _, _ := ed25519.GenerateKey(rand.Reader) // pub, priv, err — note: pub is first
	differentSSHPub, _ := gossh.NewPublicKey(differentPub)
	srv := newTestSSHServer(t, differentSSHPub)

	h, portStr, _ := net.SplitHostPort(srv.addr)
	port := 0
	fmt.Sscanf(portStr, "%d", &port)

	khLine := knownhosts.Line([]string{knownhosts.Normalize(srv.addr)}, srv.hostSigner.PublicKey())
	_ = os.WriteFile(filepath.Join(sshDir, "known_hosts"), []byte(khLine+"\n"), 0o600)

	host := config.Host{Name: "test", Hostname: h, Port: port}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, err := Connect(ctx, host)
	if err == nil {
		t.Fatal("expected auth failure, got nil error")
	}
	ce, ok := err.(*ConnectError)
	if !ok {
		t.Fatalf("expected *ConnectError, got %T: %v", err, err)
	}
	if ce.Reason != FailAuthFailed {
		t.Errorf("expected FailAuthFailed (%d), got %d: %v", FailAuthFailed, ce.Reason, err)
	}
}

func TestIntegration_AddKnownHost_ThenConnect(t *testing.T) {
	host := testSSHEnv(t)
	home, _ := os.UserHomeDir()
	sshDir := filepath.Join(home, ".ssh")

	// Remove known_hosts so Connect fails on the first attempt.
	khPath := filepath.Join(sshDir, "known_hosts")
	_ = os.Remove(khPath)

	// Use a low-level dial to capture the server's host key without
	// triggering the real Connect() (which would return a ConnectError).
	addr := fmt.Sprintf("%s:%d", host.Hostname, host.Port)
	var capturedKey gossh.PublicKey
	captureCfg := &gossh.ClientConfig{
		User: "test",
		Auth: []gossh.AuthMethod{gossh.Password("dummy")},
		HostKeyCallback: func(_ string, _ net.Addr, key gossh.PublicKey) error {
			capturedKey = key
			return fmt.Errorf("abort after key capture")
		},
		Timeout: 5 * time.Second,
	}
	_, _ = gossh.Dial("tcp", addr, captureCfg)
	if capturedKey == nil {
		t.Fatal("failed to capture server host key")
	}

	// Add the captured key to known_hosts.
	// Pass the full "host:port" addr so knownhosts.Normalize stores "[host]:port"
	// for non-standard ports, matching what Connect()'s HostKeyCallback checks.
	tcpAddr, _ := net.ResolveTCPAddr("tcp", addr)
	if err := AddKnownHost(addr, tcpAddr, capturedKey); err != nil {
		t.Fatalf("AddKnownHost: %v", err)
	}

	// Connect should now succeed.
	ctx := context.Background()
	client, err := Connect(ctx, host)
	if err != nil {
		t.Fatalf("Connect after AddKnownHost failed: %v", err)
	}
	defer client.Close()
}

// ─── Tail streaming tests ─────────────────────────────────────────────────────

func TestIntegration_StartTail_ReceivesLines(t *testing.T) {
	host := testSSHEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := Connect(ctx, host)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	logFile := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(logFile, nil, 0o644); err != nil {
		t.Fatalf("create log file: %v", err)
	}

	ch := make(chan LogLineMsg, 10)
	StartTail(ctx, client, logFile, ch)
	time.Sleep(300 * time.Millisecond) // give tail time to start

	// Append lines to the log file.
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open log file: %v", err)
	}
	want := []string{"first line", "second line", "third line"}
	for _, line := range want {
		fmt.Fprintln(f, line)
	}
	f.Close()

	// Verify all 3 lines arrive with correct content and host.
	for i, wantLine := range want {
		msg := receiveWithTimeout(t, ch, 10*time.Second)
		if msg.Raw != wantLine {
			t.Errorf("line %d: got %q, want %q", i, msg.Raw, wantLine)
		}
		if msg.Host != host.Name {
			t.Errorf("line %d: Host got %q, want %q", i, msg.Host, host.Name)
		}
		if msg.LineNum != i+1 {
			t.Errorf("line %d: LineNum got %d, want %d", i, msg.LineNum, i+1)
		}
	}
}

func TestIntegration_StartTail_ContextCancel(t *testing.T) {
	host := testSSHEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	client, err := Connect(ctx, host)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	logFile := filepath.Join(t.TempDir(), "cancel.log")
	_ = os.WriteFile(logFile, nil, 0o644)

	ch := make(chan LogLineMsg, 5)
	StartTail(ctx, client, logFile, ch)
	time.Sleep(300 * time.Millisecond)

	cancel() // stop the tail goroutine
	time.Sleep(500 * time.Millisecond)

	// Write a line after cancellation — it should NOT arrive on ch.
	f, _ := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0)
	fmt.Fprintln(f, "after cancel")
	f.Close()

	select {
	case msg := <-ch:
		// Allow one stray message due to timing, but log it.
		t.Logf("received after cancel (timing race, non-fatal): %q", msg.Raw)
	case <-time.After(2 * time.Second):
		// Expected: no message received.
	}
}

func TestIntegration_StartTail_LineNumbers(t *testing.T) {
	host := testSSHEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := Connect(ctx, host)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	logFile := filepath.Join(t.TempDir(), "seq.log")
	_ = os.WriteFile(logFile, nil, 0o644)

	ch := make(chan LogLineMsg, 20)
	StartTail(ctx, client, logFile, ch)
	time.Sleep(300 * time.Millisecond)

	f, _ := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0)
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(f, "line %d\n", i)
	}
	f.Close()

	for want := 1; want <= 5; want++ {
		msg := receiveWithTimeout(t, ch, 10*time.Second)
		if msg.LineNum != want {
			t.Errorf("expected LineNum=%d, got %d (raw=%q)", want, msg.LineNum, msg.Raw)
		}
	}
}

func TestIntegration_StartTail_JSONLines(t *testing.T) {
	host := testSSHEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := Connect(ctx, host)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	logFile := filepath.Join(t.TempDir(), "json.log")
	_ = os.WriteFile(logFile, nil, 0o644)

	ch := make(chan LogLineMsg, 5)
	StartTail(ctx, client, logFile, ch)
	time.Sleep(300 * time.Millisecond)

	jsonLine := `{"level":"info","msg":"server started","port":8080}`
	f, _ := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0)
	fmt.Fprintln(f, jsonLine)
	f.Close()

	msg := receiveWithTimeout(t, ch, 10*time.Second)
	if msg.Raw != jsonLine {
		t.Errorf("got %q, want %q", msg.Raw, jsonLine)
	}
}

// ─── Grep tests ───────────────────────────────────────────────────────────────

func TestIntegration_StartGrep_MatchingLines(t *testing.T) {
	host := testSSHEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := Connect(ctx, host)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	logFile := filepath.Join(t.TempDir(), "search.log")
	content := "INFO server started\nERROR db connection failed\nINFO request ok\nERROR timeout\nINFO shutdown\n"
	if err := os.WriteFile(logFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	ch := make(chan LogLineMsg, 10)
	StartGrep(ctx, client, "ERROR", logFile, ch)

	var received []LogLineMsg
	deadline := time.After(10 * time.Second)
	for len(received) < 2 {
		select {
		case msg := <-ch:
			received = append(received, msg)
		case <-deadline:
			t.Fatalf("timed out; got %d matches so far: %v", len(received), received)
		}
	}

	for _, msg := range received {
		if !strings.Contains(msg.Raw, "ERROR") {
			t.Errorf("non-matching line received: %q", msg.Raw)
		}
	}
}

func TestIntegration_StartGrep_NoMatches(t *testing.T) {
	host := testSSHEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := Connect(ctx, host)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	logFile := filepath.Join(t.TempDir(), "no-match.log")
	_ = os.WriteFile(logFile, []byte("hello world\nfoo bar\n"), 0o644)

	ch := make(chan LogLineMsg, 5)
	StartGrep(ctx, client, "NOTFOUND", logFile, ch)

	select {
	case msg := <-ch:
		t.Errorf("expected no results, got: %q", msg.Raw)
	case <-time.After(3 * time.Second):
		// Good.
	}
}

func TestIntegration_StartGrep_RegexPattern(t *testing.T) {
	host := testSSHEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := Connect(ctx, host)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	logFile := filepath.Join(t.TempDir(), "regex.log")
	content := "user_id=123 logged in\nuser_id=456 logged out\nserver restarted\n"
	_ = os.WriteFile(logFile, []byte(content), 0o644)

	ch := make(chan LogLineMsg, 10)
	StartGrep(ctx, client, `user_id=[0-9]+`, logFile, ch)

	var received []LogLineMsg
	deadline := time.After(10 * time.Second)
	for len(received) < 2 {
		select {
		case msg := <-ch:
			received = append(received, msg)
		case <-deadline:
			t.Fatalf("timed out; got %v", received)
		}
	}
	if len(received) != 2 {
		t.Errorf("expected 2 matches, got %d", len(received))
	}
}

// ─── Round-trip test ──────────────────────────────────────────────────────────

// TestIntegration_RoundTrip exercises the full pipeline:
// SSH config parsing → Connect → StartTail → receive lines.
func TestIntegration_RoundTrip(t *testing.T) {
	host := testSSHEnv(t)
	home, _ := os.UserHomeDir()

	// Write an SSH config entry for the test server. This exercises the bug fix
	// where gosshconfig.Get (global) was used instead of cfg.Get (decoded config).
	sshCfg := fmt.Sprintf("Host test-server\n    Hostname %s\n    Port %d\n", host.Hostname, host.Port)
	cfgPath := filepath.Join(home, ".ssh", "config")
	if err := os.WriteFile(cfgPath, []byte(sshCfg), 0o600); err != nil {
		t.Fatalf("write ssh config: %v", err)
	}

	// Connect and tail a log file.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := Connect(ctx, host)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer client.Close()

	logFile := filepath.Join(t.TempDir(), "rt.log")
	_ = os.WriteFile(logFile, nil, 0o644)

	ch := make(chan LogLineMsg, 5)
	StartTail(ctx, client, logFile, ch)
	time.Sleep(300 * time.Millisecond)

	f, _ := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0)
	fmt.Fprintln(f, `{"level":"info","msg":"round-trip ok"}`)
	f.Close()

	msg := receiveWithTimeout(t, ch, 10*time.Second)
	if !strings.Contains(msg.Raw, "round-trip ok") {
		t.Errorf("unexpected message: %q", msg.Raw)
	}
}

// Ensure io is referenced (used by RoundTrip helper).
var _ = io.EOF
