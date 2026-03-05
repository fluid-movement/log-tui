# logviewer — Architecture Specification

Reference document. Read this before implementing any screen, component, or
package. See `CLAUDE.md` for always-loaded rules, API reminders, and the
mandatory key/help component pattern.

---

## 1. Data Models

### config.Host

```go
type Host struct {
    Name     string `json:"name"`     // alias from ~/.ssh/config
    Hostname string `json:"hostname"` // resolved IP/FQDN — used for soft match
    User     string `json:"user"`
    Port     int    `json:"port"`
}
```

### config.Project

```go
type Project struct {
    ID        string    `json:"id"`
    Name      string    `json:"name"`
    Hosts     []Host    `json:"hosts"`
    LogPath   string    `json:"log_path"`
    ConfigVer int       `json:"config_ver"`
    CreatedAt time.Time `json:"created_at"`
}
```

### config.Config

```go
type Config struct {
    Version  int       `json:"version"`
    Projects []Project `json:"projects"`
}
```

On load: if `Config.Version` < current schema version, run migration and
re-save. Use `json:",omitempty"` on all optional fields.

---

## 2. SSH Config Parsing and Host Validation

### ParseSSHConfig

Reads `~/.ssh/config` via `github.com/kevinburke/ssh_config`. Returns all
`Host` entries. Excludes wildcard entries (`Host *`).

```go
type SSHEntry struct {
    Alias    string
    Hostname string
    User     string
    Port     int
}

func ParseSSHConfig() ([]SSHEntry, error)
```

### Host validation on project open

Run before connecting. Strategy:

1. **Exact alias match** — `SSHEntry.Alias == Host.Name` → use it
2. **Soft hostname match** — if alias not found, find entry where
   `SSHEntry.Hostname == Host.Hostname` → use it (handles renames transparently)
3. **Unresolvable** — mark `StatusUnresolvable`, skip, continue with other hosts

```go
type HostValidationResult struct {
    Host       config.Host
    Resolved   bool
    ResolvedAs string
    Warning    string
}

func ValidateProjectHosts(proj config.Project) []HostValidationResult
```

ProjectList shows `⚠` badge on any project with at least one unresolvable host.

---

## 3. SSH Client

```go
type Client struct {
    host   config.Host
    conn   *gossh.Client
    cancel context.CancelFunc
}

func Connect(ctx context.Context, host config.Host) (*Client, error)
```

### Auth order

1. `ssh-agent` via `SSH_AUTH_SOCK`
2. Identity files from `~/.ssh/config` for the host

### Host key verification

Use `golang.org/x/crypto/ssh/knownhosts`. Never skip.

### Typed errors

```go
type ConnectError struct {
    Host   config.Host
    Reason ConnectFailReason
    Err    error
}

type ConnectFailReason int
const (
    FailUnreachable    ConnectFailReason = iota
    FailAuthFailed     // if passphrase key: "start ssh-agent, run ssh-add"
    FailHostKeyChanged // show error + known_hosts file/line; never auto-fix
    FailHostKeyUnknown // prompt: "Add host key to known_hosts? [y/N]"
    FailTimeout
)
```

### Reconnection (exponential backoff)

```
attempt 1 → 2s, attempt 2 → 4s, attempt 3 → 8s, attempt 4+ → 30s (cap)
max 10 attempts → StatusError, stop
```

```go
func scheduleReconnect(host config.Host, attempt int) tea.Cmd {
    return tea.Tick(reconnectDelay(attempt), func(time.Time) tea.Msg {
        return reconnectMsg{host, attempt}
    })
}
```

---

## 4. Tail Streaming

### Always `tail -F` (capital F)

```go
const tailCmd = "tail -F -n 0 %s"
const grepCmd = "grep -n -E %q %s"
```

### Message types

```go
type LogLine struct {
    Host     string
    Received time.Time
    Raw      string
    LineNum  int
}
type LogLineMsg   LogLine
type HostStatusMsg struct {
    Host   string
    Status HostStatus
    Err    error
}
```

### Streaming pattern

```go
func listenForLog(ch chan LogLineMsg) tea.Cmd {
    return func() tea.Msg { return <-ch }
}
// Re-issue from Update after every LogLineMsg. See CLAUDE.md.
```

### Receive-time timestamps

Prepend to every viewport line: `15:04:05.000  <log content>`

---

## 5. Key and Help Components

Every screen and every component with keybindings uses `bubbles/v2/key` and
`bubbles/v2/help`. This is mandatory — see CLAUDE.md for the full pattern.

### Imports

```go
"charm.land/bubbles/v2/help"
"charm.land/bubbles/v2/key"
```

### Per-screen KeyMap pattern

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

### Width

Set `help.Model.Width = m.width` on every `WindowSizeMsg` so the help bar
truncates gracefully rather than wrapping.

---

## 6. Complete KeyMap Definitions

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

---

## 7. Application Screens

### Screen enum

```go
type screen int
const (
    screenProjects screen = iota
    screenCreator
    screenFileList
    screenGrid
)
```

### SwitchScreenMsg

```go
type SwitchScreenMsg struct {
    To      screen
    Payload any
}
```

### AppModel (ui/app.go)

Owns current screen and all sub-models. Routes messages. Propagates
`WindowSizeMsg` to **all** sub-models on every resize, including `help.Width`.

```go
func (m AppModel) View() tea.View {
    v := tea.NewView(content)
    v.AltScreen = true
    return v
}
```

---

## 8. ProjectList Screen

Uses `bubbles/v2/list`. Item type:

```go
type projectItem struct {
    proj       config.Project
    hasWarning bool
}
func (p projectItem) Title() string       // name + optional " ⚠"
func (p projectItem) Description() string // "N hosts · /path"
func (p projectItem) FilterValue() string
```

Embeds `keys projectsKeyMap` and `help help.Model`. Footer rendered by
`help.View(m.keys)`. Key matching via `key.Matches`.

---

## 9. ProjectCreator Screen

Three-step wizard: name → host selection → log path.

```go
type CreatorModel struct {
    step      creatorStep
    nameInput textinput.Model
    hostList  list.Model
    selected  map[int]bool
    pathInput textinput.Model
    editingID string
    keys      creatorKeyMap
    help      help.Model
}
```

---

## 10. FileList Screen

On project open: validate hosts → connect concurrently with spinner → `ls -1`
per host → merge sorted list. Partial failure is fine.

```go
type fileItem struct {
    Name      string
    Available map[string]bool
}
```

Embeds `keys filelistKeyMap` and `help help.Model`.

---

## 11. ServerGrid Screen

### Layout

```go
func gridDimensions(n, termW, termH int) (cols, rows, cardW, cardH int) {
    cols  = int(math.Ceil(math.Sqrt(float64(n))))
    rows  = int(math.Ceil(float64(n) / float64(cols)))
    cardW = termW / cols
    cardH = (termH - helpBarHeight) / rows
    return
}
```

`helpBarHeight` = 1 line when `help.ShowAll` is false, more when true.
Recalculate on every `WindowSizeMsg` and whenever `help.ShowAll` toggles.

### GridModel

```go
type GridMode int
const ( ModeTail GridMode = iota; ModeSearch )

type GridModel struct {
    project      config.Project
    filePath     string
    cards        []ServerCard
    focusedIdx   int
    mode         GridMode
    globalFilter FilterState
    detail       DetailOverlay
    width, height int
    clients      []*ssh.Client
    logChans     []chan ssh.LogLineMsg
    keys         gridKeyMap
    help         help.Model
}
```

### Rendering

```go
func (m GridModel) renderGrid() string {
    // JoinHorizontal per row, JoinVertical rows + help footer
    // help footer: m.help.View(m.keys)
}

func (m GridModel) View() string {
    grid := m.renderGrid()
    if m.detail.visible {
        return lipgloss.Place(m.width, m.height,
            lipgloss.Center, lipgloss.Center,
            m.detail.Render(m.width, m.height))
    }
    return grid
}
```

---

## 12. ServerCard Component

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

### addLine

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

### Pause / resume

On resume: flush `pauseBuf` → `lines` (honour `maxLines`), clear `pauseBuf`,
rebuild viewport.

### Markers

Stored as line indices. On `rebuildViewport`, insert at each marker index:
```
──────────────── ◆ 15:04:32 ────────────────
```

### Render

```go
func (c *ServerCard) Render(focused bool, w, h int) string
// header (1 line) + filter bar (0 or 1 line) + viewport
```

Header: status badge · hostname · `[PAUSED]` · `[FILTERED: x]` · line count ·
last-received time.

---

## 13. Filtering

### FilterState

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

### Scoping

- `f` → per-card (`FilterState` on the card)
- `F` → global (`GridModel.globalFilter`); both filters must pass

### UX

1. `f`/`F` → filter bar with `textinput` appears
2. Real-time rebuild on each keystroke
3. `ctrl+r` toggle regex; `ctrl+i` toggle case
4. `!pattern` = exclude
5. `enter` lock; `esc` clear

### Level quick filters

`1`–`4` set `GridModel.levelFilter`. No-op until Phase 2 fills `ParsedLine.Level`.

---

## 14. Search Mode

`s` → `GridModel.mode = ModeSearch`. Runs `grep -n -E` per host via separate
SSH sessions. Results stream as `SearchResultMsg`. Each card shows a
`list.Model` of results. `enter` opens `DetailOverlay`. `esc` returns to tail.

---

## 15. Phase 2 — Log Parsing

### ParsedLine

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

### Level detection

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

### JSON pretty-render

Keys → `ColorPrimary`, strings → green, numbers → cyan, booleans → yellow,
null → dim. Common fields surfaced in detail view: `level`, `msg`/`message`,
`time`/`timestamp`, `error`/`err`, `caller`, `trace_id`, `request_id`.

### DetailOverlay

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

---

## 16. Styles Reference (ui/styles/styles.go)

All lipgloss styles live here. Never define styles inline elsewhere.

| Variable           | Value      | Use                               |
|--------------------|------------|-----------------------------------|
| `ColorPrimary`     | `#7B61FF`  | focused borders, key labels       |
| `ColorSuccess`     | `#2ECC71`  | connected badge                   |
| `ColorWarning`     | `#F39C12`  | connecting, paused                |
| `ColorError`       | `#E74C3C`  | disconnected, errors              |
| `ColorMuted`       | `#666666`  | unfocused borders                 |
| `ColorHighlight`   | `#F1C40F`  | filter match background           |
| `ColorMarker`      | `#3498DB`  | marker rule                       |
| `CardBorder`       | —          | Rounded border, `ColorMuted`      |
| `CardBorderFocused`| —          | Rounded border, `ColorPrimary`    |
| `MatchHighlight`   | —          | `ColorHighlight` bg, black fg     |
| `MarkerLine`       | —          | `ColorMarker`, bold               |
| `BadgeConnected`   | `●`        | green                             |
| `BadgeConnecting`  | `◌`        | yellow                            |
| `BadgeDisconnected`| `✗`        | red                               |
| `BadgePaused`      | `[PAUSED]` | yellow bold                       |

The `help.Model` uses its own default styles which fit naturally. Override
`help.Styles` if needed to match `ColorPrimary`/`ColorMuted`.

---

## 17. Error Handling Summary

| Scenario                     | Behaviour                                                      |
|------------------------------|----------------------------------------------------------------|
| Host unreachable at FileList | Error badge on that host; others proceed                       |
| `ls` fails on log path       | Warning on that host; others show files                        |
| Auth failure                 | `FailAuthFailed`; if passphrase key: explain `ssh-add`         |
| Host key changed             | `FailHostKeyChanged`; show error + known_hosts location        |
| Host key unknown             | Modal prompt "Add to known_hosts? [y/N]"                      |
| SSH alias renamed            | Soft hostname match; warn if matched via fallback              |
| Disconnection during tail    | Badge update; exponential backoff reconnect (max 10 attempts)  |
| Unresolvable host            | Skip; warn; continue with resolved hosts                       |

---

## 18. go.mod

```go
module github.com/yourname/logviewer

go 1.23

require (
    charm.land/bubbletea/v2          v2.0.0
    charm.land/bubbles/v2            v2.0.0
    charm.land/lipgloss/v2           v2.0.0
    github.com/charmbracelet/log     v0.4.0
    github.com/kevinburke/ssh_config v1.7.0
    golang.org/x/crypto              v0.31.0
)
```