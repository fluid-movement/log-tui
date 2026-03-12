package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeSSHConfig writes content to $HOME/.ssh/config in a temp home dir.
func writeSSHConfig(t *testing.T, content string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// ─── ParseSSHConfig ───────────────────────────────────────────────────────────

func TestParseSSHConfig_NoFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// No .ssh/config written.
	entries, err := ParseSSHConfig()
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil entries for missing file, got: %v", entries)
	}
}

func TestParseSSHConfig_WildcardOnly(t *testing.T) {
	writeSSHConfig(t, `
Host *
    ServerAliveInterval 60
    StrictHostKeyChecking no
`)
	entries, err := ParseSSHConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("wildcard-only config should produce no entries, got: %v", entries)
	}
}

func TestParseSSHConfig_BasicEntry(t *testing.T) {
	writeSSHConfig(t, `
Host web1
    Hostname 10.0.0.1
    User ubuntu
    Port 2222
`)
	entries, err := ParseSSHConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(entries), entries)
	}
	e := entries[0]
	if e.Alias != "web1" {
		t.Errorf("Alias: got %q, want %q", e.Alias, "web1")
	}
	if e.Hostname != "10.0.0.1" {
		t.Errorf("Hostname: got %q, want %q", e.Hostname, "10.0.0.1")
	}
	if e.User != "ubuntu" {
		t.Errorf("User: got %q, want %q", e.User, "ubuntu")
	}
	if e.Port != 2222 {
		t.Errorf("Port: got %d, want 2222", e.Port)
	}
}

func TestParseSSHConfig_DefaultPort(t *testing.T) {
	writeSSHConfig(t, `
Host db1
    Hostname db.internal
    User admin
`)
	entries, err := ParseSSHConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Port != 22 {
		t.Errorf("default port should be 22, got %d", entries[0].Port)
	}
}

func TestParseSSHConfig_NoHostnameFallsBackToAlias(t *testing.T) {
	// If Hostname is not set, the alias itself is used as the hostname.
	writeSSHConfig(t, `
Host myserver
    User deploy
`)
	entries, err := ParseSSHConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Hostname != "myserver" {
		t.Errorf("Hostname should fall back to alias %q, got %q", "myserver", entries[0].Hostname)
	}
}

func TestParseSSHConfig_MultipleHosts(t *testing.T) {
	writeSSHConfig(t, `
Host web1
    Hostname 10.0.0.1
    User ubuntu

Host web2
    Hostname 10.0.0.2
    User ubuntu
    Port 2222

Host db1
    Hostname 10.0.0.3
`)
	entries, err := ParseSSHConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(entries), entries)
	}
	aliases := []string{entries[0].Alias, entries[1].Alias, entries[2].Alias}
	for i, want := range []string{"web1", "web2", "db1"} {
		if aliases[i] != want {
			t.Errorf("entry %d alias: got %q, want %q", i, aliases[i], want)
		}
	}
}

func TestParseSSHConfig_MixedWithWildcard(t *testing.T) {
	// Wildcards are skipped; named hosts are returned.
	writeSSHConfig(t, `
Host bastion
    Hostname bastion.example.com
    User ec2-user

Host *
    ServerAliveInterval 60
`)
	entries, err := ParseSSHConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (wildcard skipped), got %d: %v", len(entries), entries)
	}
	if entries[0].Alias != "bastion" {
		t.Errorf("expected alias 'bastion', got %q", entries[0].Alias)
	}
}

func TestParseSSHConfig_HostnameIsCorrectFromFile(t *testing.T) {
	// This is the critical regression test for the bug where gosshconfig.Get()
	// (global DefaultUserSettings) was used instead of cfg.Get() (decoded config).
	// The global getter could read from a different file, returning wrong hostnames.
	writeSSHConfig(t, `
Host prod-api
    Hostname 192.168.1.100
    User deployer
    Port 22
`)
	entries, err := ParseSSHConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	// This would have returned "prod-api" (the alias) before the bug fix,
	// because the global DefaultUserSettings couldn't find the host in the
	// real ~/.ssh/config (which was empty).
	if entries[0].Hostname != "192.168.1.100" {
		t.Errorf("Hostname: got %q, want %q — bug: global gosshconfig.Get used instead of cfg.Get",
			entries[0].Hostname, "192.168.1.100")
	}
	if entries[0].User != "deployer" {
		t.Errorf("User: got %q, want %q", entries[0].User, "deployer")
	}
}

// ─── ValidateProjectHosts ─────────────────────────────────────────────────────

func TestValidateProjectHosts_ExactAliasMatch(t *testing.T) {
	writeSSHConfig(t, `
Host web1
    Hostname 10.0.0.1
    User ubuntu
`)
	proj := Project{
		ID:   "p1",
		Name: "Test",
		Hosts: []Host{
			{Name: "web1", Hostname: "10.0.0.1"},
		},
	}
	results := ValidateProjectHosts(proj)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if !r.Resolved {
		t.Errorf("expected Resolved=true for exact alias match")
	}
	if r.ResolvedAs != "web1" {
		t.Errorf("ResolvedAs: got %q, want %q", r.ResolvedAs, "web1")
	}
	if r.Warning != "" {
		t.Errorf("expected no warning for exact match, got: %q", r.Warning)
	}
}

func TestValidateProjectHosts_SoftHostnameMatch(t *testing.T) {
	// Alias in SSH config changed but hostname is the same — soft match.
	writeSSHConfig(t, `
Host web1-renamed
    Hostname 10.0.0.1
    User ubuntu
`)
	proj := Project{
		ID:   "p1",
		Name: "Test",
		Hosts: []Host{
			// project was created with alias "web1" which no longer exists,
			// but the hostname 10.0.0.1 still matches "web1-renamed".
			{Name: "web1", Hostname: "10.0.0.1"},
		},
	}
	results := ValidateProjectHosts(proj)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if !r.Resolved {
		t.Errorf("expected Resolved=true for soft hostname match")
	}
	if r.ResolvedAs != "web1-renamed" {
		t.Errorf("ResolvedAs: got %q, want %q", r.ResolvedAs, "web1-renamed")
	}
	if r.Warning == "" {
		t.Errorf("expected a warning for soft match, got empty string")
	}
}

func TestValidateProjectHosts_NotFound(t *testing.T) {
	writeSSHConfig(t, `
Host other-host
    Hostname 10.0.0.99
`)
	proj := Project{
		ID:   "p1",
		Name: "Test",
		Hosts: []Host{
			{Name: "missing", Hostname: "192.168.1.1"},
		},
	}
	results := ValidateProjectHosts(proj)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Resolved {
		t.Errorf("expected Resolved=false for unknown host")
	}
	if r.Warning == "" {
		t.Errorf("expected a warning for unresolved host")
	}
}

func TestValidateProjectHosts_NoSSHConfig(t *testing.T) {
	// No ~/.ssh/config at all — all hosts should be unresolved.
	home := t.TempDir()
	t.Setenv("HOME", home)

	proj := Project{
		ID:   "p1",
		Name: "Test",
		Hosts: []Host{
			{Name: "web1", Hostname: "10.0.0.1"},
			{Name: "db1", Hostname: "10.0.0.2"},
		},
	}
	results := ValidateProjectHosts(proj)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Resolved {
			t.Errorf("host %q should be unresolved with no SSH config", r.Host.Name)
		}
	}
}

func TestValidateProjectHosts_MultipleHosts(t *testing.T) {
	writeSSHConfig(t, `
Host web1
    Hostname 10.0.0.1

Host db1
    Hostname 10.0.0.2
`)
	proj := Project{
		ID:   "p1",
		Name: "Test",
		Hosts: []Host{
			{Name: "web1", Hostname: "10.0.0.1"},
			{Name: "db1", Hostname: "10.0.0.2"},
			{Name: "missing", Hostname: "10.0.0.99"},
		},
	}
	results := ValidateProjectHosts(proj)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if !results[0].Resolved {
		t.Error("web1 should be resolved")
	}
	if !results[1].Resolved {
		t.Error("db1 should be resolved")
	}
	if results[2].Resolved {
		t.Error("missing should NOT be resolved")
	}
}
