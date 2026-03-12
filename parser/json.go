package parser

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
)

// TryParseJSON attempts to parse raw as a JSON object.
func TryParseJSON(raw string) (map[string]any, bool) {
	raw = strings.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return nil, false
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, false
	}
	return m, true
}

// colour helpers
var (
	jsonKeyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#7B61FF"))
	jsonStrStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#2ECC71"))
	jsonNumStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#00BCD4"))
	jsonBoolStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#F39C12"))
	jsonNullStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
	jsonPunctStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA"))
)

// surfacedKeys are printed first in the detail view, in this order.
var surfacedKeys = []string{
	"level", "severity", "lvl",
	"msg", "message",
	"time", "timestamp", "ts",
	"error", "err",
	"caller",
	"trace_id", "request_id",
}

// PrettyRenderJSON returns a coloured multi-line representation of a JSON object.
func PrettyRenderJSON(raw string, data map[string]any) string {
	if data == nil {
		data, _ = TryParseJSON(raw)
		if data == nil {
			return raw
		}
	}

	var b strings.Builder

	printed := make(map[string]bool)
	for _, k := range surfacedKeys {
		if v, ok := data[k]; ok {
			renderKV(&b, k, v)
			printed[k] = true
		}
	}

	remaining := make([]string, 0, len(data))
	for k := range data {
		if !printed[k] {
			remaining = append(remaining, k)
		}
	}
	sort.Strings(remaining)
	for _, k := range remaining {
		renderKV(&b, k, data[k])
	}
	return b.String()
}

func renderKV(b *strings.Builder, k string, v any) {
	b.WriteString(jsonKeyStyle.Render(k))
	b.WriteString(jsonPunctStyle.Render(": "))
	b.WriteString(renderValue(v))
	b.WriteString("\n")
}

func renderValue(v any) string {
	if v == nil {
		return jsonNullStyle.Render("null")
	}
	switch val := v.(type) {
	case string:
		return jsonStrStyle.Render(fmt.Sprintf("%q", val))
	case bool:
		if val {
			return jsonBoolStyle.Render("true")
		}
		return jsonBoolStyle.Render("false")
	case float64:
		if val == float64(int64(val)) {
			return jsonNumStyle.Render(fmt.Sprintf("%d", int64(val)))
		}
		return jsonNumStyle.Render(fmt.Sprintf("%g", val))
	case map[string]any:
		raw, _ := json.Marshal(val)
		return jsonPunctStyle.Render("{") + jsonStrStyle.Render(string(raw)) + jsonPunctStyle.Render("}")
	case []any:
		raw, _ := json.Marshal(val)
		return jsonPunctStyle.Render("[") + jsonNumStyle.Render(string(raw)) + jsonPunctStyle.Render("]")
	default:
		return fmt.Sprintf("%v", v)
	}
}
