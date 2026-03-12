package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/fluid-movement/log-tui/config"
	"github.com/fluid-movement/log-tui/ui/screens"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

func newTestApp(t *testing.T) AppModel {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return New(cfg)
}

// update sends a message to the app and returns the updated AppModel.
func update(t *testing.T, m AppModel, msg tea.Msg) AppModel {
	t.Helper()
	updated, _ := m.Update(msg)
	next, ok := updated.(AppModel)
	if !ok {
		t.Fatalf("Update returned %T, want AppModel", updated)
	}
	return next
}

// switchTo sends a SwitchMsg to the app.
func switchTo(t *testing.T, m AppModel, to screens.ScreenID, payload any) AppModel {
	t.Helper()
	return update(t, m, screens.SwitchMsg{To: to, Payload: payload})
}

// ─── Tests ────────────────────────────────────────────────────────────────────

func TestApp_InitialScreen_IsProjects(t *testing.T) {
	m := newTestApp(t)
	if m.current != screens.ScreenProjects {
		t.Errorf("initial screen: got %d, want ScreenProjects (%d)", m.current, screens.ScreenProjects)
	}
}

func TestApp_SwitchToCreator(t *testing.T) {
	m := newTestApp(t)
	m = switchTo(t, m, screens.ScreenCreator, nil)
	if m.current != screens.ScreenCreator {
		t.Errorf("after SwitchToCreator: got %d, want ScreenCreator (%d)", m.current, screens.ScreenCreator)
	}
}

func TestApp_SwitchToFileList(t *testing.T) {
	m := newTestApp(t)
	proj := config.Project{ID: "1", Name: "test", LogPath: "/var/log"}
	m = switchTo(t, m, screens.ScreenFileList, proj)
	if m.current != screens.ScreenFileList {
		t.Errorf("after SwitchToFileList: got %d, want ScreenFileList (%d)", m.current, screens.ScreenFileList)
	}
}

func TestApp_SwitchToGrid(t *testing.T) {
	m := newTestApp(t)
	payload := screens.GridPayload{Project: config.Project{ID: "1", Name: "test"}}
	m = switchTo(t, m, screens.ScreenGrid, payload)
	if m.current != screens.ScreenGrid {
		t.Errorf("after SwitchToGrid: got %d, want ScreenGrid (%d)", m.current, screens.ScreenGrid)
	}
}

func TestApp_WindowSizeMsg_SetsWidthHeight(t *testing.T) {
	m := newTestApp(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.width != 120 {
		t.Errorf("width: got %d, want 120", m.width)
	}
	if m.height != 40 {
		t.Errorf("height: got %d, want 40", m.height)
	}
}

func TestApp_WindowSizeMsg_PropagatedToInactiveScreens(t *testing.T) {
	m := newTestApp(t)
	// Current screen is Projects; all other screens should also receive the size.
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	if m.projects.Width() != 100 {
		t.Errorf("projects.Width: got %d, want 100", m.projects.Width())
	}
	if m.creator.Width() != 100 {
		t.Errorf("creator.Width: got %d, want 100", m.creator.Width())
	}
	if m.filelist.Width() != 100 {
		t.Errorf("filelist.Width: got %d, want 100", m.filelist.Width())
	}
	if m.grid.Width() != 100 {
		t.Errorf("grid.Width: got %d, want 100", m.grid.Width())
	}
}

func TestApp_SwitchRoundTrip_CreatorBackToProjects(t *testing.T) {
	m := newTestApp(t)
	m = switchTo(t, m, screens.ScreenCreator, nil)
	if m.current != screens.ScreenCreator {
		t.Fatalf("expected ScreenCreator, got %d", m.current)
	}
	m = switchTo(t, m, screens.ScreenProjects, nil)
	if m.current != screens.ScreenProjects {
		t.Errorf("after round-trip: got %d, want ScreenProjects (%d)", m.current, screens.ScreenProjects)
	}
}

func TestApp_SwitchToFileList_HasProject(t *testing.T) {
	m := newTestApp(t)
	proj := config.Project{ID: "42", Name: "myapp", LogPath: "/var/log/app"}
	m = switchTo(t, m, screens.ScreenFileList, proj)
	// Verify the filelist model was initialized with the right project.
	if m.filelist.ProjectName() != "myapp" {
		t.Errorf("filelist project: got %q, want %q", m.filelist.ProjectName(), "myapp")
	}
}
