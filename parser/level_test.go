package parser

import (
	"testing"
)

// ─── DetectLevelFromJSON ──────────────────────────────────────────────────────

func TestDetectLevelFromJSON_KeyPriority(t *testing.T) {
	// "level" must win over "severity" when both are present.
	data := map[string]any{
		"level":    "info",
		"severity": "error",
	}
	got := DetectLevelFromJSON(data)
	if got != LevelInfo {
		t.Errorf("expected LevelInfo, got %v", got)
	}
}

func TestDetectLevelFromJSON_SeverityFallback(t *testing.T) {
	// "severity" is used when "level" is absent.
	data := map[string]any{"severity": "warn"}
	got := DetectLevelFromJSON(data)
	if got != LevelWarn {
		t.Errorf("expected LevelWarn, got %v", got)
	}
}

func TestDetectLevelFromJSON_LvlFallback(t *testing.T) {
	data := map[string]any{"lvl": "debug"}
	if got := DetectLevelFromJSON(data); got != LevelDebug {
		t.Errorf("expected LevelDebug, got %v", got)
	}
}

func TestDetectLevelFromJSON_LogLevelFallback(t *testing.T) {
	data := map[string]any{"log_level": "fatal"}
	if got := DetectLevelFromJSON(data); got != LevelFatal {
		t.Errorf("expected LevelFatal, got %v", got)
	}
}

func TestDetectLevelFromJSON_CaseInsensitiveValue(t *testing.T) {
	cases := []struct {
		val  string
		want Level
	}{
		{"INFO", LevelInfo},
		{"Info", LevelInfo},
		{"  info  ", LevelInfo},
		{"WARNING", LevelWarn},
		{"Error", LevelError},
		{"FATAL", LevelFatal},
		{"CRITICAL", LevelFatal},
		{"PANIC", LevelFatal},
		{"trace", LevelDebug},
		{"DBG", LevelDebug},
	}
	for _, tc := range cases {
		data := map[string]any{"level": tc.val}
		got := DetectLevelFromJSON(data)
		if got != tc.want {
			t.Errorf("level=%q: expected %v, got %v", tc.val, tc.want, got)
		}
	}
}

func TestDetectLevelFromJSON_UnknownValue(t *testing.T) {
	data := map[string]any{"level": "verbose"}
	if got := DetectLevelFromJSON(data); got != LevelUnknown {
		t.Errorf("expected LevelUnknown for unknown label, got %v", got)
	}
}

func TestDetectLevelFromJSON_NonStringLevelField(t *testing.T) {
	// A non-string "level" value (e.g. integer) should return LevelUnknown.
	data := map[string]any{"level": float64(42)}
	if got := DetectLevelFromJSON(data); got != LevelUnknown {
		t.Errorf("expected LevelUnknown for numeric level, got %v", got)
	}
}

func TestDetectLevelFromJSON_EmptyMap(t *testing.T) {
	if got := DetectLevelFromJSON(map[string]any{}); got != LevelUnknown {
		t.Errorf("expected LevelUnknown for empty map, got %v", got)
	}
}

func TestDetectLevelFromJSON_NilMap(t *testing.T) {
	// Should not panic on nil map.
	if got := DetectLevelFromJSON(nil); got != LevelUnknown {
		t.Errorf("expected LevelUnknown for nil map, got %v", got)
	}
}

// ─── DetectLevelFromText ──────────────────────────────────────────────────────

func TestDetectLevelFromText_FatalBeforeError(t *testing.T) {
	// "FATAL ERROR" must return LevelFatal, not LevelError.
	got := DetectLevelFromText("FATAL ERROR: something bad")
	if got != LevelFatal {
		t.Errorf("expected LevelFatal, got %v", got)
	}
}

func TestDetectLevelFromText_CaseInsensitive(t *testing.T) {
	cases := []struct {
		raw  string
		want Level
	}{
		{"fatal crash", LevelFatal},
		{"critical failure", LevelFatal},
		{"panic: runtime error", LevelFatal},
		{"error reading file", LevelError},
		{"2024/01/02 [ERROR] bad", LevelError},
		{"WARNING: disk full", LevelWarn},
		{"[WARN] low memory", LevelWarn},
		{"time=12:00 [INFO] started", LevelInfo},
		{"some  info  message", LevelInfo},
		{"[DEBUG] breakpoint hit", LevelDebug},
		{"some  debug  logs", LevelDebug},
		{"[TRACE] enter func", LevelDebug},
		{"no level here", LevelUnknown},
	}
	for _, tc := range cases {
		got := DetectLevelFromText(tc.raw)
		if got != tc.want {
			t.Errorf("raw=%q: expected %v, got %v", tc.raw, tc.want, got)
		}
	}
}

func TestDetectLevelFromText_ErrSubstringWithSpaces(t *testing.T) {
	// " ERR " (with spaces) matches LevelError.
	got := DetectLevelFromText("connection err failed")
	// "ERR" alone (without brackets or spaces) does NOT match — only " ERR ".
	// "connection err failed" uppercased → "CONNECTION ERR FAILED" — contains " ERR " → match
	if got != LevelError {
		t.Errorf("expected LevelError for ' ERR ' token, got %v", got)
	}
}

func TestDetectLevelFromText_ErrorInURL(t *testing.T) {
	// "error" in a URL like "/user/errors" still matches because
	// DetectLevelFromText uses strings.Contains on the whole uppercased line.
	// This is intentional (substring match, not word boundary).
	got := DetectLevelFromText("GET /user/errors 200")
	if got != LevelError {
		t.Errorf("expected LevelError (substring match in URL), got %v", got)
	}
}

func TestDetectLevelFromText_Empty(t *testing.T) {
	if got := DetectLevelFromText(""); got != LevelUnknown {
		t.Errorf("expected LevelUnknown for empty string, got %v", got)
	}
}

// ─── LevelPrefix ─────────────────────────────────────────────────────────────

func TestLevelPrefix_Width(t *testing.T) {
	// Every prefix must be exactly 7 characters wide.
	levels := []Level{LevelUnknown, LevelDebug, LevelInfo, LevelWarn, LevelError, LevelFatal}
	for _, l := range levels {
		p := LevelPrefix(l)
		if len(p) != 7 {
			t.Errorf("LevelPrefix(%v) = %q has length %d, want 7", l, p, len(p))
		}
	}
}

func TestLevelPrefix_UnknownIsSpaces(t *testing.T) {
	p := LevelPrefix(LevelUnknown)
	if p != "       " {
		t.Errorf("LevelPrefix(LevelUnknown) = %q, want 7 spaces", p)
	}
}

func TestLevelPrefix_Values(t *testing.T) {
	cases := []struct {
		level Level
		want  string
	}{
		{LevelDebug, "[DEBUG]"},
		{LevelInfo, "[INFO] "},
		{LevelWarn, "[WARN] "},
		{LevelError, "[ERROR]"},
		{LevelFatal, "[FATAL]"},
	}
	for _, tc := range cases {
		got := LevelPrefix(tc.level)
		if got != tc.want {
			t.Errorf("LevelPrefix(%v) = %q, want %q", tc.level, got, tc.want)
		}
	}
}
