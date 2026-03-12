package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// redirectConfig sets XDG_CONFIG_HOME to a temp dir so configPath() writes there.
func redirectConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return dir
}

func TestLoad_FileNotExist(t *testing.T) {
	redirectConfig(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Version != currentSchemaVersion {
		t.Errorf("Version: expected %d, got %d", currentSchemaVersion, cfg.Version)
	}
	if len(cfg.Projects) != 0 {
		t.Errorf("expected empty projects, got %v", cfg.Projects)
	}
}

func TestLoad_CorruptJSON(t *testing.T) {
	dir := redirectConfig(t)
	cfgPath := filepath.Join(dir, "logviewer", "projects.json")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load()
	if err == nil {
		t.Error("expected error for corrupt JSON, got nil")
	}
}

func TestSaveLoad_Roundtrip(t *testing.T) {
	redirectConfig(t)
	now := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	cfg := &Config{
		Version: currentSchemaVersion,
		Projects: []Project{
			{
				ID:        "proj-1",
				Name:      "My Server",
				LogPath:   "/var/log/app.log",
				CreatedAt: now,
				Hosts: []Host{
					{Name: "web1", Hostname: "10.0.0.1", User: "admin", Port: 2222},
				},
			},
		},
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loaded.Version != cfg.Version {
		t.Errorf("Version: got %d, want %d", loaded.Version, cfg.Version)
	}
	if len(loaded.Projects) != 1 {
		t.Fatalf("Projects count: got %d, want 1", len(loaded.Projects))
	}
	p := loaded.Projects[0]
	if p.ID != "proj-1" || p.Name != "My Server" || p.LogPath != "/var/log/app.log" {
		t.Errorf("project fields wrong: %+v", p)
	}
	if len(p.Hosts) != 1 {
		t.Fatalf("hosts count: got %d, want 1", len(p.Hosts))
	}
	h := p.Hosts[0]
	if h.Name != "web1" || h.Hostname != "10.0.0.1" || h.User != "admin" || h.Port != 2222 {
		t.Errorf("host fields wrong: %+v", h)
	}
}

func TestSave_CreatesParentDir(t *testing.T) {
	dir := redirectConfig(t)
	// Ensure the logviewer dir does NOT exist yet.
	logviewerDir := filepath.Join(dir, "logviewer")
	if err := os.RemoveAll(logviewerDir); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{Version: currentSchemaVersion}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save should create parent dirs: %v", err)
	}
	cfgPath := filepath.Join(logviewerDir, "projects.json")
	if _, err := os.Stat(cfgPath); err != nil {
		t.Errorf("config file not created: %v", err)
	}
}

func TestAddProject(t *testing.T) {
	redirectConfig(t)
	cfg := &Config{Version: currentSchemaVersion}

	p := Project{ID: "a", Name: "Alpha", LogPath: "/tmp/a.log", CreatedAt: time.Now()}
	if err := AddProject(cfg, p); err != nil {
		t.Fatalf("AddProject error: %v", err)
	}
	if len(cfg.Projects) != 1 {
		t.Errorf("expected 1 project, got %d", len(cfg.Projects))
	}

	// Verify it persisted.
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Projects) != 1 || loaded.Projects[0].ID != "a" {
		t.Errorf("project not persisted: %+v", loaded.Projects)
	}
}

func TestDeleteProject_RemovesCorrectEntry(t *testing.T) {
	redirectConfig(t)
	cfg := &Config{
		Version: currentSchemaVersion,
		Projects: []Project{
			{ID: "keep", Name: "Keep"},
			{ID: "del", Name: "Delete"},
		},
	}
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}

	if err := DeleteProject(cfg, "del"); err != nil {
		t.Fatalf("DeleteProject error: %v", err)
	}
	if len(cfg.Projects) != 1 {
		t.Errorf("expected 1 project remaining, got %d", len(cfg.Projects))
	}
	if cfg.Projects[0].ID != "keep" {
		t.Errorf("wrong project removed: %+v", cfg.Projects)
	}
}

func TestDeleteProject_UnknownID(t *testing.T) {
	redirectConfig(t)
	cfg := &Config{Version: currentSchemaVersion}
	err := DeleteProject(cfg, "nonexistent")
	if err == nil {
		t.Error("expected error for unknown project ID")
	}
}

func TestDeleteProject_FirstOfThree(t *testing.T) {
	redirectConfig(t)
	cfg := &Config{
		Version: currentSchemaVersion,
		Projects: []Project{
			{ID: "1", Name: "One"},
			{ID: "2", Name: "Two"},
			{ID: "3", Name: "Three"},
		},
	}
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err := DeleteProject(cfg, "1"); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Projects) != 2 {
		t.Fatalf("expected 2 remaining, got %d", len(cfg.Projects))
	}
	if cfg.Projects[0].ID != "2" || cfg.Projects[1].ID != "3" {
		t.Errorf("wrong order after delete: %v", cfg.Projects)
	}
}

func TestUpdateProject_ReplacesCorrectEntry(t *testing.T) {
	redirectConfig(t)
	cfg := &Config{
		Version: currentSchemaVersion,
		Projects: []Project{
			{ID: "x", Name: "Old Name"},
			{ID: "y", Name: "Other"},
		},
	}
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}

	updated := Project{ID: "x", Name: "New Name", LogPath: "/updated.log"}
	if err := UpdateProject(cfg, updated); err != nil {
		t.Fatalf("UpdateProject error: %v", err)
	}

	if cfg.Projects[0].Name != "New Name" {
		t.Errorf("expected 'New Name', got %q", cfg.Projects[0].Name)
	}
	// Other project unchanged.
	if cfg.Projects[1].Name != "Other" {
		t.Errorf("other project should be unchanged, got %q", cfg.Projects[1].Name)
	}
}

func TestUpdateProject_UnknownID(t *testing.T) {
	redirectConfig(t)
	cfg := &Config{Version: currentSchemaVersion}
	err := UpdateProject(cfg, Project{ID: "ghost", Name: "Ghost"})
	if err == nil {
		t.Error("expected error for unknown project ID")
	}
}

func TestMigrate_SetsVersion(t *testing.T) {
	redirectConfig(t)
	// Save a config with version 0 (below currentSchemaVersion).
	dir, _ := os.UserConfigDir()
	cfgPath := filepath.Join(dir, "logviewer", "projects.json")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(Config{Version: 0, Projects: nil})
	if err := os.WriteFile(cfgPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load after migration: %v", err)
	}
	if loaded.Version != currentSchemaVersion {
		t.Errorf("after migration Version should be %d, got %d", currentSchemaVersion, loaded.Version)
	}
}

func TestOmitemptyFields_ZeroPort(t *testing.T) {
	redirectConfig(t)
	// Host with zero Port and empty User should not include those fields in JSON.
	cfg := &Config{
		Version: currentSchemaVersion,
		Projects: []Project{
			{
				ID:      "z",
				Name:    "Zero",
				LogPath: "/z.log",
				Hosts:   []Host{{Name: "h1", Hostname: "1.2.3.4"}}, // Port=0, User=""
			},
		},
	}
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}

	dir, _ := os.UserConfigDir()
	raw, err := os.ReadFile(filepath.Join(dir, "logviewer", "projects.json"))
	if err != nil {
		t.Fatal(err)
	}
	// "port" and "user" should not appear in the JSON (omitempty).
	body := string(raw)
	if contains(body, `"port"`) {
		t.Error("zero Port should be omitted from JSON due to omitempty")
	}
	if contains(body, `"user"`) {
		t.Error("empty User should be omitted from JSON due to omitempty")
	}
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
