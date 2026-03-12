package screens

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/fluid-movement/log-tui/config"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// tempCfg creates an isolated config in a temp dir and returns a loaded *Config.
func tempCfg(t *testing.T) *config.Config {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir()) // avoid reading real ~/.ssh/config in NewCreatorModel
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

// newCreatorWithHosts builds a CreatorModel pre-populated with fake SSH entries
// so tests don't depend on the real ~/.ssh/config.
func newCreatorWithHosts(t *testing.T, cfg *config.Config, entries ...config.SSHEntry) CreatorModel {
	t.Helper()
	m := NewCreatorModel(cfg, nil)
	m.hosts = entries
	return m
}

// pressKey sends a single key to the creator model.
func pressKeyCreator(m CreatorModel, code rune, text string) (CreatorModel, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: code, Text: text})
}

func pressEnterCreator(m CreatorModel) (CreatorModel, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
}

func pressEscCreator(m CreatorModel) (CreatorModel, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
}

func pressSpaceCreator(m CreatorModel) (CreatorModel, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
}

// setNameValue injects a value into the name input by updating the model's
// nameInput directly (we're in the same package).
func setNameValue(m CreatorModel, name string) CreatorModel {
	m.nameInput.SetValue(name)
	return m
}

// setPathValue injects a value into the path input.
func setPathValue(m CreatorModel, path string) CreatorModel {
	m.pathInput.SetValue(path)
	return m
}

// execSafe calls cmd() only when cmd is non-nil and is expected to return
// immediately (no channel blocking). For SwitchMsg / QuitMsg commands only.
func execSafe(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	return cmd()
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// ── Step 1: name ──────────────────────────────────────────────────────────────

func TestCreator_StepName_EmptyName_ShowsError(t *testing.T) {
	cfg := tempCfg(t)
	m := NewCreatorModel(cfg, nil) // no hosts needed for this step
	m, _ = pressEnterCreator(m)
	if m.errMsg == "" {
		t.Error("expected an error message for empty name, got empty string")
	}
	if m.step != stepName {
		t.Errorf("step should remain stepName (%d), got %d", stepName, m.step)
	}
}

func TestCreator_StepName_ValidName_AdvancesToHosts(t *testing.T) {
	cfg := tempCfg(t)
	m := newCreatorWithHosts(t, cfg, config.SSHEntry{Alias: "web1", Hostname: "10.0.0.1", Port: 22})
	m = setNameValue(m, "my-project")
	m, _ = pressEnterCreator(m)
	if m.step != stepHosts {
		t.Errorf("expected stepHosts (%d), got %d", stepHosts, m.step)
	}
	if m.errMsg != "" {
		t.Errorf("unexpected error: %q", m.errMsg)
	}
}

// ── Step 2: hosts ─────────────────────────────────────────────────────────────

func TestCreator_StepHosts_SpaceTogglesSelection(t *testing.T) {
	cfg := tempCfg(t)
	m := newCreatorWithHosts(t, cfg,
		config.SSHEntry{Alias: "web1", Hostname: "10.0.0.1", Port: 22},
		config.SSHEntry{Alias: "db1", Hostname: "10.0.0.2", Port: 22},
	)
	m = setNameValue(m, "proj")
	m, _ = pressEnterCreator(m) // advance to stepHosts
	if m.step != stepHosts {
		t.Fatalf("expected stepHosts, got %d", m.step)
	}

	// Toggle first host on.
	m, _ = pressSpaceCreator(m)
	if !m.selected[0] {
		t.Error("expected host[0] to be selected after Space")
	}

	// Toggle first host off.
	m, _ = pressSpaceCreator(m)
	if m.selected[0] {
		t.Error("expected host[0] to be deselected after second Space")
	}
}

func TestCreator_StepHosts_NoSelection_ShowsError(t *testing.T) {
	cfg := tempCfg(t)
	m := newCreatorWithHosts(t, cfg, config.SSHEntry{Alias: "web1", Hostname: "10.0.0.1", Port: 22})
	m = setNameValue(m, "proj")
	m, _ = pressEnterCreator(m) // → stepHosts (no selection made)
	m, _ = pressEnterCreator(m) // try to advance without selecting
	if m.errMsg == "" {
		t.Error("expected error for no hosts selected")
	}
	if m.step != stepHosts {
		t.Errorf("step should remain stepHosts (%d), got %d", stepHosts, m.step)
	}
}

func TestCreator_StepHosts_WithSelection_AdvancesToPath(t *testing.T) {
	cfg := tempCfg(t)
	m := newCreatorWithHosts(t, cfg, config.SSHEntry{Alias: "web1", Hostname: "10.0.0.1", Port: 22})
	m = setNameValue(m, "proj")
	m, _ = pressEnterCreator(m)  // → stepHosts
	m, _ = pressSpaceCreator(m)  // select host
	m, _ = pressEnterCreator(m)  // → stepPath
	if m.step != stepPath {
		t.Errorf("expected stepPath (%d), got %d", stepPath, m.step)
	}
}

// ── Step 3: path ──────────────────────────────────────────────────────────────

func TestCreator_StepPath_EmptyPath_ShowsError(t *testing.T) {
	cfg := tempCfg(t)
	m := newCreatorWithHosts(t, cfg, config.SSHEntry{Alias: "web1", Hostname: "10.0.0.1", Port: 22})
	m = setNameValue(m, "proj")
	m, _ = pressEnterCreator(m) // → stepHosts
	m, _ = pressSpaceCreator(m)
	m, _ = pressEnterCreator(m) // → stepPath
	m = setPathValue(m, "")     // clear the default "/var/log"
	m, _ = pressEnterCreator(m)
	if m.errMsg == "" {
		t.Error("expected error for empty/non-absolute path")
	}
	if m.step != stepPath {
		t.Errorf("step should remain stepPath (%d), got %d", stepPath, m.step)
	}
}

func TestCreator_StepPath_RelativePath_ShowsError(t *testing.T) {
	cfg := tempCfg(t)
	m := newCreatorWithHosts(t, cfg, config.SSHEntry{Alias: "web1", Hostname: "10.0.0.1", Port: 22})
	m = setNameValue(m, "proj")
	m, _ = pressEnterCreator(m)
	m, _ = pressSpaceCreator(m)
	m, _ = pressEnterCreator(m)
	m = setPathValue(m, "relative/path")
	m, _ = pressEnterCreator(m)
	if m.errMsg == "" {
		t.Error("expected error for relative path")
	}
}

func TestCreator_StepPath_Save_ReturnsSwitchToProjects(t *testing.T) {
	cfg := tempCfg(t)
	m := newCreatorWithHosts(t, cfg, config.SSHEntry{Alias: "web1", Hostname: "10.0.0.1", Port: 22})
	m = setNameValue(m, "proj")
	m, _ = pressEnterCreator(m)
	m, _ = pressSpaceCreator(m)
	m, _ = pressEnterCreator(m)
	m = setPathValue(m, "/var/log/app.log")
	m, cmd := pressEnterCreator(m)
	msg := execSafe(t, cmd)
	sw, ok := msg.(SwitchMsg)
	if !ok {
		t.Fatalf("expected SwitchMsg, got %T: %v", msg, msg)
	}
	if sw.To != ScreenProjects {
		t.Errorf("expected ScreenProjects (%d), got %d", ScreenProjects, sw.To)
	}
	_ = m
}

func TestCreator_StepPath_Save_ProjectInConfig(t *testing.T) {
	cfg := tempCfg(t)
	m := newCreatorWithHosts(t, cfg,
		config.SSHEntry{Alias: "web1", Hostname: "10.0.0.1", User: "ubuntu", Port: 22},
	)
	m = setNameValue(m, "myapp")
	m, _ = pressEnterCreator(m) // → stepHosts
	m, _ = pressSpaceCreator(m) // select web1
	m, _ = pressEnterCreator(m) // → stepPath
	m = setPathValue(m, "/var/log/myapp.log")
	_, cmd := pressEnterCreator(m) // save
	execSafe(t, cmd)

	// Reload config from disk to verify persistence.
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(loaded.Projects))
	}
	p := loaded.Projects[0]
	if p.Name != "myapp" {
		t.Errorf("Name: got %q, want %q", p.Name, "myapp")
	}
	if p.LogPath != "/var/log/myapp.log" {
		t.Errorf("LogPath: got %q, want %q", p.LogPath, "/var/log/myapp.log")
	}
	if len(p.Hosts) != 1 || p.Hosts[0].Name != "web1" {
		t.Errorf("Hosts: got %v, want [{web1}]", p.Hosts)
	}
}

// ── Navigation / back ─────────────────────────────────────────────────────────

func TestCreator_EscFromStepName_ReturnsSwitchToProjects(t *testing.T) {
	cfg := tempCfg(t)
	m := NewCreatorModel(cfg, nil)
	m, cmd := pressEscCreator(m)
	msg := execSafe(t, cmd)
	sw, ok := msg.(SwitchMsg)
	if !ok {
		t.Fatalf("expected SwitchMsg, got %T", msg)
	}
	if sw.To != ScreenProjects {
		t.Errorf("expected ScreenProjects, got %d", sw.To)
	}
	_ = m
}

func TestCreator_EscFromStepHosts_GoesToStepName(t *testing.T) {
	cfg := tempCfg(t)
	m := newCreatorWithHosts(t, cfg, config.SSHEntry{Alias: "web1", Hostname: "10.0.0.1", Port: 22})
	m = setNameValue(m, "proj")
	m, _ = pressEnterCreator(m) // → stepHosts
	m, cmd := pressEscCreator(m)
	if m.step != stepName {
		t.Errorf("expected stepName (%d) after Esc from stepHosts, got %d", stepName, m.step)
	}
	if cmd != nil {
		// Esc from stepHosts should NOT send a SwitchMsg — just go back a step.
		msg := cmd()
		if _, ok := msg.(SwitchMsg); ok {
			t.Error("Esc from stepHosts should not emit SwitchMsg")
		}
	}
}

// ── Edit mode ─────────────────────────────────────────────────────────────────

func TestCreator_EditMode_UpdatesExistingProject(t *testing.T) {
	cfg := tempCfg(t)

	// Create an existing project.
	existing := config.Project{
		ID:      "original-id",
		Name:    "old-name",
		LogPath: "/var/log/old.log",
		Hosts:   []config.Host{{Name: "web1", Hostname: "10.0.0.1", Port: 22}},
	}
	if err := config.AddProject(cfg, existing); err != nil {
		t.Fatalf("AddProject: %v", err)
	}

	// Open in edit mode.
	m := newCreatorWithHosts(t, cfg,
		config.SSHEntry{Alias: "web1", Hostname: "10.0.0.1", Port: 22},
	)
	m.editingID = existing.ID
	m.nameInput.SetValue("new-name")
	m.selected[0] = true

	// Advance through to save.
	m, _ = pressEnterCreator(m) // → stepHosts
	m, _ = pressEnterCreator(m) // → stepPath
	m = setPathValue(m, "/var/log/new.log")
	_, cmd := pressEnterCreator(m) // save (UpdateProject)
	execSafe(t, cmd)

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Projects) != 1 {
		t.Fatalf("expected 1 project after edit, got %d", len(loaded.Projects))
	}
	p := loaded.Projects[0]
	if p.ID != existing.ID {
		t.Errorf("ID changed: got %q, want %q", p.ID, existing.ID)
	}
	if p.Name != "new-name" {
		t.Errorf("Name: got %q, want new-name", p.Name)
	}
	if p.LogPath != "/var/log/new.log" {
		t.Errorf("LogPath: got %q, want /var/log/new.log", p.LogPath)
	}
}
