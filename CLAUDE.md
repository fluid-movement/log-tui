# logviewer — Claude Code Instructions

Read `SPEC.md` before any architectural decisions or implementing any screen/component/package.

---

## Charm v2 API (breaking changes from v1)

**Import paths — `charm.land`, NOT `github.com`:**
```go
tea "charm.land/bubbletea/v2"
"charm.land/bubbles/v2/help"
"charm.land/bubbles/v2/key"
"charm.land/lipgloss/v2"
```

**`View()` returns `tea.View`, NOT `string`. AltScreen/mouse go in `View()`, NOT `NewProgram()`:**
```go
func (m Model) View() tea.View {
    v := tea.NewView(content)
    v.AltScreen = true
    return v
}
```

**Clipboard (OSC52):** `tea.SetClipboard(s)` / `tea.ReadClipboard()` / `case tea.ClipboardMsg`

---

## Rules

- Every screen MUST use `bubbles/v2/key` + `bubbles/v2/help` — never render help manually, never use `msg.String()` for key matching
- Key bindings: QWERTZ-safe — use `b`/`w` for prev/next marker (NOT `[`/`]`)
- **Never hardcode `~/.config`** — use `os.UserConfigDir()`
- **Always `tail -F`** (capital F) — handles log rotation
- **Never write to stdout** — TUI owns it; use file logger
- **Never auto-accept unknown SSH host keys** — always prompt
- **Never share `*gossh.Session` between goroutines**
- **Always set explicit `lipgloss.NewStyle().Width(n)` on grid cells**
- **Always propagate `WindowSizeMsg` to all sub-models** including inactive screens
- **Re-issue `listenForLog(ch)` after every `LogLineMsg` received:**
  ```go
  case ssh.LogLineMsg:
      m.cards[idx].addLine(...)
      return m, listenForLog(m.logChans[idx])
  ```
- **`maxLines = 2000` ring buffer per card**
- **`json:",omitempty"` on all new config struct fields**
- **All lipgloss styles in `ui/styles/styles.go`** — nowhere else
