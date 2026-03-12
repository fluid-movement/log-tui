package screens

import (
	"context"
	"testing"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/fluid-movement/log-tui/config"
	"github.com/fluid-movement/log-tui/parser"
	gossh "github.com/fluid-movement/log-tui/ssh"
	"github.com/fluid-movement/log-tui/ui/components"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// newGridFor builds a GridModel with named server cards but no real SSH clients.
// It does NOT call Init() (which would start tail goroutines).
func newGridFor(t *testing.T, hostNames ...string) GridModel {
	t.Helper()
	gi := textinput.New()
	si := textinput.New()
	m := GridModel{
		keys:          defaultGridKeys,
		help:          help.New(),
		detail:        components.NewDetailOverlay(),
		globalInput:   gi,
		searchInput:   si,
		searchResults: make(map[string][]string),
	}
	for _, name := range hostNames {
		host := config.Host{Name: name, Hostname: "127.0.0.1"}
		m.cards = append(m.cards, components.NewServerCard(host))
		m.logChans = append(m.logChans, make(chan gossh.LogLineMsg, 256))
		ctx, cancel := context.WithCancel(context.Background())
		m.ctxs = append(m.ctxs, ctx)
		m.cancels = append(m.cancels, cancel)
		// m.clients intentionally empty — avoids need for real SSH.
	}
	m.updateKeyStates()
	t.Cleanup(func() {
		for _, c := range m.cancels {
			c()
		}
	})
	return m
}

// pressKeyGrid sends a printable key press to the grid.
func pressKeyGrid(m GridModel, ch rune) (GridModel, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
}

// pressSpecialGrid sends a special key press (Enter, Esc, Tab, …) to the grid.
func pressSpecialGrid(m GridModel, code rune, mod ...tea.KeyMod) (GridModel, tea.Cmd) {
	msg := tea.KeyPressMsg{Code: code}
	if len(mod) > 0 {
		msg.Mod = mod[0]
	}
	return m.Update(msg)
}

// sendLine injects a log line for the given host into the grid model.
func sendLine(m GridModel, host, raw string) (GridModel, tea.Cmd) {
	return m.Update(gossh.LogLineMsg{
		Host:     host,
		Raw:      raw,
		Received: time.Now(),
		LineNum:  1,
	})
}

// execGridCmd calls a cmd() only for expected non-blocking commands.
func execGridCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	return cmd()
}

// ─── Layout dimension tests (pure, no I/O) ────────────────────────────────────

func TestGridDimensions_OneCard(t *testing.T) {
	cols, rows, cardW, cardH := gridDimensions(1, 120, 40, 1)
	if cols != 1 || rows != 1 {
		t.Errorf("cols=%d rows=%d, want 1,1", cols, rows)
	}
	if cardW != 120 {
		t.Errorf("cardW=%d, want 120", cardW)
	}
	// effective = 40 - 1 - 1 = 38; cardH = 38/1 = 38
	if cardH != 38 {
		t.Errorf("cardH=%d, want 38", cardH)
	}
}

func TestGridDimensions_TwoCards(t *testing.T) {
	cols, rows, cardW, cardH := gridDimensions(2, 120, 40, 1)
	if cols != 2 || rows != 1 {
		t.Errorf("cols=%d rows=%d, want 2,1", cols, rows)
	}
	if cardW != 60 {
		t.Errorf("cardW=%d, want 60", cardW)
	}
	if cardH != 38 {
		t.Errorf("cardH=%d, want 38", cardH)
	}
}

func TestGridDimensions_FourCards(t *testing.T) {
	cols, rows, cardW, cardH := gridDimensions(4, 120, 40, 1)
	if cols != 2 || rows != 2 {
		t.Errorf("cols=%d rows=%d, want 2,2", cols, rows)
	}
	if cardW != 60 {
		t.Errorf("cardW=%d, want 60", cardW)
	}
	// effective = 38; cardH = 38/2 = 19
	if cardH != 19 {
		t.Errorf("cardH=%d, want 19", cardH)
	}
}

func TestGridDimensions_NineCards(t *testing.T) {
	cols, rows, cardW, cardH := gridDimensions(9, 120, 40, 1)
	if cols != 3 || rows != 3 {
		t.Errorf("cols=%d rows=%d, want 3,3", cols, rows)
	}
	if cardW != 40 {
		t.Errorf("cardW=%d, want 40", cardW)
	}
	// effective = 38; cardH = 38/3 = 12
	if cardH != 12 {
		t.Errorf("cardH=%d, want 12", cardH)
	}
}

func TestGridDimensions_ZeroCards(t *testing.T) {
	cols, rows, cardW, cardH := gridDimensions(0, 120, 40, 1)
	if cols != 1 || rows != 1 {
		t.Errorf("cols=%d rows=%d, want 1,1", cols, rows)
	}
	if cardW != 120 {
		t.Errorf("cardW=%d, want 120", cardW)
	}
	if cardH != 38 {
		t.Errorf("cardH=%d, want 38", cardH)
	}
}

func TestGridDimensions_FullHelpBar(t *testing.T) {
	_, _, _, cardH := gridDimensions(1, 120, 40, 5)
	// effective = 40 - 5 - 1 = 34
	if cardH != 34 {
		t.Errorf("cardH=%d, want 34", cardH)
	}
}

func TestGridDimensions_SmallTerminal_AtLeastOne(t *testing.T) {
	_, _, _, cardH := gridDimensions(1, 80, 5, 5)
	if cardH < 1 {
		t.Errorf("cardH=%d should be >= 1 even for small terminal", cardH)
	}
}

// ─── Log line routing ─────────────────────────────────────────────────────────

func TestGrid_LogLine_RoutesToCorrectCard(t *testing.T) {
	m := newGridFor(t, "host-a", "host-b")
	m, _ = sendLine(m, "host-b", "hello from b")
	if len(m.cards[0].Lines()) != 0 {
		t.Errorf("card[0] (host-a) should have 0 lines, got %d", len(m.cards[0].Lines()))
	}
	if len(m.cards[1].Lines()) != 1 {
		t.Errorf("card[1] (host-b) should have 1 line, got %d", len(m.cards[1].Lines()))
	}
}

func TestGrid_LogLine_UnknownHost_NoPanic(t *testing.T) {
	m := newGridFor(t, "host-a")
	// Should not panic and should not modify any cards.
	m, _ = sendLine(m, "unknown-host", "stray line")
	if len(m.cards[0].Lines()) != 0 {
		t.Errorf("card[0] should have 0 lines, got %d", len(m.cards[0].Lines()))
	}
}

func TestGrid_LogLines_MultipleHosts(t *testing.T) {
	m := newGridFor(t, "a", "b")
	for i := 0; i < 5; i++ {
		m, _ = sendLine(m, "a", "line from a")
	}
	for i := 0; i < 3; i++ {
		m, _ = sendLine(m, "b", "line from b")
	}
	if len(m.cards[0].Lines()) != 5 {
		t.Errorf("card[a]: got %d lines, want 5", len(m.cards[0].Lines()))
	}
	if len(m.cards[1].Lines()) != 3 {
		t.Errorf("card[b]: got %d lines, want 3", len(m.cards[1].Lines()))
	}
}

func TestGrid_LogLine_ParsedByParser(t *testing.T) {
	m := newGridFor(t, "srv")
	jsonLine := `{"level":"info","msg":"hello"}`
	m, _ = sendLine(m, "srv", jsonLine)
	lines := m.cards[0].Lines()
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if !lines[0].IsJSON {
		t.Error("expected IsJSON=true for a JSON log line")
	}
}

// ─── Window resize ────────────────────────────────────────────────────────────

func TestGrid_WindowSizeMsg_SetsSize(t *testing.T) {
	m := newGridFor(t, "srv")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.Width() != 120 {
		t.Errorf("Width: got %d, want 120", m.Width())
	}
	if m.Height() != 40 {
		t.Errorf("Height: got %d, want 40", m.Height())
	}
}

func TestGrid_WindowSizeMsg_CardsSized(t *testing.T) {
	m := newGridFor(t, "srv")
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	// The card viewport should have been resized (non-zero dimensions).
	vp := m.cards[0].Viewport()
	if vp.Width() == 0 && vp.Height() == 0 {
		t.Error("card viewport width and height are both 0 after WindowSizeMsg")
	}
}

// ─── Help toggle ──────────────────────────────────────────────────────────────

func TestGrid_HelpToggle_ChangesShowAll(t *testing.T) {
	m := newGridFor(t, "srv")
	if m.help.ShowAll {
		t.Fatal("help.ShowAll should start false")
	}
	m, _ = pressKeyGrid(m, '?')
	if !m.help.ShowAll {
		t.Error("expected help.ShowAll=true after '?'")
	}
	m, _ = pressKeyGrid(m, '?')
	if m.help.ShowAll {
		t.Error("expected help.ShowAll=false after second '?'")
	}
}

// ─── Level filter ─────────────────────────────────────────────────────────────

func TestGrid_LevelFilter_Keys(t *testing.T) {
	m := newGridFor(t, "srv")

	m, _ = pressKeyGrid(m, '2')
	if m.levelFilter != parser.LevelInfo {
		t.Errorf("'2': levelFilter got %v, want LevelInfo", m.levelFilter)
	}

	m, _ = pressKeyGrid(m, '3')
	if m.levelFilter != parser.LevelWarn {
		t.Errorf("'3': levelFilter got %v, want LevelWarn", m.levelFilter)
	}

	m, _ = pressKeyGrid(m, '4')
	if m.levelFilter != parser.LevelError {
		t.Errorf("'4': levelFilter got %v, want LevelError", m.levelFilter)
	}

	m, _ = pressKeyGrid(m, '1')
	if m.levelFilter != parser.LevelUnknown {
		t.Errorf("'1': levelFilter got %v, want LevelUnknown (all)", m.levelFilter)
	}
}

// ─── Focus navigation ─────────────────────────────────────────────────────────

func TestGrid_FocusNext_Tab_CyclesForward(t *testing.T) {
	m := newGridFor(t, "a", "b", "c")
	if m.focusedIdx != 0 {
		t.Fatalf("initial focusedIdx should be 0, got %d", m.focusedIdx)
	}
	m, _ = pressSpecialGrid(m, tea.KeyTab)
	if m.focusedIdx != 1 {
		t.Errorf("after Tab: focusedIdx=%d, want 1", m.focusedIdx)
	}
	m, _ = pressSpecialGrid(m, tea.KeyTab)
	if m.focusedIdx != 2 {
		t.Errorf("after Tab: focusedIdx=%d, want 2", m.focusedIdx)
	}
	m, _ = pressSpecialGrid(m, tea.KeyTab)
	if m.focusedIdx != 0 {
		t.Errorf("after Tab (wrap): focusedIdx=%d, want 0", m.focusedIdx)
	}
}

func TestGrid_FocusPrev_ShiftTab_CyclesBackward(t *testing.T) {
	m := newGridFor(t, "a", "b", "c")
	m, _ = pressSpecialGrid(m, tea.KeyTab, tea.ModShift)
	// 0 - 1 wraps to 2 (len=3)
	if m.focusedIdx != 2 {
		t.Errorf("after Shift+Tab from 0: focusedIdx=%d, want 2", m.focusedIdx)
	}
}

// ─── Mode transitions ─────────────────────────────────────────────────────────

func TestGrid_FilterKey_StartsCardFiltering(t *testing.T) {
	m := newGridFor(t, "srv")
	m, _ = pressKeyGrid(m, 'f')
	if !m.cards[m.focusedIdx].Filtering() {
		t.Error("expected card to be in filtering mode after 'f'")
	}
}

func TestGrid_GlobalFilter_EnterAndExit(t *testing.T) {
	m := newGridFor(t, "srv")
	m, _ = pressKeyGrid(m, 'F')
	if m.mode != ModeGlobalFilter {
		t.Errorf("mode after 'F': got %v, want ModeGlobalFilter", m.mode)
	}
	m, _ = pressSpecialGrid(m, tea.KeyEscape)
	if m.mode != ModeTail {
		t.Errorf("mode after Esc: got %v, want ModeTail", m.mode)
	}
}

func TestGrid_SearchKey_EntersSearchMode(t *testing.T) {
	m := newGridFor(t, "srv")
	m, _ = pressKeyGrid(m, 's')
	if m.mode != ModeSearch {
		t.Errorf("mode after 's': got %v, want ModeSearch", m.mode)
	}
}

func TestGrid_SearchEsc_ReturnsToTail(t *testing.T) {
	m := newGridFor(t, "srv")
	m, _ = pressKeyGrid(m, 's')     // enter search mode
	m, _ = pressSpecialGrid(m, tea.KeyEscape) // exit
	if m.mode != ModeTail {
		t.Errorf("mode after Esc from search: got %v, want ModeTail", m.mode)
	}
}

func TestGrid_Pause_TogglesOnFocusedCard(t *testing.T) {
	m := newGridFor(t, "srv")
	m, _ = pressKeyGrid(m, 'p')
	if !m.cards[0].Paused() {
		t.Error("expected card to be paused after 'p'")
	}
	m, _ = pressKeyGrid(m, 'p')
	if m.cards[0].Paused() {
		t.Error("expected card to be resumed after second 'p'")
	}
}

func TestGrid_Marker_Added(t *testing.T) {
	m := newGridFor(t, "srv")
	// Add a line first so there's something to mark.
	m, _ = sendLine(m, "srv", "something happened")
	m, _ = pressKeyGrid(m, 'm')
	if len(m.cards[0].Markers()) != 1 {
		t.Errorf("expected 1 marker, got %d", len(m.cards[0].Markers()))
	}
}

// ─── Back / quit ──────────────────────────────────────────────────────────────

func TestGrid_EscKey_SendsBackToFileList(t *testing.T) {
	m := newGridFor(t, "srv")
	m.project = config.Project{ID: "1", Name: "test"}
	m, cmd := pressSpecialGrid(m, tea.KeyEscape)
	_ = m
	msg := execGridCmd(t, cmd)
	sw, ok := msg.(SwitchMsg)
	if !ok {
		t.Fatalf("expected SwitchMsg, got %T: %v", msg, msg)
	}
	if sw.To != ScreenFileList {
		t.Errorf("expected ScreenFileList (%d), got %d", ScreenFileList, sw.To)
	}
}

func TestGrid_QuitKey_SendsQuit(t *testing.T) {
	m := newGridFor(t, "srv")
	m, cmd := pressKeyGrid(m, 'q')
	_ = m
	msg := execGridCmd(t, cmd)
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg, got %T: %v", msg, msg)
	}
}
