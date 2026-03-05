# logviewer — Architecture Specification

Reference document. Read this before implementing any screen, component, or
package. See `CLAUDE.md` for always-loaded rules and API reminders.

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
    Version  int       `json:"version"`  // schema version for migration
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
    ResolvedAs string // may differ from Host.Name if soft-matched
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
    FailAuthFailed     // no valid key; if passphrase-protected: "start ssh-agent, run ssh-add"
    FailHostKeyChanged // show exact error + known_hosts file/line; never auto-fix
    FailHostKeyUnknown // prompt user: "Add host key to known_hosts? [y/N]"
    FailTimeout
)
```

### Reconnection (exponential backoff)

```
attempt 1 → 2s, attempt 2 → 4s, attempt 3 → 8s, attempt 4+ → 30s (cap)
max 10 attempts → StatusError, stop retrying
```

```go
func scheduleReconnect(host config.Host, attempt int) tea.Cmd {
    return tea.Tick(reconnectDelay(attempt), func(time.Time) tea.Msg {
        return reconnectMsg{host, attempt}
    })
}
```

Cancel all sessions via context when leaving the grid screen.

---

## 4. Tail Streaming

### Always `tail -F` (capital F)

Follows by filename, handles log rotation. `-n 0` streams only new lines.

```go
const tailCmd = "tail -F -n 0 %s"
const grepCmd = "grep -n -E %q %s"  // search mode
```

### Message types

```go
type LogLine struct {
    Host     string
    Received time.Time
    Raw      string
    LineNum  int       // search mode only
}
type LogLineMsg   LogLine
type HostStatusMsg struct {
    Host   string
    Status HostStatus
    Err    error
}
```

### StartTail

Opens a new `*gossh.Session` (not shared), runs `tail -F`, feeds lines into
`ch chan LogLineMsg`. Returns `listenForLog(ch)` as initial `tea.Cmd`.

```go
func listenForLog(ch chan LogLineMsg) tea.Cmd {
    return func() tea.Msg { return <-ch }
}
// Re-issue from Update after every LogLineMsg. See CLAUDE.md.
```

### Receive-time timestamps

Prepend to every viewport line:
```
15:04:05.000  <original log content>
```

---

## 5. Application Screens

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
`WindowSizeMsg` to **all** sub-models on every resize.

```go
func (m AppModel) View() tea.View {
    // render active sub-model as string, set in tea.View
    v := tea.NewView(content)
    v.AltScreen = true
    return v
}
```

---

## 6. ProjectList Screen

Uses `bubbles/v2/list`. Item type:

```go
type projectItem struct {
    proj       config.Project
    hasWarning bool  // true if any host unresolvable
}
func (p projectItem) Title() string       // name + optional " ⚠"
func (p projectItem) Description() string // "N hosts · /path"
func (p projectItem) FilterValue() string
```

Key bindings: `enter` open, `n` new, `e` edit, `d` delete (confirm modal),
`q`/`ctrl+c` quit.

---

## 7. ProjectCreator Screen

Three-step wizard: name → host selection → log path.

```go
type CreatorModel struct {
    step      creatorStep     // stepName | stepHostPick | stepPathInput
    nameInput textinput.Model
    hostList  list.Model      // SSH entries, multi-select with [x]/[ ] checkboxes
    selected  map[int]bool
    pathInput textinput.Model // pre-filled "/var/log"
    editingID string          // non-empty when editing
}
```

Validate path is absolute on confirm. Save project, `SwitchScreenMsg` to
ProjectList.

---

## 8. FileList Screen

On project open:

1. Validate hosts (Section 2)
2. Connect concurrently — spinner per host
3. Run `ls -1 <logPath>` per host
4. Merge into unified sorted list

Partial failure is fine — failed hosts show error badge, working hosts proceed.

```go
type fileItem struct {
    Name      string
    Available map[string]bool // host alias → present
}
// Description renders: "● server1  ● server2  ○ server3"
```

Key bindings: `enter` tail → Grid, `r` rescan, `esc` back.

---

## 9. ServerGrid Screen

### Layout

```go
func gridDimensions(n, termW, termH int) (cols, rows, cardW, cardH int) {
    cols  = int(math.Ceil(math.Sqrt(float64(n))))
    rows  = int(math.Ceil(float64(n) / float64(cols)))
    cardW = termW / cols
    cardH = (termH - 1) / rows  // 1 line reserved for help bar
    return
}
// 1→1×1  2→2×1  3-4→2×2  5-6→3×2  7-9→3×3
```

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
}
```

### Rendering

```go
func (m GridModel) renderGrid() string {
    // build cols×rows grid using lipgloss.JoinHorizontal + JoinVertical
    // empty cells: lipgloss.NewStyle().Width(cardW).Height(cardH).Render("")
    // append footer help bar with JoinVertical
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

### Key bindings

| Key           | Scope  | Action                                      |
|---------------|--------|---------------------------------------------|
| `tab`         | Grid   | Focus next card                             |
| `shift+tab`   | Grid   | Focus previous card                         |
| `↑`/`k`       | Card   | Scroll up                                   |
| `↓`/`j`       | Card   | Scroll down                                 |
| `g`           | Card   | Jump to top                                 |
| `G`           | Card   | Jump to bottom / follow mode                |
| `f`           | Card   | Per-card filter bar                         |
| `F`           | Grid   | Global filter bar                           |
| `ctrl+r`      | Filter | Toggle regex mode                           |
| `ctrl+i`      | Filter | Toggle case sensitivity                     |
| `1`–`4`       | Grid   | Minimum log level (DEBUG / INFO / WARN / ERROR) |
| `s`           | Grid   | Enter search mode                           |
| `p`           | Card   | Toggle pause / resume                       |
| `m`           | Card   | Drop marker at current position             |
| `[` / `]`     | Card   | Jump to prev / next marker                  |
| `enter`       | Card   | Open detail overlay (Phase 2)               |
| `y`           | Card   | Copy line to clipboard                      |
| `esc`         | Ctx    | Close filter / exit search / back to FileList |
| `q`/`ctrl+c`  | App    | Quit                                        |

---

## 10. ServerCard Component

```go
const maxLines = 2000

type ServerCard struct {
    host      config.Host
    viewport  viewport.Model
    status    ssh.HostStatus
    filter    FilterState
    lines     []parser.ParsedLine // ring buffer
    paused    bool
    pauseBuf  []parser.ParsedLine
    following bool
    markers   []int               // indices into lines
}
```

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

Stored as line indices. On `rebuildViewport`, insert a full-width rule at
each marker index:
```
──────────────── ◆ 15:04:32 ────────────────
```

### Render

```go
func (c *ServerCard) Render(focused bool, w, h int) string {
    header    := c.renderHeader(focused, w)   // 1 line
    filterBar := c.renderFilterBar(w)          // 1 line or ""
    // viewport height = h - lipgloss border (2) - header (1) - filterBar (0 or 1)
    body      := c.viewport.View()
    return lipgloss.JoinVertical(lipgloss.Left, header, filterBar, body)
}
```

Header format:
```
● server-prod-1    [PAUSED]  [FILTERED: error]    42 lines  15:04:05
```

---

## 11. Filtering

### FilterState

```go
type FilterMode int
const ( FilterNone FilterMode = iota; FilterText; FilterRegex )

type FilterState struct {
    Mode       FilterMode
    Pattern    string
    Compiled   *regexp.Regexp
    Exclude    bool       // "!" prefix → hide matching lines
    IgnoreCase bool       // default true
}

func (f FilterState) Matches(line string) bool
func (f FilterState) Highlight(line string, matchStyle lipgloss.Style) string
```

### Scoping

- `f` → per-card filter (card's own `FilterState`)
- `F` → global filter (`GridModel.globalFilter`) — all cards must pass both

### UX flow

1. `f`/`F` → filter bar appears, cursor active
2. Type pattern → real-time update, matched substrings highlighted
3. `ctrl+r` → toggle regex; `ctrl+i` → toggle case
4. `!pattern` → negative filter
5. `enter` → lock filter (bar dims); `esc` → clear + close

### Level quick filters

`1`=DEBUG+, `2`=INFO+, `3`=WARN+, `4`=ERROR+. No-op in Phase 1.

---

## 12. Search Mode

```go
type SearchState struct {
    Query   string
    Regex   bool
    Results map[string][]SearchResult // host → results
    Loading map[string]bool
}
type SearchResult struct {
    LineNum int
    Raw     string
    Parsed  parser.ParsedLine
}
```

Activated by `s`. Runs `grep -n -E <query> <filePath>` per host concurrently.
Results stream in as `SearchResultMsg`. Each card shows results in a
`list.Model`. `enter` on a result opens `DetailOverlay`. `esc` returns to tail.

---

## 13. Phase 2 — Log Parsing

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

Phase 1: `DefaultParser.Parse` returns passthrough (only `Raw`, `Rendered=raw`,
`Received` set). Parser is wired in from day one.

### Level detection

Check JSON fields `"level"`, `"severity"`, `"lvl"` first, then scan raw text:
`FATAL`/`CRITICAL` → Fatal, `ERROR` → Error, `WARN`/`WARNING` → Warn,
`INFO` → Info, `DEBUG`/`TRACE` → Debug.

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
null → dim. Width-aware wrapping. Common fields surfaced in detail view:
`level`, `msg`/`message`, `time`/`timestamp`, `error`/`err`, `caller`,
`trace_id`, `request_id`.

### DetailOverlay

```go
type DetailOverlay struct {
    visible  bool
    line     parser.ParsedLine
    host     string
    viewport viewport.Model
}

func (d *DetailOverlay) Render(w, h int) string {
    width  := min(w-8, 100)
    height := min(h-6, 40)
    // rounded border, ColorPrimary, scrollable viewport inside
}
```

Keys: `↑`/`k` scroll, `↓`/`j` scroll, `y` copy raw, `Y` copy JSON, `esc` close.

---

## 14. Styles Reference (ui/styles/styles.go)

All lipgloss styles live here. Never define styles inline elsewhere.

| Variable           | Use                                   |
|--------------------|---------------------------------------|
| `ColorPrimary`     | `#7B61FF` — focused borders, keys     |
| `ColorSuccess`     | `#2ECC71` — connected badge           |
| `ColorWarning`     | `#F39C12` — connecting, paused        |
| `ColorError`       | `#E74C3C` — disconnected, errors      |
| `ColorMuted`       | `#666666` — unfocused borders, help   |
| `ColorHighlight`   | `#F1C40F` — filter match background   |
| `ColorMarker`      | `#3498DB` — marker rule               |
| `CardBorder`       | Rounded border, `ColorMuted`          |
| `CardBorderFocused`| Rounded border, `ColorPrimary`        |
| `MatchHighlight`   | `ColorHighlight` bg, black fg         |
| `MarkerLine`       | `ColorMarker`, bold                   |
| `HelpBar`          | `ColorMuted`                          |
| `BadgeConnected`   | `●` green                             |
| `BadgeConnecting`  | `◌` yellow                            |
| `BadgeDisconnected`| `✗` red                               |
| `BadgePaused`      | `[PAUSED]` yellow bold                |

---

## 15. Error Handling Summary

| Scenario                     | Behaviour                                                      |
|------------------------------|----------------------------------------------------------------|
| Host unreachable at FileList | Error badge on that host; others proceed normally              |
| `ls` fails on log path       | Warning on that host; others show files normally               |
| Auth failure                 | `FailAuthFailed` error; if passphrase key: explain `ssh-add`   |
| Host key changed             | `FailHostKeyChanged`; show error + known_hosts location        |
| Host key unknown             | Modal prompt "Add to known_hosts? [y/N]"                      |
| SSH alias renamed            | Soft hostname match; warn if matched via fallback              |
| Disconnection during tail    | Badge update; exponential backoff reconnect (max 10 attempts)  |
| Unresolvable host            | Skip; warn; continue with resolved hosts                       |

---

## 16. go.mod

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