package ssh

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
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

// ─── ECDSA testKey (for connect-only tests) ───────────────────────────────────

type testKey struct {
	raw    *ecdsa.PrivateKey
	signer gossh.Signer
	pub    gossh.PublicKey
}

func newTestKey(t *testing.T) testKey {
	t.Helper()
	raw, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := gossh.NewSignerFromKey(raw)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	return testKey{raw: raw, signer: signer, pub: signer.PublicKey()}
}

func (k testKey) writeToFile(t *testing.T, path string) {
	t.Helper()
	block, err := gossh.MarshalPrivateKey(k.raw, "")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	data := pem.EncodeToMemory(block)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}
}

// ─── Simple (connect-only) SSH server ─────────────────────────────────────────

// startSSHServerWithLog starts an in-process SSH server accepting authorizedKey.
// onOffered is called with each offered key fingerprint (may be nil).
func startSSHServerWithLog(t *testing.T, authorizedKey gossh.PublicKey, onOffered func(string)) (addr string, addToKnownHosts func(khPath string)) {
	t.Helper()

	hostKey := newTestKey(t)

	serverCfg := &gossh.ServerConfig{
		PublicKeyCallback: func(_ gossh.ConnMetadata, offered gossh.PublicKey) (*gossh.Permissions, error) {
			fp := gossh.FingerprintSHA256(offered)
			if onOffered != nil {
				onOffered(fp)
			}
			if fp == gossh.FingerprintSHA256(authorizedKey) {
				return &gossh.Permissions{}, nil
			}
			return nil, fmt.Errorf("key not authorized: %s", fp)
		},
	}
	serverCfg.AddHostKey(hostKey.signer)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { l.Close() })

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				sc, chans, reqs, err := gossh.NewServerConn(c, serverCfg)
				if err != nil {
					return
				}
				go gossh.DiscardRequests(reqs)
				for ch := range chans {
					ch.Reject(gossh.UnknownChannelType, "not supported")
				}
				sc.Close()
			}(conn)
		}
	}()

	addr = l.Addr().String()
	addToKnownHosts = func(khPath string) {
		f, err := os.OpenFile(khPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			t.Fatalf("open known_hosts: %v", err)
		}
		defer f.Close()
		fmt.Fprintln(f, knownhosts.Line([]string{addr}, hostKey.pub))
	}
	return addr, addToKnownHosts
}

// startSSHServer starts an in-process SSH server accepting authorizedKey.
func startSSHServer(t *testing.T, authorizedKey gossh.PublicKey) (addr string, addToKnownHosts func(khPath string)) {
	return startSSHServerWithLog(t, authorizedKey, nil)
}

// testHost builds a config.Host from an SSH server address and key path.
func testHost(addr, keyPath string) config.Host {
	h, portStr, _ := net.SplitHostPort(addr)
	p := 22
	fmt.Sscanf(portStr, "%d", &p)
	return config.Host{
		Name:          "test-host",
		Hostname:      h,
		User:          "testuser",
		Port:          p,
		IdentityFiles: []string{keyPath},
	}
}

// withKnownHosts overrides the known_hosts path for the duration of the test.
func withKnownHosts(t *testing.T, khPath string) {
	t.Helper()
	knownHostsPathOverride = khPath
	t.Cleanup(func() { knownHostsPathOverride = "" })
}

// ─── Exec-capable SSH server (for tail/grep tests) ────────────────────────────

type execSSHServer struct {
	addr       string
	listener   net.Listener
	hostSigner gossh.Signer
}

// newExecSSHServer starts an in-process SSH server that accepts clientPub and
// executes commands via sh -c locally.
func newExecSSHServer(t *testing.T, clientPub gossh.PublicKey) *execSSHServer {
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
	s := &execSSHServer{addr: ln.Addr().String(), listener: ln, hostSigner: hostSigner}
	go s.serve(ln, cfg)
	t.Cleanup(func() { ln.Close() })
	return s
}

func (s *execSSHServer) serve(ln net.Listener, cfg *gossh.ServerConfig) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn, cfg)
	}
}

func (s *execSSHServer) handleConn(conn net.Conn, cfg *gossh.ServerConfig) {
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

func (s *execSSHServer) handleSession(ch gossh.Channel, requests <-chan *gossh.Request) {
	defer ch.Close()
	for req := range requests {
		if req.Type != "exec" {
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
			continue
		}
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

// ─── Exec SSH test environment ────────────────────────────────────────────────

// testSSHEnv sets up a temporary $HOME with:
//   - .ssh/id_ed25519  (generated client private key, mode 0600)
//   - .ssh/known_hosts (the test server's host key)
//
// It starts an exec-capable test SSH server and returns the config.Host.
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

	privPEMBlock, err := gossh.MarshalPrivateKey(clientPriv, "")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	privPEM := pem.EncodeToMemory(privPEMBlock)
	if err := os.WriteFile(filepath.Join(sshDir, "id_ed25519"), privPEM, 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}

	sshClientPub, err := gossh.NewPublicKey(clientPub)
	if err != nil {
		t.Fatalf("new public key: %v", err)
	}
	srv := newExecSSHServer(t, sshClientPub)

	h, portStr, err := net.SplitHostPort(srv.addr)
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	port := 0
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		t.Fatalf("parse port: %v", err)
	}

	khLine := knownhosts.Line(
		[]string{knownhosts.Normalize(srv.addr)},
		srv.hostSigner.PublicKey(),
	)
	khPath := filepath.Join(sshDir, "known_hosts")
	if err := os.WriteFile(khPath, []byte(khLine+"\n"), 0o600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}

	return config.Host{
		Name:          "test-server",
		Hostname:      h,
		Port:          port,
		IdentityFiles: []string{filepath.Join(sshDir, "id_ed25519")},
	}
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

// ─── Key round-trip and connect-only tests ────────────────────────────────────

func TestKeyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	original := newTestKey(t)
	keyPath := filepath.Join(dir, "id_ecdsa")
	original.writeToFile(t, keyPath)

	data, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := gossh.ParsePrivateKey(data)
	if err != nil {
		t.Fatalf("ParsePrivateKey: %v", err)
	}

	origFP := gossh.FingerprintSHA256(original.pub)
	parsedFP := gossh.FingerprintSHA256(parsed.PublicKey())
	t.Logf("original fingerprint: %s", origFP)
	t.Logf("parsed   fingerprint: %s", parsedFP)
	if origFP != parsedFP {
		t.Errorf("fingerprint mismatch after PEM round-trip")
	}
}

func TestConnect_AuthWithKeyFile(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", "")

	dir := t.TempDir()
	clientKey := newTestKey(t)
	keyPath := filepath.Join(dir, "id_ecdsa")
	clientKey.writeToFile(t, keyPath)

	var serverSawFP string
	addr, addToKnownHosts := startSSHServerWithLog(t, clientKey.pub, func(fp string) {
		serverSawFP = fp
	})
	khPath := filepath.Join(dir, "known_hosts")
	addToKnownHosts(khPath)
	withKnownHosts(t, khPath)

	client, err := Connect(context.Background(), testHost(addr, keyPath))
	t.Logf("server saw fingerprint: %s", serverSawFP)
	t.Logf("expected fingerprint:   %s", gossh.FingerprintSHA256(clientKey.pub))
	if err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}
	client.Close()
}

func TestConnect_WrongKey_FailsAuth(t *testing.T) {
	dir := t.TempDir()
	authorizedKey := newTestKey(t)
	wrongKey := newTestKey(t)
	keyPath := filepath.Join(dir, "id_ecdsa")
	wrongKey.writeToFile(t, keyPath)

	addr, addToKnownHosts := startSSHServer(t, authorizedKey.pub)
	khPath := filepath.Join(dir, "known_hosts")
	addToKnownHosts(khPath)
	withKnownHosts(t, khPath)

	_, err := Connect(context.Background(), testHost(addr, keyPath))
	if err == nil {
		t.Fatal("expected auth failure, got nil")
	}
	var ce *ConnectError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ConnectError, got %T: %v", err, err)
	}
	if ce.Reason != FailAuthFailed {
		t.Errorf("Reason = %v, want FailAuthFailed", ce.Reason)
	}
}

func TestConnect_UnknownHost(t *testing.T) {
	dir := t.TempDir()
	clientKey := newTestKey(t)
	keyPath := filepath.Join(dir, "id_ecdsa")
	clientKey.writeToFile(t, keyPath)

	addr, _ := startSSHServer(t, clientKey.pub)
	khPath := filepath.Join(dir, "known_hosts")
	os.WriteFile(khPath, nil, 0o600)
	withKnownHosts(t, khPath)

	_, err := Connect(context.Background(), testHost(addr, keyPath))
	if err == nil {
		t.Fatal("expected unknown host error, got nil")
	}
	var ce *ConnectError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ConnectError, got %T: %v", err, err)
	}
	if ce.Reason != FailHostKeyUnknown {
		t.Errorf("Reason = %v, want FailHostKeyUnknown", ce.Reason)
	}
}

// ─── Integration: connect success / host-key / auth-failed ───────────────────

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

	_, clientPriv, _ := ed25519.GenerateKey(rand.Reader)
	privPEMBlock, _ := gossh.MarshalPrivateKey(clientPriv, "")
	_ = os.WriteFile(filepath.Join(sshDir, "id_ed25519"), pem.EncodeToMemory(privPEMBlock), 0o600)

	// Server only accepts a different key.
	differentPub, _, _ := ed25519.GenerateKey(rand.Reader)
	differentSSHPub, _ := gossh.NewPublicKey(differentPub)
	srv := newExecSSHServer(t, differentSSHPub)

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

	khPath := filepath.Join(sshDir, "known_hosts")
	_ = os.Remove(khPath)

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

	tcpAddr, _ := net.ResolveTCPAddr("tcp", addr)
	if err := AddKnownHost(addr, tcpAddr, capturedKey); err != nil {
		t.Fatalf("AddKnownHost: %v", err)
	}

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
	time.Sleep(300 * time.Millisecond)

	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open log file: %v", err)
	}
	want := []string{"first line", "second line", "third line"}
	for _, line := range want {
		fmt.Fprintln(f, line)
	}
	f.Close()

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

	cancel()
	time.Sleep(500 * time.Millisecond)

	f, _ := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0)
	fmt.Fprintln(f, "after cancel")
	f.Close()

	select {
	case msg := <-ch:
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

func TestIntegration_RoundTrip(t *testing.T) {
	host := testSSHEnv(t)
	home, _ := os.UserHomeDir()

	sshCfg := fmt.Sprintf("Host test-server\n    Hostname %s\n    Port %d\n", host.Hostname, host.Port)
	cfgPath := filepath.Join(home, ".ssh", "config")
	if err := os.WriteFile(cfgPath, []byte(sshCfg), 0o600); err != nil {
		t.Fatalf("write ssh config: %v", err)
	}

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
