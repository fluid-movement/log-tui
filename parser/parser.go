package parser

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// Parser turns a raw log line into a ParsedLine.
type Parser interface {
	Parse(raw string, received time.Time) ParsedLine
}

// ParsedLine holds both the original and processed form of a log line.
type ParsedLine struct {
	Raw       string
	Rendered  string
	Level     Level
	IsJSON    bool
	JSONData  map[string]any
	Fields    map[string]string
	Timestamp *time.Time
	Received  time.Time
}

// DefaultParser is the full Phase 2 implementation.
type DefaultParser struct{}

var levelStyle = map[Level]lipgloss.Style{
	LevelDebug: lipgloss.NewStyle().Foreground(LevelColors[LevelDebug]),
	LevelInfo:  lipgloss.NewStyle().Foreground(LevelColors[LevelInfo]),
	LevelWarn:  lipgloss.NewStyle().Foreground(LevelColors[LevelWarn]),
	LevelError: lipgloss.NewStyle().Foreground(LevelColors[LevelError]),
	LevelFatal: lipgloss.NewStyle().Foreground(LevelColors[LevelFatal]).Bold(true),
}

// Parse parses a raw log line into a ParsedLine.
func (p DefaultParser) Parse(raw string, received time.Time) ParsedLine {
	pl := ParsedLine{
		Raw:      raw,
		Received: received,
	}

	// Try JSON first.
	if data, ok := TryParseJSON(raw); ok {
		pl.IsJSON = true
		pl.JSONData = data
		pl.Level = DetectLevelFromJSON(data)
		pl.Rendered = renderJSONLine(raw, data, pl.Level)
		return pl
	}

	// Plain text: detect level from content.
	pl.Level = DetectLevelFromText(raw)
	pl.Rendered = renderPlainLine(raw, pl.Level)
	return pl
}

func renderJSONLine(raw string, data map[string]any, level Level) string {
	var parts []string

	// Level prefix
	if level != LevelUnknown {
		if style, ok := levelStyle[level]; ok {
			parts = append(parts, style.Render(LevelPrefix(level)))
		}
	}

	// msg / message
	for _, k := range []string{"msg", "message"} {
		if v, ok := data[k]; ok {
			if s, ok := v.(string); ok {
				parts = append(parts, s)
				break
			}
		}
	}

	// Remaining key=value pairs (skip fields shown in prefix/msg).
	skip := map[string]bool{
		"level": true, "severity": true, "lvl": true,
		"msg": true, "message": true,
		"time": true, "timestamp": true, "ts": true,
	}
	for k, v := range data {
		if skip[k] {
			continue
		}
		parts = append(parts, k+"="+renderValueInline(v))
	}

	if len(parts) == 0 {
		return raw
	}
	return strings.Join(parts, " ")
}

func renderValueInline(v any) string {
	if v == nil {
		return "null"
	}
	switch val := v.(type) {
	case string:
		if strings.ContainsAny(val, " \t") {
			return `"` + val + `"`
		}
		return val
	default:
		_ = val
		return renderValue(v) // from json.go
	}
}

func renderPlainLine(raw string, level Level) string {
	if level == LevelUnknown {
		return raw
	}
	style, ok := levelStyle[level]
	if !ok {
		return raw
	}
	return style.Render(raw)
}
