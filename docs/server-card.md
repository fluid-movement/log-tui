---
title: ServerCard Component
description: ServerCard struct, addLine ring buffer, pause/resume, marker rendering, FilterState, and Render() signature.
---

# ServerCard Component

```go
const maxLines = 2000

type ServerCard struct {
    host      config.Host
    viewport  viewport.Model
    status    ssh.HostStatus
    filter    FilterState
    lines     []parser.ParsedLine
    paused    bool
    pauseBuf  []parser.ParsedLine
    following bool
    markers   []int
}
```

Card-local key handling (scroll, pause, marker, filter, copy) is handled in
`GridModel.Update` via `key.Matches` against `gridKeys`. The card itself does
not own a `help.Model` — the grid's help covers all bindings.

## addLine

```go
func (c *ServerCard) addLine(line parser.ParsedLine) {
    if c.paused {
        c.pauseBuf = append(c.pauseBuf, line)
        return
    }
    if len(c.lines) >= maxLines {
        c.lines = c.lines[1:]
    }
    c.lines = append(c.lines, line)
    c.rebuildViewport()
    if c.following { c.viewport.GotoBottom() }
}
```

## Pause / resume

On resume: flush `pauseBuf` → `lines` (honour `maxLines`), clear `pauseBuf`,
rebuild viewport.

## Markers

Stored as line indices. On `rebuildViewport`, insert at each marker index:
```
──────────────── ◆ 15:04:32 ────────────────
```

## Render

```go
func (c *ServerCard) Render(focused bool, w, h int) string
// header (1 line) + filter bar (0 or 1 line) + viewport
```

Header: status badge · hostname · `[PAUSED]` · `[FILTERED: x]` · line count ·
last-received time.
