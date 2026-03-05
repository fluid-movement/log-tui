# logviewer — Claude Code Instructions

Read this file before every task. Read `SPEC.md` before making any
architectural decisions or implementing any screen, component, or package.

---

## Critical: Charm v2 API (breaking changes from v1)

### Import paths — charm.land, NOT github.com

```go
import (
    tea "charm.land/bubbletea/v2"
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

### Key messages use tea.KeyPressMsg, NOT tea.KeyMsg

```go
// WRONG
case tea.KeyMsg:

// CORRECT
case tea.KeyPressMsg:
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

## Non-negotiable rules

- **Never hardcode `~/.config`** — always use `os.UserConfigDir()`
- **Always `tail -F`** (capital F), never `tail -f` — handles log rotation
- **Never write to stdout** — the TUI owns it; use the file logger
- **Never auto-accept unknown SSH host keys** — always prompt the user
- **Never share `*gossh.Session` between goroutines** — sessions are not
  concurrent-safe; `*gossh.Client` may be shared
- **Always set explicit `lipgloss.NewStyle().Width(n)` on grid cells** —
  without this, `JoinHorizontal` produces ragged columns
- **Always propagate `WindowSizeMsg` to all sub-models**, including inactive screens
- **Always call `viewport.SetSize(w, h)` on every `WindowSizeMsg`** for every card
- **Re-issue `listenForLog(ch)` after every `LogLineMsg` received** — the
  command reads one message and returns; failing to re-issue silently stops streaming
- **Enforce `maxLines = 2000` ring buffer per card** — no unbounded growth
- **Use `json:",omitempty"` on all new config struct fields**

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
│   │   ├── projects.go
│   │   ├── creator.go
│   │   ├── filelist.go
│   │   └── grid.go
│   ├── components/
│   │   ├── card.go     # ServerCard: viewport, ring buffer, filter, pause, markers
│   │   └── detail.go   # DetailOverlay modal (Phase 2)
│   └── styles/
│       └── styles.go   # all lipgloss styles — edit here, nowhere else
└── parser/
    ├── parser.go        # ParsedLine, Parser interface
    ├── json.go          # JSON detection + pretty-render (Phase 2)
    └── level.go         # level detection + colour map
```

---

## Streaming pattern (critical)

```go
// listenForLog reads ONE message then returns.
// ALWAYS re-issue it from Update after receiving a LogLineMsg.
func listenForLog(ch chan ssh.LogLineMsg) tea.Cmd {
    return func() tea.Msg { return <-ch }
}

// In Update:
case ssh.LogLineMsg:
    m.cards[idx].addLine(...)
    return m, listenForLog(m.logChans[idx])  // <-- must re-issue
```

---

## Screen transition pattern

```go
type SwitchScreenMsg struct {
    To      screen
    Payload any
}
// Send from any sub-model to trigger a screen change in AppModel.
```

---

## Config storage

```go
base, _ := os.UserConfigDir()
// macOS  → ~/Library/Application Support/logviewer/
// Linux  → ~/.config/logviewer/
// Windows → %AppData%\logviewer\
path := filepath.Join(base, "logviewer", "projects.json")
```

---

## Debug logging

```go
// Only active when LOG_DEBUG=1
// Never use fmt.Println — TUI owns stdout
log.SetOutput(file)  // github.com/charmbracelet/log
```

Run with: `LOG_DEBUG=1 ./logviewer` then `tail -f /tmp/logviewer-debug.log`