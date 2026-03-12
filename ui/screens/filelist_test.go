package screens

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/fluid-movement/log-tui/config"
	gossh "github.com/fluid-movement/log-tui/ssh"
	cryptossh "golang.org/x/crypto/ssh"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// newFileListFor builds a FileListModel with named hosts without calling Init().
func newFileListFor(t *testing.T, hostNames ...string) FileListModel {
	t.Helper()
	var hosts []config.Host
	for _, name := range hostNames {
		hosts = append(hosts, config.Host{Name: name, Hostname: "127.0.0.1", Port: 22})
	}
	proj := config.Project{ID: "1", Name: "test-proj", LogPath: "/var/log", Hosts: hosts}
	return NewFileListModel(proj)
}

// fakeUnknownKeyError creates a *gossh.ConnectError with FailHostKeyUnknown and a
// real (but ephemeral) server public key, so the prompt stores a non-nil key.
func fakeUnknownKeyError() *gossh.ConnectError {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	sshPub, _ := cryptossh.NewPublicKey(pub)
	return &gossh.ConnectError{
		Reason:    gossh.FailHostKeyUnknown,
		Err:       errors.New("unknown host key"),
		ServerKey: sshPub,
	}
}

func pressKeyFilelist(m FileListModel, code rune, text string) (FileListModel, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: code, Text: text})
}

func pressSpecialFilelist(m FileListModel, code rune) (FileListModel, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: code})
}

// execCmd calls cmd() and returns the message. Only use for non-blocking cmds.
func execCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected non-nil cmd, got nil")
	}
	return cmd()
}

// ─── Connection message tests ─────────────────────────────────────────────────

func TestFileList_HostConnected_SetsConnectedStatus(t *testing.T) {
	m := newFileListFor(t, "srv1")
	m, _ = m.Update(hostConnectedMsg{host: "srv1", client: nil})
	hs := m.hostStates["srv1"]
	if hs == nil {
		t.Fatal("hostState for srv1 not found")
	}
	if hs.status != gossh.StatusConnected {
		t.Errorf("status: got %v, want StatusConnected", hs.status)
	}
}

func TestFileList_HostConnectError_SetsErrorStatus(t *testing.T) {
	m := newFileListFor(t, "srv1")
	connErr := &gossh.ConnectError{Reason: gossh.FailAuthFailed, Err: errors.New("auth failed")}
	m, _ = m.Update(hostConnectedMsg{host: "srv1", err: connErr})
	hs := m.hostStates["srv1"]
	if hs == nil {
		t.Fatal("hostState not found")
	}
	if hs.status != gossh.StatusError {
		t.Errorf("status: got %v, want StatusError", hs.status)
	}
	if hs.err == nil {
		t.Error("expected non-nil err in hostState")
	}
}

func TestFileList_UnknownHostKey_ShowsPrompt(t *testing.T) {
	m := newFileListFor(t, "srv1")
	connErr := fakeUnknownKeyError()
	m, _ = m.Update(hostConnectedMsg{host: "srv1", err: connErr})
	if !m.keyPrompt.active {
		t.Fatal("expected keyPrompt.active to be true")
	}
	if m.keyPrompt.hostname != "srv1" {
		t.Errorf("keyPrompt.hostname: got %q, want %q", m.keyPrompt.hostname, "srv1")
	}
	if m.keyPrompt.serverKey == nil {
		t.Error("expected serverKey to be populated in keyPrompt")
	}
}

func TestFileList_FilesListed_PopulatesList(t *testing.T) {
	m := newFileListFor(t, "srv1")
	m, _ = m.Update(filesListedMsg{host: "srv1", files: []string{"app.log", "error.log"}})
	if len(m.files["srv1"]) != 2 {
		t.Errorf("expected 2 files for srv1, got %d", len(m.files["srv1"]))
	}
}

func TestFileList_CompoundMsg_SetsStatusAndFiles(t *testing.T) {
	m := newFileListFor(t, "srv1")
	compound := struct {
		connected hostConnectedMsg
		listed    filesListedMsg
	}{
		connected: hostConnectedMsg{host: "srv1", client: nil},
		listed:    filesListedMsg{host: "srv1", files: []string{"app.log"}},
	}
	m, _ = m.Update(compound)
	if m.hostStates["srv1"].status != gossh.StatusConnected {
		t.Error("expected StatusConnected after compound message")
	}
	if len(m.files["srv1"]) != 1 {
		t.Errorf("expected 1 file, got %d", len(m.files["srv1"]))
	}
}

func TestFileList_MultipleHosts_MergedFileList(t *testing.T) {
	m := newFileListFor(t, "host-a", "host-b")
	m, _ = m.Update(filesListedMsg{host: "host-a", files: []string{"common.log", "a-only.log"}})
	m, _ = m.Update(filesListedMsg{host: "host-b", files: []string{"common.log", "b-only.log"}})

	// rebuildList merges files across all hosts into a unique set.
	items := m.list.Items()
	if len(items) != 3 {
		t.Errorf("expected 3 unique files in merged list, got %d", len(items))
	}
}

// ─── Key prompt tests ─────────────────────────────────────────────────────────

func TestFileList_KeyPrompt_Y_ClearsPromptAndReturnsCmd(t *testing.T) {
	m := newFileListFor(t, "srv1")
	// Activate the prompt manually.
	m.keyPrompt = unknownKeyPrompt{
		active:   true,
		hostname: "srv1",
		host:     m.project.Hosts[0],
		// serverKey and remoteAddr left nil to avoid AddKnownHost side-effects in tests.
	}
	m, cmd := pressKeyFilelist(m, 'y', "y")
	if m.keyPrompt.active {
		t.Error("expected keyPrompt.active to be false after 'y'")
	}
	if cmd == nil {
		t.Error("expected non-nil cmd (reconnect attempt) after 'y'")
	}
}

func TestFileList_KeyPrompt_N_ClearsPromptNoReconnect(t *testing.T) {
	m := newFileListFor(t, "srv1")
	m.keyPrompt = unknownKeyPrompt{active: true, hostname: "srv1"}
	m, _ = pressKeyFilelist(m, 'n', "n")
	if m.keyPrompt.active {
		t.Error("expected keyPrompt.active to be false after 'n'")
	}
}

// ─── Navigation tests ─────────────────────────────────────────────────────────

func TestFileList_EscKey_SendsBackToProjects(t *testing.T) {
	m := newFileListFor(t, "srv1")
	m, cmd := pressSpecialFilelist(m, tea.KeyEscape)
	_ = m
	msg := execCmd(t, cmd)
	sw, ok := msg.(SwitchMsg)
	if !ok {
		t.Fatalf("expected SwitchMsg, got %T: %v", msg, msg)
	}
	if sw.To != ScreenProjects {
		t.Errorf("expected ScreenProjects (%d), got %d", ScreenProjects, sw.To)
	}
}

func TestFileList_HelpToggle(t *testing.T) {
	m := newFileListFor(t, "srv1")
	if m.help.ShowAll {
		t.Fatal("help.ShowAll should be false initially")
	}
	m, _ = pressKeyFilelist(m, '?', "?")
	if !m.help.ShowAll {
		t.Error("expected help.ShowAll to be true after '?'")
	}
	m, _ = pressKeyFilelist(m, '?', "?")
	if m.help.ShowAll {
		t.Error("expected help.ShowAll to be false after second '?'")
	}
}

func TestFileList_EnterOnSelectedFile_SendsSwitchToGrid(t *testing.T) {
	m := newFileListFor(t, "srv1")
	// Connect the host so the client map is populated.
	m, _ = m.Update(hostConnectedMsg{host: "srv1", client: nil})
	// Give the host a file to select.
	m, _ = m.Update(filesListedMsg{host: "srv1", files: []string{"app.log"}})

	// Press Enter — should open the grid with the selected file.
	m, cmd := pressSpecialFilelist(m, tea.KeyEnter)
	_ = m
	if cmd == nil {
		t.Fatal("expected non-nil cmd after Enter on a file")
	}
	msg := cmd()
	sw, ok := msg.(SwitchMsg)
	if !ok {
		t.Fatalf("expected SwitchMsg, got %T: %v", msg, msg)
	}
	if sw.To != ScreenGrid {
		t.Errorf("expected ScreenGrid (%d), got %d", ScreenGrid, sw.To)
	}
	payload, ok := sw.Payload.(GridPayload)
	if !ok {
		t.Fatalf("expected GridPayload, got %T", sw.Payload)
	}
	if payload.FilePath != "app.log" {
		t.Errorf("FilePath: got %q, want %q", payload.FilePath, "app.log")
	}
}

// ─── SetSize tests ────────────────────────────────────────────────────────────

func TestFileList_SetSize_StoresDimensions(t *testing.T) {
	m := newFileListFor(t, "srv1")
	m.SetSize(120, 40)
	if m.Width() != 120 {
		t.Errorf("Width: got %d, want 120", m.Width())
	}
	if m.Height() != 40 {
		t.Errorf("Height: got %d, want 40", m.Height())
	}
}

func TestFileList_SetSize_FullHelp_Accepted(t *testing.T) {
	// Verify that SetSize with full help enabled doesn't panic.
	m := newFileListFor(t, "srv1", "srv2")
	m.help.ShowAll = true
	m.SetSize(120, 40) // should not panic
}

func TestFileList_SetSize_MultipleHosts_Accepted(t *testing.T) {
	// Verify SetSize with various host counts doesn't panic.
	for _, n := range []int{1, 2, 5} {
		names := make([]string, n)
		for i := range names {
			names[i] = "host"
		}
		m := newFileListFor(t, names...)
		m.SetSize(120, 40) // must not panic
	}
}
