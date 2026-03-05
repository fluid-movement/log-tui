package components

import (
	"regexp"
	"testing"
	"time"

	"github.com/fluid-movement/log-tui/config"
	"github.com/fluid-movement/log-tui/parser"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func testHost() config.Host {
	return config.Host{Name: "test-host", Hostname: "10.0.0.1"}
}

func makeLine(raw string, level parser.Level) parser.ParsedLine {
	return parser.ParsedLine{
		Raw:      raw,
		Rendered: raw,
		Level:    level,
		Received: time.Now(),
	}
}

// ─── Ring buffer ─────────────────────────────────────────────────────────────

func TestAddLine_RingBuffer_ExactlyMaxLines(t *testing.T) {
	c := NewServerCard(testHost())
	for i := 0; i < maxLines; i++ {
		c.AddLine(makeLine("line", parser.LevelInfo))
	}
	if len(c.lines) != maxLines {
		t.Errorf("expected %d lines, got %d", maxLines, len(c.lines))
	}
}

func TestAddLine_RingBuffer_DropsOldest(t *testing.T) {
	c := NewServerCard(testHost())
	// Fill the buffer.
	for i := 0; i < maxLines; i++ {
		c.AddLine(makeLine("old", parser.LevelInfo))
	}
	// Add one more with distinct content.
	c.AddLine(makeLine("newest", parser.LevelWarn))

	if len(c.lines) != maxLines {
		t.Errorf("ring buffer overflow: expected %d, got %d", maxLines, len(c.lines))
	}
	// Last element should be the new line.
	last := c.lines[len(c.lines)-1]
	if last.Raw != "newest" {
		t.Errorf("last line should be 'newest', got %q", last.Raw)
	}
	// First element should NOT be the very first "old" line — it was dropped.
	// All remaining should be "old" except the last.
	if c.lines[0].Raw != "old" {
		t.Errorf("expected first line to still be 'old' (just the very first dropped), got %q", c.lines[0].Raw)
	}
}

func TestAddLine_RingBuffer_Overfill(t *testing.T) {
	// Add 2×maxLines; only the last maxLines should be retained.
	c := NewServerCard(testHost())
	for i := 0; i < maxLines; i++ {
		c.AddLine(makeLine("early", parser.LevelInfo))
	}
	for i := 0; i < maxLines; i++ {
		c.AddLine(makeLine("late", parser.LevelInfo))
	}
	if len(c.lines) != maxLines {
		t.Errorf("expected %d lines after overfill, got %d", maxLines, len(c.lines))
	}
	for i, line := range c.lines {
		if line.Raw != "late" {
			t.Errorf("line %d should be 'late', got %q", i, line.Raw)
		}
	}
}

// ─── Pause / Resume ──────────────────────────────────────────────────────────

func TestPause_BuffersLines(t *testing.T) {
	c := NewServerCard(testHost())
	c.Pause()
	if !c.paused {
		t.Fatal("card should be paused")
	}

	c.AddLine(makeLine("buffered1", parser.LevelInfo))
	c.AddLine(makeLine("buffered2", parser.LevelInfo))

	if len(c.lines) != 0 {
		t.Errorf("lines should be empty while paused, got %d", len(c.lines))
	}
	if len(c.pauseBuf) != 2 {
		t.Errorf("pauseBuf should have 2 entries, got %d", len(c.pauseBuf))
	}
}

func TestResume_FlushesInOrder(t *testing.T) {
	c := NewServerCard(testHost())
	c.AddLine(makeLine("before", parser.LevelInfo))
	c.Pause()
	c.AddLine(makeLine("paused1", parser.LevelInfo))
	c.AddLine(makeLine("paused2", parser.LevelInfo))
	c.Resume()

	if c.paused {
		t.Error("card should not be paused after Resume")
	}
	if len(c.pauseBuf) != 0 {
		t.Errorf("pauseBuf should be empty after Resume, got %d", len(c.pauseBuf))
	}
	if len(c.lines) != 3 {
		t.Errorf("expected 3 lines after resume, got %d", len(c.lines))
	}
	if c.lines[0].Raw != "before" {
		t.Errorf("first line should be 'before', got %q", c.lines[0].Raw)
	}
	if c.lines[1].Raw != "paused1" {
		t.Errorf("second line should be 'paused1', got %q", c.lines[1].Raw)
	}
	if c.lines[2].Raw != "paused2" {
		t.Errorf("third line should be 'paused2', got %q", c.lines[2].Raw)
	}
}

func TestResume_PauseBufOverflow(t *testing.T) {
	// Fill lines near capacity, then pause and add more than remaining capacity.
	c := NewServerCard(testHost())
	// Add maxLines-1 lines.
	for i := 0; i < maxLines-1; i++ {
		c.AddLine(makeLine("pre", parser.LevelInfo))
	}
	c.Pause()
	// Add 10 lines during pause — only 1 will fit before hitting maxLines.
	for i := 0; i < 10; i++ {
		c.AddLine(makeLine("paused", parser.LevelInfo))
	}
	c.Resume()

	if len(c.lines) != maxLines {
		t.Errorf("expected maxLines=%d after resume, got %d", maxLines, len(c.lines))
	}
}

func TestTogglePause(t *testing.T) {
	c := NewServerCard(testHost())
	if c.Paused() {
		t.Error("should start unpaused")
	}
	c.TogglePause()
	if !c.Paused() {
		t.Error("should be paused after toggle")
	}
	c.TogglePause()
	if c.Paused() {
		t.Error("should be unpaused after second toggle")
	}
}

// ─── Markers ─────────────────────────────────────────────────────────────────

func TestAddMarker_NoLines(t *testing.T) {
	c := NewServerCard(testHost())
	// Should not panic and markers should remain empty.
	c.AddMarker()
	if len(c.markers) != 0 {
		t.Errorf("expected no markers with no lines, got %v", c.markers)
	}
}

func TestAddMarker_WithLines(t *testing.T) {
	c := NewServerCard(testHost())
	for i := 0; i < 5; i++ {
		c.AddLine(makeLine("line", parser.LevelInfo))
	}
	c.AddMarker()
	if len(c.markers) != 1 {
		t.Fatalf("expected 1 marker, got %d", len(c.markers))
	}
	// Marker should point to the last line index (4).
	if c.markers[0] != 4 {
		t.Errorf("marker should be at index 4, got %d", c.markers[0])
	}
}

func TestAddMarker_Multiple(t *testing.T) {
	c := NewServerCard(testHost())
	c.AddLine(makeLine("a", parser.LevelInfo))
	c.AddMarker() // index 0
	c.AddLine(makeLine("b", parser.LevelInfo))
	c.AddMarker() // index 1
	if len(c.markers) != 2 {
		t.Errorf("expected 2 markers, got %d", len(c.markers))
	}
}

func TestJumpToPrevMarker_NoMarkers(t *testing.T) {
	c := NewServerCard(testHost())
	// Should not panic.
	c.JumpToPrevMarker()
}

func TestJumpToNextMarker_NoMarkers(t *testing.T) {
	c := NewServerCard(testHost())
	// Should not panic.
	c.JumpToNextMarker()
}

func TestMarkers_Accessor(t *testing.T) {
	c := NewServerCard(testHost())
	c.AddLine(makeLine("x", parser.LevelInfo))
	c.AddMarker()
	if len(c.Markers()) != 1 {
		t.Errorf("Markers() should return 1 marker, got %d", len(c.Markers()))
	}
}

// ─── Level filter ─────────────────────────────────────────────────────────────

func TestSetLevelFilter_HidesLowerLevels(t *testing.T) {
	c := NewServerCard(testHost())
	c.AddLine(makeLine("debug msg", parser.LevelDebug))
	c.AddLine(makeLine("info msg", parser.LevelInfo))
	c.AddLine(makeLine("warn msg", parser.LevelWarn))
	c.AddLine(makeLine("error msg", parser.LevelError))

	// Set minimum level to Warn — Debug and Info should be filtered out.
	c.SetLevelFilter(parser.LevelWarn)

	// Check levelFilter field.
	if c.levelFilter != parser.LevelWarn {
		t.Errorf("levelFilter should be LevelWarn, got %v", c.levelFilter)
	}

	// Verify lines are still in the buffer (filter is display-only).
	if len(c.lines) != 4 {
		t.Errorf("all 4 lines should remain in buffer, got %d", len(c.lines))
	}
}

func TestSetLevelFilter_UnknownShowsAll(t *testing.T) {
	c := NewServerCard(testHost())
	c.AddLine(makeLine("debug msg", parser.LevelDebug))
	c.SetLevelFilter(parser.LevelUnknown)
	// LevelUnknown means no level filter active.
	if c.levelFilter != parser.LevelUnknown {
		t.Errorf("expected LevelUnknown filter, got %v", c.levelFilter)
	}
}

// ─── Global + per-card filter interaction ────────────────────────────────────

func TestGlobalAndCardFilter_BothMustMatch(t *testing.T) {
	c := NewServerCard(testHost())
	c.AddLine(makeLine("user login error", parser.LevelError))
	c.AddLine(makeLine("user login success", parser.LevelInfo))
	c.AddLine(makeLine("admin action error", parser.LevelError))

	// Global filter: must contain "user"
	c.SetGlobalFilter(NewFilterState("user", FilterText, false))
	// Per-card filter: must contain "error"
	c.Filter = NewFilterState("error", FilterText, false)
	c.rebuildViewport()

	// Only "user login error" matches both filters.
	// Check that exactly one line passes both filters by counting viewLines in viewport.
	content := c.viewport.View()
	// "user login error" should appear.
	if !containsString(content, "user login error") {
		t.Errorf("expected 'user login error' to pass both filters; viewport content: %q", content)
	}
	// "user login success" fails per-card filter.
	if containsString(content, "user login success") {
		t.Errorf("'user login success' should be filtered by per-card filter")
	}
	// "admin action error" fails global filter.
	if containsString(content, "admin action error") {
		t.Errorf("'admin action error' should be filtered by global filter")
	}
}

// stripANSI removes ANSI escape codes so we can search plain text.
func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

func containsString(s, sub string) bool {
	plain := stripANSI(s)
	return len(sub) > 0 && len(plain) >= len(sub) && (func() bool {
		for i := 0; i <= len(plain)-len(sub); i++ {
			if plain[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

// ─── markerViewportOffset ─────────────────────────────────────────────────────

func TestMarkerViewportOffset_NoMarkers(t *testing.T) {
	c := NewServerCard(testHost())
	// With no markers, offset = lineIdx.
	got := c.markerViewportOffset(5)
	if got != 5 {
		t.Errorf("expected offset 5 with no markers, got %d", got)
	}
}

func TestMarkerViewportOffset_MarkerBeforeTarget(t *testing.T) {
	c := NewServerCard(testHost())
	c.markers = []int{2} // marker at line 2
	// For lineIdx=5, marker at 2 is < 5, so offset = 5 + 1 = 6.
	got := c.markerViewportOffset(5)
	if got != 6 {
		t.Errorf("expected offset 6 with marker before target, got %d", got)
	}
}

func TestMarkerViewportOffset_MarkerAtTarget(t *testing.T) {
	c := NewServerCard(testHost())
	c.markers = []int{5} // marker at line 5
	// Marker at 5 is NOT < 5, so offset = 5 (no extra offset).
	got := c.markerViewportOffset(5)
	if got != 5 {
		t.Errorf("expected offset 5 when marker is at target (not before it), got %d", got)
	}
}

func TestMarkerViewportOffset_MultipleMarkersBeforeTarget(t *testing.T) {
	c := NewServerCard(testHost())
	c.markers = []int{1, 3, 7} // two before lineIdx=5, one after
	// Markers at 1 and 3 are < 5, so offset = 5 + 2 = 7.
	got := c.markerViewportOffset(5)
	if got != 7 {
		t.Errorf("expected offset 7, got %d", got)
	}
}
