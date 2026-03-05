package ssh

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/fluid-movement/log-tui/config"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

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

// startSSHServerWithLog is like startSSHServer but calls onOffered with each offered key fingerprint.
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
// Returns the listener address and a helper to register its host key in a known_hosts file.
func startSSHServer(t *testing.T, authorizedKey gossh.PublicKey) (addr string, addToKnownHosts func(khPath string)) {
	return startSSHServerWithLog(t, authorizedKey, nil)
}


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

func withKnownHosts(t *testing.T, khPath string) {
	t.Helper()
	knownHostsPathOverride = khPath
	t.Cleanup(func() { knownHostsPathOverride = "" })
}

// ── Tests ────────────────────────────────────────────────────────────────────

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
	// Disable SSH agent so we test key-file auth in isolation.
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
	// Empty known_hosts — server host key is unknown.
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
