package parser

import (
	"strings"
	"testing"
	"time"
)

var testTime = time.Date(2024, 1, 2, 15, 4, 5, 0, time.UTC)

// ─── DefaultParser.Parse ──────────────────────────────────────────────────────

func TestParse_EmptyString(t *testing.T) {
	p := DefaultParser{}
	pl := p.Parse("", testTime)
	if pl.Raw != "" {
		t.Errorf("Raw: expected empty string, got %q", pl.Raw)
	}
	if pl.IsJSON {
		t.Error("IsJSON: expected false for empty string")
	}
	if pl.Level != LevelUnknown {
		t.Errorf("Level: expected LevelUnknown, got %v", pl.Level)
	}
	if pl.Received != testTime {
		t.Errorf("Received: expected %v, got %v", testTime, pl.Received)
	}
}

func TestParse_WhitespaceOnly(t *testing.T) {
	p := DefaultParser{}
	pl := p.Parse("   ", testTime)
	if pl.IsJSON {
		t.Error("whitespace-only should not be parsed as JSON")
	}
	if pl.Level != LevelUnknown {
		t.Errorf("Level: expected LevelUnknown, got %v", pl.Level)
	}
}

func TestParse_PlainText_NoLevel(t *testing.T) {
	p := DefaultParser{}
	pl := p.Parse("server started on port 8080", testTime)
	if pl.IsJSON {
		t.Error("plain text should not be parsed as JSON")
	}
	if pl.Level != LevelUnknown {
		t.Errorf("Level: expected LevelUnknown, got %v", pl.Level)
	}
	if pl.Raw != "server started on port 8080" {
		t.Errorf("Raw not preserved: %q", pl.Raw)
	}
}

func TestParse_PlainText_WithLevel(t *testing.T) {
	p := DefaultParser{}
	pl := p.Parse("[ERROR] database connection failed", testTime)
	if pl.IsJSON {
		t.Error("should not be JSON")
	}
	if pl.Level != LevelError {
		t.Errorf("expected LevelError, got %v", pl.Level)
	}
}

func TestParse_JSON_Basic(t *testing.T) {
	p := DefaultParser{}
	raw := `{"level":"info","msg":"user logged in","user_id":"42"}`
	pl := p.Parse(raw, testTime)
	if !pl.IsJSON {
		t.Fatal("expected IsJSON=true")
	}
	if pl.Level != LevelInfo {
		t.Errorf("expected LevelInfo, got %v", pl.Level)
	}
	if pl.JSONData == nil {
		t.Error("JSONData should not be nil")
	}
	if pl.Raw != raw {
		t.Errorf("Raw not preserved: %q", pl.Raw)
	}
}

func TestParse_JSON_MsgBeforeMessage(t *testing.T) {
	// "msg" must win over "message" when both are present in a JSON line.
	p := DefaultParser{}
	raw := `{"msg":"from msg","message":"from message"}`
	pl := p.Parse(raw, testTime)
	if !pl.IsJSON {
		t.Fatal("expected IsJSON=true")
	}
	// The rendered output should contain "from msg" and NOT "from message"
	// as the message portion (since "msg" is checked first).
	if !strings.Contains(pl.Rendered, "from msg") {
		t.Errorf("Rendered should contain 'from msg', got: %q", pl.Rendered)
	}
}

func TestParse_JSON_NoLevelField(t *testing.T) {
	p := DefaultParser{}
	raw := `{"msg":"hello","service":"api"}`
	pl := p.Parse(raw, testTime)
	if !pl.IsJSON {
		t.Fatal("expected IsJSON=true")
	}
	if pl.Level != LevelUnknown {
		t.Errorf("expected LevelUnknown when no level field, got %v", pl.Level)
	}
}

func TestParse_JSON_LevelFromFieldNotMsgContent(t *testing.T) {
	// If msg contains "ERROR" but level field says "info",
	// the level should come from the JSON field, not the message text.
	p := DefaultParser{}
	raw := `{"level":"info","msg":"encountered ERROR in subsystem"}`
	pl := p.Parse(raw, testTime)
	if !pl.IsJSON {
		t.Fatal("expected IsJSON=true")
	}
	if pl.Level != LevelInfo {
		t.Errorf("expected LevelInfo (from JSON field), got %v", pl.Level)
	}
}

func TestParse_JSON_NonStringLevelField(t *testing.T) {
	// A numeric "level" field should result in LevelUnknown.
	p := DefaultParser{}
	raw := `{"level":3,"msg":"log at level 3"}`
	pl := p.Parse(raw, testTime)
	if !pl.IsJSON {
		t.Fatal("expected IsJSON=true")
	}
	if pl.Level != LevelUnknown {
		t.Errorf("expected LevelUnknown for numeric level, got %v", pl.Level)
	}
}

func TestParse_JSON_EmptyObject(t *testing.T) {
	// An empty JSON object should still parse as JSON.
	p := DefaultParser{}
	pl := p.Parse("{}", testTime)
	if !pl.IsJSON {
		t.Fatal("expected IsJSON=true for {}")
	}
	if pl.Level != LevelUnknown {
		t.Errorf("expected LevelUnknown for {}, got %v", pl.Level)
	}
	// Empty object with no message fields: Rendered falls back to raw.
	if pl.Rendered != "{}" {
		t.Errorf("empty JSON Rendered should equal raw %q, got %q", "{}", pl.Rendered)
	}
}

func TestParse_PlainText_FatalBeatError(t *testing.T) {
	// "FATAL ERROR" in plain text → LevelFatal wins.
	p := DefaultParser{}
	pl := p.Parse("FATAL ERROR: out of memory", testTime)
	if pl.Level != LevelFatal {
		t.Errorf("expected LevelFatal, got %v", pl.Level)
	}
}

// ─── renderValueInline ────────────────────────────────────────────────────────

func TestRenderValueInline_StringWithSpaces(t *testing.T) {
	// Strings containing spaces must be quoted.
	got := renderValueInline("hello world")
	if !strings.HasPrefix(got, `"`) || !strings.HasSuffix(got, `"`) {
		t.Errorf("string with spaces should be quoted, got: %q", got)
	}
}

func TestRenderValueInline_StringWithoutSpaces(t *testing.T) {
	// Strings without spaces must NOT be quoted.
	got := renderValueInline("plainvalue")
	if strings.Contains(got, `"`) {
		t.Errorf("string without spaces should not be quoted, got: %q", got)
	}
	if got != "plainvalue" {
		t.Errorf("expected 'plainvalue', got: %q", got)
	}
}

func TestRenderValueInline_Nil(t *testing.T) {
	got := renderValueInline(nil)
	if got != "null" {
		t.Errorf("nil should render as 'null', got: %q", got)
	}
}

func TestRenderValueInline_StringWithTab(t *testing.T) {
	// Tabs also count as whitespace → should be quoted.
	got := renderValueInline("a\tb")
	if !strings.Contains(got, `"`) {
		t.Errorf("string with tab should be quoted, got: %q", got)
	}
}
