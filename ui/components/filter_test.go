package components

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// ─── FilterState.Active ───────────────────────────────────────────────────────

func TestFilterState_Active_None(t *testing.T) {
	f := FilterState{Mode: FilterNone, Pattern: "anything"}
	if f.Active() {
		t.Error("FilterNone should never be active")
	}
}

func TestFilterState_Active_EmptyPattern(t *testing.T) {
	f := FilterState{Mode: FilterText, Pattern: ""}
	if f.Active() {
		t.Error("empty pattern should not be active")
	}
}

func TestFilterState_Active_Text(t *testing.T) {
	f := FilterState{Mode: FilterText, Pattern: "error"}
	if !f.Active() {
		t.Error("FilterText with non-empty pattern should be active")
	}
}

// ─── NewFilterState ───────────────────────────────────────────────────────────

func TestNewFilterState_ExcludePrefix(t *testing.T) {
	f := NewFilterState("!error", FilterText, false)
	if !f.Exclude {
		t.Error("expected Exclude=true for '!' prefix")
	}
	if f.Pattern != "error" {
		t.Errorf("expected pattern='error', got %q", f.Pattern)
	}
}

func TestNewFilterState_ExcludeOnlyBang(t *testing.T) {
	// "!" alone → pattern is empty after stripping → Active() must be false.
	f := NewFilterState("!", FilterText, false)
	if f.Active() {
		t.Error("'!' with empty remainder should not be active")
	}
	if f.Pattern != "" {
		t.Errorf("expected empty pattern, got %q", f.Pattern)
	}
	if !f.Exclude {
		t.Error("Exclude should still be true")
	}
}

func TestNewFilterState_EmptyPattern(t *testing.T) {
	f := NewFilterState("", FilterText, false)
	if f.Active() {
		t.Error("empty pattern should not be active")
	}
	if f.Exclude {
		t.Error("Exclude should be false for empty pattern")
	}
}

func TestNewFilterState_RegexCompiled(t *testing.T) {
	f := NewFilterState("err.*", FilterRegex, false)
	if f.Compiled == nil {
		t.Error("valid regex should be compiled")
	}
}

func TestNewFilterState_RegexInvalid(t *testing.T) {
	// An invalid regex should not panic; Compiled should be nil.
	f := NewFilterState("[bad(regex", FilterRegex, false)
	if f.Compiled != nil {
		t.Error("invalid regex should leave Compiled=nil")
	}
	// Active() should still be true (pattern is non-empty).
	if !f.Active() {
		t.Error("Active() should be true even with invalid regex (pattern is set)")
	}
}

func TestNewFilterState_CaseInsensitiveRegexFlag(t *testing.T) {
	f := NewFilterState("error", FilterRegex, true)
	if f.Compiled == nil {
		t.Fatal("compiled regex should not be nil")
	}
	// The compiled regex should match both "error" and "ERROR".
	if !f.Compiled.MatchString("ERROR") {
		t.Error("case-insensitive regex should match 'ERROR'")
	}
}

// ─── FilterState.Matches ──────────────────────────────────────────────────────

func TestMatches_Inactive_AlwaysTrue(t *testing.T) {
	f := FilterState{Mode: FilterText, Pattern: ""}
	for _, line := range []string{"", "any text", "error", "xyz"} {
		if !f.Matches(line) {
			t.Errorf("inactive filter should match anything; failed on %q", line)
		}
	}
}

func TestMatches_Text_CaseSensitive(t *testing.T) {
	f := NewFilterState("error", FilterText, false)
	if !f.Matches("an error occurred") {
		t.Error("should match lowercase 'error'")
	}
	if f.Matches("an Error occurred") {
		t.Error("case-sensitive match should not match 'Error' with pattern 'error'")
	}
}

func TestMatches_Text_CaseInsensitive(t *testing.T) {
	f := NewFilterState("error", FilterText, true)
	if !f.Matches("an Error occurred") {
		t.Error("case-insensitive should match 'Error'")
	}
	if !f.Matches("an ERROR occurred") {
		t.Error("case-insensitive should match 'ERROR'")
	}
}

func TestMatches_Text_Exclude(t *testing.T) {
	f := NewFilterState("!debug", FilterText, false)
	if f.Matches("debug: verbose output") {
		t.Error("exclude filter should NOT match lines containing pattern")
	}
	if !f.Matches("info: server started") {
		t.Error("exclude filter SHOULD match lines not containing pattern")
	}
}

func TestMatches_Regex_Basic(t *testing.T) {
	f := NewFilterState(`err.*`, FilterRegex, false)
	if !f.Matches("error reading file") {
		t.Error("regex should match")
	}
	if f.Matches("info: connected") {
		t.Error("regex should not match unrelated line")
	}
}

func TestMatches_Regex_ExcludeInverts(t *testing.T) {
	f := NewFilterState("!err.*", FilterRegex, false)
	if f.Matches("error reading file") {
		t.Error("excluded regex match should return false")
	}
	if !f.Matches("info: no problem") {
		t.Error("excluded regex should pass non-matching line")
	}
}

func TestMatches_Regex_InvalidFallsBackToStringContains(t *testing.T) {
	// Invalid regex → Compiled is nil → falls back to strings.Contains.
	f := NewFilterState("[bad", FilterRegex, false)
	if f.Compiled != nil {
		t.Skip("regex compiled unexpectedly")
	}
	// Falls back to string contains: "[bad" as literal string.
	if !f.Matches("prefix [bad suffix") {
		t.Error("invalid regex should fall back to string contains")
	}
	if f.Matches("no match here") {
		t.Error("invalid regex fallback should not match unrelated line")
	}
}

func TestMatches_FilterNone_EmptyPattern(t *testing.T) {
	f := FilterState{Mode: FilterNone, Pattern: "error"}
	// Mode=FilterNone → Active()=false → always returns true.
	if !f.Matches("anything") {
		t.Error("FilterNone should always match regardless of pattern")
	}
}

// ─── FilterState.Highlight ───────────────────────────────────────────────────

var dummyStyle = lipgloss.NewStyle().Bold(true)

func TestHighlight_Inactive_NoChange(t *testing.T) {
	f := FilterState{Mode: FilterText, Pattern: ""}
	line := "hello world"
	got := f.Highlight(line, dummyStyle)
	if got != line {
		t.Errorf("inactive filter should return line unchanged, got %q", got)
	}
}

func TestHighlight_Exclude_NoChange(t *testing.T) {
	// Exclude filters never highlight.
	f := NewFilterState("!error", FilterText, false)
	line := "error: disk full"
	got := f.Highlight(line, dummyStyle)
	if got != line {
		t.Errorf("exclude filter should not modify line, got %q", got)
	}
}

func TestHighlight_Text_MatchFound(t *testing.T) {
	f := NewFilterState("error", FilterText, false)
	line := "error: disk full"
	got := f.Highlight(line, dummyStyle)
	// Output should differ (match is wrapped in ANSI codes).
	if got == line {
		t.Error("matched text should be highlighted (output should differ from input)")
	}
	// The non-highlighted suffix must be preserved exactly.
	if !strings.HasSuffix(got, ": disk full") {
		t.Errorf("suffix not preserved; got: %q", got)
	}
}

func TestHighlight_Text_NoMatch(t *testing.T) {
	f := NewFilterState("xyz", FilterText, false)
	line := "error: disk full"
	got := f.Highlight(line, dummyStyle)
	if got != line {
		t.Errorf("no-match should return line unchanged, got %q", got)
	}
}

func TestHighlight_Text_CaseInsensitive_PreservesOriginalCase(t *testing.T) {
	// The highlighted portion should preserve the original casing of the line,
	// not the lowercased pattern.
	f := NewFilterState("error", FilterText, true)
	line := "Error: disk full"
	got := f.Highlight(line, dummyStyle)
	// The original "Error" characters should be present in the output.
	if !strings.Contains(got, "Error") {
		t.Errorf("original case should be preserved in highlight output; got: %q", got)
	}
}

func TestHighlight_Regex_AllOccurrences(t *testing.T) {
	// Regex highlight should replace ALL occurrences.
	f := NewFilterState(`\d+`, FilterRegex, false)
	line := "user 42 logged in from 127001"
	got := f.Highlight(line, dummyStyle)
	// Output should differ from input.
	if got == line {
		t.Error("regex highlight should modify matching line")
	}
	// The non-digit parts should still be present.
	if !strings.Contains(got, "user") || !strings.Contains(got, "logged in from") {
		t.Errorf("non-matching parts should be preserved; got: %q", got)
	}
}

func TestHighlight_Regex_NoMatch(t *testing.T) {
	f := NewFilterState(`\d+`, FilterRegex, false)
	line := "no digits here"
	got := f.Highlight(line, dummyStyle)
	if got != line {
		t.Errorf("regex no-match should return line unchanged, got %q", got)
	}
}
