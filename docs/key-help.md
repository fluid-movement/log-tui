---
title: Key and Help Components
description: Mandatory key.Binding/help.Model pattern for every screen; per-screen KeyMap definitions for all four screens and the detail overlay.
---

# Key and Help Components

Every screen and every component with keybindings uses `bubbles/v2/key` and
`bubbles/v2/help`. This is mandatory.

## Imports

```go
"charm.land/bubbles/v2/help"
"charm.land/bubbles/v2/key"
```

## Per-screen KeyMap pattern

Each screen defines its own `keyMap` struct satisfying `help.KeyMap`:

```go
type KeyMap interface {
    ShortHelp() []key.Binding   // shown in one-line footer
    FullHelp()  [][]key.Binding // shown in full help (toggled with ?)
}
```

Each model embeds `help.Model` and a `keyMap`:

```go
type SomeModel struct {
    keys SomeKeyMap
    help help.Model
    // ...
}
```

The footer is rendered as:

```go
func (m SomeModel) renderFooter() string {
    return m.help.View(m.keys) // short by default; full when m.help.ShowAll
}
```

Toggle with the `?` binding:

```go
case key.Matches(msg, m.keys.Help):
    m.help.ShowAll = !m.help.ShowAll
```

Disable bindings that don't apply in the current state — they disappear from
the help view automatically:

```go
m.keys.PrevMark.SetEnabled(len(m.cards[m.focusedIdx].markers) > 0)
```

## Width

Set `help.Model.Width = m.width` on every `WindowSizeMsg` so the help bar
truncates gracefully rather than wrapping.

---

## Complete KeyMap Definitions

### projects screen

```go
var projectsKeys = projectsKeyMap{
    Open:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
    New:    key.NewBinding(key.WithKeys("n"),     key.WithHelp("n", "new project")),
    Edit:   key.NewBinding(key.WithKeys("e"),     key.WithHelp("e", "edit")),
    Delete: key.NewBinding(key.WithKeys("d"),     key.WithHelp("d", "delete")),
    Help:   key.NewBinding(key.WithKeys("?"),     key.WithHelp("?", "help")),
    Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}
```

### creator screen

```go
var creatorKeys = creatorKeyMap{
    Next:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "next")),
    Toggle: key.NewBinding(key.WithKeys(" "),     key.WithHelp("space", "select host")),
    Back:   key.NewBinding(key.WithKeys("esc"),   key.WithHelp("esc", "back")),
    Quit:   key.NewBinding(key.WithKeys("ctrl+c"),key.WithHelp("ctrl+c", "quit")),
}
```

### filelist screen

```go
var filelistKeys = filelistKeyMap{
    Open:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "tail file")),
    Rescan: key.NewBinding(key.WithKeys("r"),     key.WithHelp("r", "rescan")),
    Back:   key.NewBinding(key.WithKeys("esc"),   key.WithHelp("esc", "back")),
    Help:   key.NewBinding(key.WithKeys("?"),     key.WithHelp("?", "help")),
    Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}
```

### grid screen

```go
var gridKeys = gridKeyMap{
    Up:          key.NewBinding(key.WithKeys("up", "k"),   key.WithHelp("↑/k", "scroll up")),
    Down:        key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "scroll down")),
    Top:         key.NewBinding(key.WithKeys("g"),         key.WithHelp("g", "top")),
    Bottom:      key.NewBinding(key.WithKeys("G"),         key.WithHelp("G", "bottom")),
    FocusNext:   key.NewBinding(key.WithKeys("tab"),       key.WithHelp("tab", "next panel")),
    FocusPrev:   key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev panel")),
    Filter:      key.NewBinding(key.WithKeys("f"),         key.WithHelp("f", "filter panel")),
    GlobalFilter:key.NewBinding(key.WithKeys("F"),         key.WithHelp("F", "filter all")),
    LevelDebug:  key.NewBinding(key.WithKeys("1"),         key.WithHelp("1", "show all")),
    LevelInfo:   key.NewBinding(key.WithKeys("2"),         key.WithHelp("2", "info+")),
    LevelWarn:   key.NewBinding(key.WithKeys("3"),         key.WithHelp("3", "warn+")),
    LevelError:  key.NewBinding(key.WithKeys("4"),         key.WithHelp("4", "error+")),
    Search:      key.NewBinding(key.WithKeys("s"),         key.WithHelp("s", "search")),
    Pause:       key.NewBinding(key.WithKeys("p"),         key.WithHelp("p", "pause")),
    Marker:      key.NewBinding(key.WithKeys("m"),         key.WithHelp("m", "marker")),
    PrevMarker:  key.NewBinding(key.WithKeys("b"),         key.WithHelp("b", "prev marker")),
    NextMarker:  key.NewBinding(key.WithKeys("w"),         key.WithHelp("w", "next marker")),
    Detail:      key.NewBinding(key.WithKeys("enter"),     key.WithHelp("enter", "detail")),
    Copy:        key.NewBinding(key.WithKeys("y"),         key.WithHelp("y", "copy line")),
    Back:        key.NewBinding(key.WithKeys("esc"),       key.WithHelp("esc", "back")),
    Help:        key.NewBinding(key.WithKeys("?"),         key.WithHelp("?", "help")),
    Quit:        key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

// ShortHelp: the 5-6 most important bindings for the one-line footer
func (k gridKeyMap) ShortHelp() []key.Binding {
    return []key.Binding{k.Filter, k.Search, k.Pause, k.Marker, k.Help, k.Quit}
}

// FullHelp: all bindings grouped into logical columns
func (k gridKeyMap) FullHelp() [][]key.Binding {
    return [][]key.Binding{
        {k.Up, k.Down, k.Top, k.Bottom, k.FocusNext, k.FocusPrev},
        {k.Filter, k.GlobalFilter, k.LevelDebug, k.LevelInfo, k.LevelWarn, k.LevelError},
        {k.Search, k.Pause, k.Marker, k.PrevMarker, k.NextMarker},
        {k.Detail, k.Copy, k.Back, k.Help, k.Quit},
    }
}
```

### detail overlay

```go
var detailKeys = detailKeyMap{
    Up:       key.NewBinding(key.WithKeys("up", "k"),   key.WithHelp("↑/k", "scroll up")),
    Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "scroll down")),
    Copy:     key.NewBinding(key.WithKeys("y"),         key.WithHelp("y", "copy raw")),
    CopyJSON: key.NewBinding(key.WithKeys("Y"),         key.WithHelp("Y", "copy JSON")),
    Close:    key.NewBinding(key.WithKeys("esc"),       key.WithHelp("esc", "close")),
}
```
