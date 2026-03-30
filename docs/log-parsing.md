---
title: Log Parsing (Phase 2)
description: ParsedLine struct, Level detection from JSON fields and raw text, JSON pretty-rendering with per-type colours, and DetailOverlay spec.
---

# Phase 2 — Log Parsing

## ParsedLine

```go
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
```

Phase 1: passthrough — only `Raw`, `Rendered = raw`, `Received` set.

## Level detection

Check JSON fields `"level"`, `"severity"`, `"lvl"` first, then scan raw text.

```go
var LevelColors = map[Level]lipgloss.Color{
    LevelDebug: "#888888",
    LevelInfo:  "#2ECC71",
    LevelWarn:  "#F39C12",
    LevelError: "#E74C3C",
    LevelFatal: "#FF0000",
}
```

## JSON pretty-render

Keys → `ColorPrimary`, strings → green, numbers → cyan, booleans → yellow,
null → dim. Common fields surfaced in detail view: `level`, `msg`/`message`,
`time`/`timestamp`, `error`/`err`, `caller`, `trace_id`, `request_id`.

## DetailOverlay

```go
type DetailOverlay struct {
    visible  bool
    line     parser.ParsedLine
    host     string
    viewport viewport.Model
    keys     detailKeyMap
    help     help.Model
}
```

Keys handled via `key.Matches`. Footer rendered by `help.View(d.keys)`.
`y` → `tea.SetClipboard(line.Raw)`, `Y` → `tea.SetClipboard(prettyJSON)`.
