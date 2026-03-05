package parser

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// Level represents the severity of a log line.
type Level int

const (
	LevelUnknown Level = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// LevelColors maps each level to a display colour.
var LevelColors = map[Level]color.Color{
	LevelDebug: lipgloss.Color("#888888"),
	LevelInfo:  lipgloss.Color("#2ECC71"),
	LevelWarn:  lipgloss.Color("#F39C12"),
	LevelError: lipgloss.Color("#E74C3C"),
	LevelFatal: lipgloss.Color("#FF0000"),
}

// levelLabels maps normalised level strings to Level constants.
var levelLabels = map[string]Level{
	"debug": LevelDebug,
	"dbg":   LevelDebug,
	"trace": LevelDebug,
	"info":  LevelInfo,
	"information": LevelInfo,
	"warn":  LevelWarn,
	"warning": LevelWarn,
	"error": LevelError,
	"err":   LevelError,
	"fatal": LevelFatal,
	"critical": LevelFatal,
	"panic": LevelFatal,
}

// DetectLevel returns the log level for a raw line.
// Checks JSON fields first, then scans raw text.
func DetectLevel(raw string) Level {
	return LevelUnknown
}

// DetectLevelFromJSON extracts the level from a parsed JSON object.
func DetectLevelFromJSON(data map[string]any) Level {
	for _, key := range []string{"level", "severity", "lvl", "log_level"} {
		if v, ok := data[key]; ok {
			if s, ok := v.(string); ok {
				if lvl, found := levelLabels[strings.ToLower(strings.TrimSpace(s))]; found {
					return lvl
				}
			}
		}
	}
	return LevelUnknown
}

// DetectLevelFromText scans raw log text for common level patterns.
func DetectLevelFromText(raw string) Level {
	upper := strings.ToUpper(raw)
	for _, token := range []struct {
		substr string
		level  Level
	}{
		{"FATAL", LevelFatal},
		{"CRITICAL", LevelFatal},
		{"PANIC", LevelFatal},
		{"ERROR", LevelError},
		{" ERR ", LevelError},
		{"[ERR]", LevelError},
		{"WARNING", LevelWarn},
		{"[WARN]", LevelWarn},
		{" WARN ", LevelWarn},
		{"[INFO]", LevelInfo},
		{" INFO ", LevelInfo},
		{"[DEBUG]", LevelDebug},
		{" DEBUG ", LevelDebug},
		{"[TRACE]", LevelDebug},
	} {
		if strings.Contains(upper, token.substr) {
			return token.level
		}
	}
	return LevelUnknown
}

// LevelPrefix returns a fixed-width 7-char label for a level.
func LevelPrefix(l Level) string {
	switch l {
	case LevelDebug:
		return "[DEBUG]"
	case LevelInfo:
		return "[INFO] "
	case LevelWarn:
		return "[WARN] "
	case LevelError:
		return "[ERROR]"
	case LevelFatal:
		return "[FATAL]"
	default:
		return "       "
	}
}
