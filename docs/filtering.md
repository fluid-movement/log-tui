---
title: Filtering and Search Mode
description: FilterState struct with Matches/Highlight, per-card vs global filter scoping, UX flow, level quick filters, and grep-based search mode.
---

# Filtering

## FilterState

```go
type FilterMode int
const ( FilterNone FilterMode = iota; FilterText; FilterRegex )

type FilterState struct {
    Mode       FilterMode
    Pattern    string
    Compiled   *regexp.Regexp
    Exclude    bool
    IgnoreCase bool
}

func (f FilterState) Matches(line string) bool
func (f FilterState) Highlight(line string, matchStyle lipgloss.Style) string
```

## Scoping

- `f` → per-card (`FilterState` on the card)
- `F` → global (`GridModel.globalFilter`); both filters must pass

## UX

1. `f`/`F` → filter bar with `textinput` appears
2. Real-time rebuild on each keystroke
3. `ctrl+r` toggle regex; `ctrl+i` toggle case
4. `!pattern` = exclude
5. `enter` lock; `esc` clear

## Level quick filters

`1`–`4` set `GridModel.levelFilter`. No-op until Phase 2 fills `ParsedLine.Level`.

---

# Search Mode

`s` → `GridModel.mode = ModeSearch`. Runs `grep -n -E` per host via separate
SSH sessions. Results stream as `SearchResultMsg`. Each card shows a
`list.Model` of results. `enter` opens `DetailOverlay`. `esc` returns to tail.
