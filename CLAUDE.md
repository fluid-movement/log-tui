# logviewer — Claude Code Instructions

Read this file before every task. Read `SPEC.md` before making any
architectural decisions or implementing any screen, component, or package.

---

## Critical: Charm v2 API (breaking changes from v1)

### Import paths — charm.land, NOT github.com

```go
import (
    tea  "charm.land/bubbletea/v2"
    "charm.land/bubbles/v2/help"
    "charm.land/bubbles/v2/key"
    "charm.land/bubbles/v2/list"
    "charm.land/bubbles/v2/viewport"
    "charm.land/bubbles/v2/textinput"
    "charm.land/bubbles/v2/spinner"
    "charm.land/lipgloss/v2"
)
```

### View() returns tea.View, NOT string

```go
// WRONG
func (m Model) View() string { return "..." }

// CORRECT
func (m Model) View() tea.View {
    v := tea.NewView(content)
    v.AltScreen = true
    return v
}
```

### Key handling uses key.Matches, NOT raw string switches

```go
// WRONG — raw string switch
case tea.KeyPressMsg:
    switch msg.String() { case "j": ... }

// CORRECT — key.Matches against a KeyMap binding
case tea.KeyPressMsg:
    switch {
    case key.Matches(msg, m.keys.Down):
        ...
    case key.Matches(msg, m.keys.Quit):
        return m, tea.Quit
    }
```

### AltScreen and mouse go in View(), NOT NewProgram()

```go
// WRONG
p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

// CORRECT
p := tea.NewProgram(m)
// then in View():
v.AltScreen = true
v.MouseMode = tea.MouseModeCellMotion
```

### Clipboard (OSC52, works over SSH)

```go
return m, tea.SetClipboard("text")  // copy
return m, tea.ReadClipboard()       // read
case tea.ClipboardMsg: msg.String() // receive
```

---

## Key and Help components (mandatory pattern)

Every screen and component that has keybindings MUST use `bubbles/v2/key` and
`bubbles/v2/help`. No exceptions. Do not render help text manually.

### Define a KeyMap for every screen/component

```go
import (
    "charm.land/bubbles/v2/help"
    "charm.land/bubbles/v2/key"
)

type gridKeyMap struct {
    Up      key.Binding
    Down    key.Binding
    Focus   key.Binding
    Filter  key.Binding
    Pause   key.Binding
    Marker  key.Binding
    PrevMark key.Binding
    NextMark key.Binding
    Search  key.Binding
    Copy    key.Binding
    Help    key.Binding
    Quit    key.Binding
}

// ShortHelp returns the minimal bindings shown in the one-line footer.
func (k gridKeyMap) ShortHelp() []key.Binding {
    return []key.Binding{k.Filter, k.Search, k.Pause, k.Help, k.Quit}
}

// FullHelp returns all bindings, grouped into columns, shown on '?'.
func (k gridKeyMap) FullHelp() [][]key.Binding {
    return [][]key.Binding{
        {k.Up, k.Down, k.Focus},
        {k.Filter, k.Search},
        {k.Pause, k.Marker, k.PrevMark, k.NextMark},
        {k.Copy, k.Help, k.Quit},
    }
}

var defaultGridKeys = gridKeyMap{
    Up: key.NewBinding(
        key.WithKeys("up", "k"),
        key.WithHelp("↑/k", "scroll up"),
    ),
    Down: key.NewBinding(
        key.WithKeys("down", "j"),
        key.WithHelp("↓/j", "scroll down"),
    ),
    PrevMark: key.NewBinding(
        key.WithKeys("b"),
        key.WithHelp("b", "prev marker"),
    ),
    NextMark: key.NewBinding(
        key.WithKeys("w"),
        key.WithHelp("w", "next marker"),
    ),
    // ... etc
}
```

### Embed help.Model in every screen/component model

```go
type GridModel struct {
    keys gridKeyMap
    help help.Model
    // ...
}

func NewGridModel(...) GridModel {
    return GridModel{
        keys: defaultGridKeys,
        help: help.New(),
    }
}
```

### Render the help footer using help.Model.View()

```go
func (m GridModel) renderFooter() string {
    if m.help.ShowAll {
        return m.help.FullHelpView(m.keys.FullHelp())
    }
    return m.help.View(m.keys)   // renders ShortHelp
}

// Toggle with '?':
case key.Matches(msg, m.keys.Help):
    m.help.ShowAll = !m.help.ShowAll
```

### Check keys with key.Matches, never msg.String()

```go
// In Update:
case tea.KeyPressMsg:
    switch {
    case key.Matches(msg, m.keys.Up):
        m.cards[m.focusedIdx].viewport.LineUp(1)
    case key.Matches(msg, m.keys.PrevMark):
        m.cards[m.focusedIdx].jumpToPrevMarker()
    case key.Matches(msg, m.keys.NextMark):
        m.cards[m.focusedIdx].jumpToNextMarker()
    }
```

### Disable bindings contextually

When a binding is not applicable in the current state, disable it so it
disappears from the help view automatically:

```go
m.keys.PrevMark.SetEnabled(len(m.cards[m.focusedIdx].markers) > 0)
m.keys.Search.SetEnabled(m.mode == ModeTail)
```

---

## Key binding reference (QWERTZ-safe)

All keys are reachable without modifier on a German QWERTZ keyboard.

| Binding     | Key(s)        | Note                              |
|-------------|---------------|-----------------------------------|
| ScrollUp    | `↑`, `k`      |                                   |
| ScrollDown  | `↓`, `j`      |                                   |
| Top         | `g`           |                                   |
| Bottom      | `G`           |                                   |
| FocusNext   | `tab`         |                                   |
| FocusPrev   | `shift+tab`   |                                   |
| Filter      | `f`           | per-card                          |
| GlobalFilter| `F`           | all cards                         |
| ToggleRegex | `ctrl+r`      | inside filter bar                 |
| ToggleCase  | `ctrl+i`      | inside filter bar                 |
| LevelDebug  | `1`           |                                   |
| LevelInfo   | `2`           |                                   |
| LevelWarn   | `3`           |                                   |
| LevelError  | `4`           |                                   |
| Search      | `s`           |                                   |
| Pause       | `p`           |                                   |
| Marker      | `m`           |                                   |
| PrevMarker  | `b`           | NOT `[` — unusable on QWERTZ      |
| NextMarker  | `w`           | NOT `]` — unusable on QWERTZ      |
| Detail      | `enter`       |                                   |
| Copy        | `y`           |                                   |
| CopyJSON    | `Y`           | detail overlay only               |
| Help        | `?`           | toggle short/full help            |
| Back/Close  | `esc`         |                                   |
| Quit        | `q`, `ctrl+c` |                                   |

---

## Non-negotiable rules

- **Never hardcode `~/.config`** — always use `os.UserConfigDir()`
- **Always `tail -F`** (capital F), never `tail -f` — handles log rotation
- **Never write to stdout** — the TUI owns it; use the file logger
- **Never auto-accept unknown SSH host keys** — always prompt the user
- **Never share `*gossh.Session` between goroutines** — sessions are not
  concurrent-safe; `*gossh.Client` may be shared
- **Always set explicit `lipgloss.NewStyle().Width(n)` on grid cells**
- **Always propagate `WindowSizeMsg` to all sub-models** including inactive screens
- **Always call `viewport.SetSize(w, h)` on every `WindowSizeMsg`** for every card
- **Re-issue `listenForLog(ch)` after every `LogLineMsg` received**
- **Enforce `maxLines = 2000` ring buffer per card**
- **Use `json:",omitempty"` on all new config struct fields**
- **Never render help text manually** — always use `help.Model`
- **Never match keys with `msg.String()`** — always use `key.Matches`

---

## Project structure

```
logviewer/
├── main.go
├── config/
│   ├── config.go       # load/save JSON, os.UserConfigDir(), migration
│   └── ssh.go          # ~/.ssh/config parser, host validation
├── ssh/
│   ├── client.go       # connect, auth, knownhosts, typed errors, reconnect
│   └── tail.go         # tail -F streaming, grep for search
├── ui/
│   ├── app.go          # root model, screen routing, SwitchScreenMsg
│   ├── screens/
│   │   ├── projects.go # KeyMap + help.Model
│   │   ├── creator.go  # KeyMap + help.Model
│   │   ├── filelist.go # KeyMap + help.Model
│   │   └── grid.go     # KeyMap + help.Model
│   ├── components/
│   │   ├── card.go     # KeyMap for card-local bindings
│   │   └── detail.go   # KeyMap + help.Model (Phase 2)
│   └── styles/
│       └── styles.go   # all lipgloss styles — edit here, nowhere else
└── parser/
    ├── parser.go
    ├── json.go
    └── level.go
```

---

## Streaming pattern (critical)

```go
func listenForLog(ch chan ssh.LogLineMsg) tea.Cmd {
    return func() tea.Msg { return <-ch }
}

// In Update — ALWAYS re-issue after receiving:
case ssh.LogLineMsg:
    m.cards[idx].addLine(...)
    return m, listenForLog(m.logChans[idx])
```

---

## Screen transition pattern

```go
type SwitchScreenMsg struct {
    To      screen
    Payload any
}
```

---

## Config storage

```go
base, _ := os.UserConfigDir()
// macOS   → ~/Library/Application Support/logviewer/
// Linux   → ~/.config/logviewer/
// Windows → %AppData%\logviewer\
path := filepath.Join(base, "logviewer", "projects.json")
```

---

## Debug logging

```go
// Only active when LOG_DEBUG=1 — never use fmt.Println
log.SetOutput(file)  // github.com/charmbracelet/log
```

Run with: `LOG_DEBUG=1 ./logviewer` then `tail -f /tmp/logviewer-debug.log`