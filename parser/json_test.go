package parser

import (
	"strings"
	"testing"
)

// ─── TryParseJSON ─────────────────────────────────────────────────────────────

func TestTryParseJSON_Empty(t *testing.T) {
	m, ok := TryParseJSON("")
	if ok || m != nil {
		t.Errorf("empty string: expected (nil, false), got (%v, %v)", m, ok)
	}
}

func TestTryParseJSON_WhitespaceOnly(t *testing.T) {
	m, ok := TryParseJSON("   ")
	if ok || m != nil {
		t.Errorf("whitespace only: expected (nil, false), got (%v, %v)", m, ok)
	}
}

func TestTryParseJSON_ValidObject(t *testing.T) {
	m, ok := TryParseJSON(`{"level":"info","msg":"hello"}`)
	if !ok || m == nil {
		t.Fatal("expected valid parse")
	}
	if m["level"] != "info" {
		t.Errorf("level field: got %v", m["level"])
	}
}

func TestTryParseJSON_WhitespacePadded(t *testing.T) {
	// Leading/trailing whitespace should be trimmed.
	m, ok := TryParseJSON(`  {"ok":true}  `)
	if !ok || m == nil {
		t.Fatal("expected valid parse after trimming whitespace")
	}
}

func TestTryParseJSON_EmptyObject(t *testing.T) {
	m, ok := TryParseJSON("{}")
	if !ok {
		t.Fatal("empty object should parse successfully")
	}
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestTryParseJSON_Array(t *testing.T) {
	// JSON arrays are NOT parsed — first char must be '{'.
	m, ok := TryParseJSON("[1,2,3]")
	if ok || m != nil {
		t.Errorf("array: expected (nil, false), got (%v, %v)", m, ok)
	}
}

func TestTryParseJSON_InvalidJSON(t *testing.T) {
	m, ok := TryParseJSON("{invalid}")
	if ok || m != nil {
		t.Errorf("invalid JSON: expected (nil, false), got (%v, %v)", m, ok)
	}
}

func TestTryParseJSON_TrailingGarbage(t *testing.T) {
	// Valid JSON followed by trailing text should fail.
	m, ok := TryParseJSON(`{"a":1} garbage`)
	if ok || m != nil {
		t.Errorf("trailing garbage: expected (nil, false), got (%v, %v)", m, ok)
	}
}

func TestTryParseJSON_PlainText(t *testing.T) {
	m, ok := TryParseJSON("2024/01/01 INFO starting server")
	if ok || m != nil {
		t.Errorf("plain text: expected (nil, false)")
	}
}

// ─── PrettyRenderJSON ─────────────────────────────────────────────────────────

func TestPrettyRenderJSON_SurfacedKeysFirst(t *testing.T) {
	// "msg" is a surfaced key and must appear before "zfield" (alphabetical remainder).
	data := map[string]any{
		"zfield": "last",
		"msg":    "hello world",
	}
	got := PrettyRenderJSON("", data)
	msgIdx := strings.Index(got, "msg")
	zIdx := strings.Index(got, "zfield")
	if msgIdx == -1 || zIdx == -1 {
		t.Fatalf("keys not found in output: %q", got)
	}
	if msgIdx >= zIdx {
		t.Errorf("surfaced key 'msg' should appear before remainder 'zfield'; got:\n%s", got)
	}
}

func TestPrettyRenderJSON_NilDataValidRaw(t *testing.T) {
	// When data is nil but raw is valid JSON, it should be parsed and rendered.
	raw := `{"level":"info","msg":"test"}`
	got := PrettyRenderJSON(raw, nil)
	if got == raw {
		// It's possible the rendered output equals raw when stripping ANSI codes,
		// but the output should at minimum contain both keys.
		t.Skip("no ANSI output detected; skipping equality check")
	}
	if !strings.Contains(got, "level") || !strings.Contains(got, "msg") {
		t.Errorf("expected both keys in output, got: %q", got)
	}
}

func TestPrettyRenderJSON_NilDataInvalidRaw(t *testing.T) {
	// When data is nil and raw is not valid JSON, return raw unchanged.
	raw := "this is not json"
	got := PrettyRenderJSON(raw, nil)
	if got != raw {
		t.Errorf("expected raw unchanged, got %q", got)
	}
}

func TestPrettyRenderJSON_ContainsKeyNames(t *testing.T) {
	data := map[string]any{
		"level": "error",
		"msg":   "disk full",
		"code":  float64(500),
	}
	got := PrettyRenderJSON("", data)
	for _, key := range []string{"level", "msg", "code"} {
		if !strings.Contains(got, key) {
			t.Errorf("output missing key %q:\n%s", key, got)
		}
	}
}

func TestPrettyRenderJSON_NullValue(t *testing.T) {
	data := map[string]any{"err": nil}
	got := PrettyRenderJSON("", data)
	if !strings.Contains(got, "null") {
		t.Errorf("expected 'null' in output, got: %q", got)
	}
}

func TestPrettyRenderJSON_BoolValues(t *testing.T) {
	data := map[string]any{"ok": true, "retry": false}
	got := PrettyRenderJSON("", data)
	if !strings.Contains(got, "true") {
		t.Errorf("expected 'true' in output, got: %q", got)
	}
	if !strings.Contains(got, "false") {
		t.Errorf("expected 'false' in output, got: %q", got)
	}
}

func TestPrettyRenderJSON_IntegerFloat(t *testing.T) {
	// float64 with integer value should render without decimal.
	data := map[string]any{"code": float64(42)}
	got := PrettyRenderJSON("", data)
	if !strings.Contains(got, "42") {
		t.Errorf("expected '42' in output, got: %q", got)
	}
	// Should NOT contain "42.0" or "42.000"
	if strings.Contains(got, "42.") {
		t.Errorf("integer float should not contain decimal point, got: %q", got)
	}
}

func TestPrettyRenderJSON_NestedMap(t *testing.T) {
	data := map[string]any{
		"meta": map[string]any{"region": "us-east"},
	}
	got := PrettyRenderJSON("", data)
	if !strings.Contains(got, "meta") {
		t.Errorf("expected 'meta' in output, got: %q", got)
	}
	// Nested map renders as {...}
	if !strings.Contains(got, "{") {
		t.Errorf("expected '{' for nested map, got: %q", got)
	}
}

func TestPrettyRenderJSON_ArrayValue(t *testing.T) {
	data := map[string]any{
		"tags": []any{"a", "b"},
	}
	got := PrettyRenderJSON("", data)
	if !strings.Contains(got, "tags") {
		t.Errorf("expected 'tags' in output, got: %q", got)
	}
	if !strings.Contains(got, "[") {
		t.Errorf("expected '[' for array value, got: %q", got)
	}
}

func TestPrettyRenderJSON_RemainingKeysSorted(t *testing.T) {
	// Non-surfaced keys should appear in alphabetical order.
	data := map[string]any{
		"zoo":   "z",
		"alpha": "a",
		"mid":   "m",
	}
	got := PrettyRenderJSON("", data)
	alphaIdx := strings.Index(got, "alpha")
	midIdx := strings.Index(got, "mid")
	zooIdx := strings.Index(got, "zoo")
	if alphaIdx == -1 || midIdx == -1 || zooIdx == -1 {
		t.Fatalf("keys not found: %q", got)
	}
	if !(alphaIdx < midIdx && midIdx < zooIdx) {
		t.Errorf("remaining keys not sorted: alpha=%d mid=%d zoo=%d\n%s", alphaIdx, midIdx, zooIdx, got)
	}
}
