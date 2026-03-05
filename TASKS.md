# logviewer — Task Breakdown

Each task is designed to be given to Claude Code as a single session. Tasks are
ordered by dependency — complete them in sequence. Each task produces working,
compilable code before the next one begins.

Before starting any task: **read CLAUDE.md**. Before any architectural work:
**read SPEC.md**.

---

## Task 1 — Project scaffold and config package

**Goal:** Compilable project skeleton with working config load/save.

**Deliver:**
- `go.mod` and `go.sum` with all dependencies from SPEC.md Section 16
- `config/config.go`
  - `Host`, `Project`, `Config` structs (SPEC.md Section 1)
  - `configPath()` using `os.UserConfigDir()` — never hardcode `~/.config`
  - `Load()`, `Save()`, `AddProject()`, `DeleteProject()`, `UpdateProject()`
  - Schema version field + migration stub (log and no-op for now)
  - Create config directory with `os.MkdirAll` if missing
- `config/ssh.go`
  - `SSHEntry` struct
  - `ParseSSHConfig()` using `github.com/kevinburke/ssh_config`
  - `ValidateProjectHosts()` with exact alias match + soft hostname fallback
    (SPEC.md Section 2)
- `main.go` stub — just loads config and prints "ok", exits

**Test:** `go build ./...` passes. `go run . ` prints "ok".

---

## Task 2 — SSH client package

**Goal:** Reliable SSH connection with proper auth, host key handling, and
typed errors. No UI yet.

**Deliver:**
- `ssh/client.go`
  - `Client` struct with `host`, `conn *gossh.Client`, `cancel`
  - `Connect()` — auth order: ssh-agent → identity files (SPEC.md Section 3)
  - `ConnectError` + `ConnectFailReason` enum (all five reasons)
  - `knownhosts` verification — never skip, never auto-accept
  - `FailHostKeyUnknown`: return error with a flag so the caller can prompt
  - `FailAuthFailed`: include helpful message if passphrase key detected
- `ssh/tail.go`
  - `LogLine`, `LogLineMsg`, `HostStatusMsg` types
  - `StartTail()` — runs `tail -F -n 0`, feeds `chan LogLineMsg` from goroutine,
    returns `listenForLog(ch)` as initial `tea.Cmd`
  - `listenForLog()` — reads one message from channel
  - `StartGrep()` — runs `grep -n -E`, same channel pattern, for search mode
  - `scheduleReconnect()` — exponential backoff via `tea.Tick` (SPEC.md Section 3)

**Test:** `go build ./...` passes. Write a small `cmd/sshtest/main.go` (not
committed) that connects to a real host and prints 5 tail lines — delete after
verifying.

---

## Task 3 — Parser package (Phase 1 passthrough)

**Goal:** Parser wired in from day one, Phase 1 is a no-op passthrough.

**Deliver:**
- `parser/parser.go`
  - `Level` type + constants (`LevelUnknown` through `LevelFatal`)
  - `ParsedLine` struct (all fields from SPEC.md Section 13)
  - `Parser` interface
  - `DefaultParser` — `Parse()` returns passthrough: `Raw`, `Rendered = raw`,
    `Received` set, everything else zero
- `parser/level.go`
  - `DetectLevel()` stub — returns `LevelUnknown` for now (Phase 2 fills this in)
  - `LevelColors` map
  - `LevelPrefix()` stub — returns `""` for now
- `parser/json.go`
  - `TryParseJSON()` stub — returns `nil, false` for now
  - `PrettyRenderJSON()` stub — returns `raw` for now

**Test:** `go build ./...` passes. Unit test `DefaultParser.Parse` returns
correct passthrough struct.

---

## Task 4 — Styles package and app skeleton

**Goal:** All lipgloss styles defined centrally; root Bubbletea app boots to a
placeholder screen.

**Deliver:**
- `ui/styles/styles.go` — all variables from SPEC.md Section 14. No styles
  defined anywhere else in the codebase.
- `ui/app.go`
  - `AppModel` with `screen` enum, `width`/`height`, all sub-model fields
  - `Init()`, `Update()`, `View()` — `View()` returns `tea.View` (NOT string)
  - `WindowSizeMsg` propagated to all sub-models
  - `SwitchScreenMsg` handled for all screen transitions
  - Currently renders a placeholder `"Loading..."` string
- `main.go` — updated to create `AppModel` and run `tea.NewProgram(m)` with
  NO options (AltScreen goes in `View()`)
- `initLogger()` — file logger, only active when `LOG_DEBUG=1`

**Test:** `go run .` opens a full-screen TUI showing "Loading...", `q` quits.

---

## Task 5 — ProjectList screen

**Goal:** First real screen. Shows projects, supports create/edit/delete
navigation (actions go to placeholders for now).

**Deliver:**
- `ui/screens/projects.go`
  - `ProjectsModel` using `bubbles/v2/list`
  - `projectItem` implementing `list.Item` — title with `⚠` warning badge if
    any host unresolvable (call `ValidateProjectHosts`)
  - Key bindings: `enter` → `SwitchScreenMsg{screenFileList}`, `n` → creator,
    `e` → creator with `editingID`, `d` → confirmation modal (simple inline
    yes/no prompt), `q`/`ctrl+c` → quit
  - Loads projects from config on `Init()`
  - Refreshes list after returning from creator screen

**Test:** `go run .` shows project list. With no projects: shows empty state
message and hint to press `n`. With projects: list renders with descriptions.
`d` asks for confirmation before deleting.

---

## Task 6 — ProjectCreator screen

**Goal:** Three-step wizard to create or edit a project.

**Deliver:**
- `ui/screens/creator.go`
  - `CreatorModel` — steps: name input → host picker → log path input
    (SPEC.md Section 7)
  - Step 1: `textinput` for project name
  - Step 2: `list.Model` populated from `ParseSSHConfig()`, multi-select with
    `[x]`/`[ ]` toggle on `space`, at least one host required
  - Step 3: `textinput` pre-filled `/var/log`, validate absolute path on confirm
  - On save: `config.AddProject()` or `UpdateProject()`, then
    `SwitchScreenMsg{screenProjects}`
  - Edit mode: pre-populates all fields from existing project

**Test:** Full create flow works end-to-end. Created project persists after
restart. Edit flow pre-populates correctly. Empty name or no hosts selected
shows inline error, does not advance.

---

## Task 7 — FileList screen

**Goal:** Connect to all project hosts, list log files, show partial failures
gracefully.

**Deliver:**
- `ui/screens/filelist.go`
  - `FileListModel` — receives `config.Project` via `SwitchScreenMsg.Payload`
  - On `Init()`: validate hosts, then connect concurrently (one goroutine per
    host), run `ls -1 <logPath>`; show `spinner.Model` per host while connecting
  - `fileItem` with `Available map[string]bool` — description renders coloured
    dots per host (SPEC.md Section 8)
  - Failed hosts: show error badge inline, do not block the list
  - Failed `ls`: show warning badge for that host
  - Key bindings: `enter` → `SwitchScreenMsg{screenGrid, payload}`, `r` →
    re-scan, `esc` → back to projects
  - Stores connected `*ssh.Client` slice for hand-off to GridModel

**Test:** Opens against a real (or mocked) project. Spinner appears per host.
Files appear after connect. If one host fails, others still show. `r` rescans.

---

## Task 8 — ServerCard component

**Goal:** The core rendering primitive. Viewport with header, ring buffer,
pause, markers, and filter bar.

**Deliver:**
- `ui/components/card.go`
  - `ServerCard` struct (SPEC.md Section 10)
  - `addLine()` — ring buffer (max 2000), pause buffering, follow mode,
    `rebuildViewport()`
  - `rebuildViewport()` — applies `FilterState`, inserts marker rules at correct
    positions, prepends `15:04:05.000` timestamp to each line
  - Pause / resume — flush `pauseBuf` on resume (SPEC.md Section 10.2)
  - Markers — `[]int` of line indices; render as coloured full-width rule
    (SPEC.md Section 10.3)
  - `FilterState` type with `Matches()` and `Highlight()` (SPEC.md Section 11)
  - `renderHeader()` — status badge, hostname, paused badge, filter badge,
    line count, last-received time; all right-aligned with explicit widths
  - `renderFilterBar()` — empty string when not active
  - `Render(focused bool, w, h int) string` — correct height accounting for
    border + header + optional filter bar

**Test:** Unit tests for `addLine` ring buffer rollover; filter `Matches()`;
marker insertion in `rebuildViewport()`. No TUI needed yet.

---

## Task 9 — ServerGrid screen (tail mode)

**Goal:** The main live view. Grid of cards, streaming tail, focus management.
Search mode and global filter deferred to Task 10.

**Deliver:**
- `ui/screens/grid.go`
  - `GridModel` — receives `config.Project`, `filePath`, connected `[]*ssh.Client`
    via `SwitchScreenMsg.Payload`
  - `gridDimensions()` — ceil(sqrt(n)) column layout (SPEC.md Section 9)
  - One `ServerCard` and one `chan ssh.LogLineMsg` per host
  - `Init()` — call `ssh.StartTail()` for each host, return
    `tea.Batch(listenForLog(ch)...)` for all channels
  - `Update()` — handle `LogLineMsg` (add to correct card, re-issue
    `listenForLog`), `HostStatusMsg` (update card status badge),
    `reconnectMsg` (reconnect + restart tail), `WindowSizeMsg` (recalculate
    all card sizes)
  - `renderGrid()` — `JoinHorizontal` per row, `JoinVertical` for rows + footer
  - Focus management: `tab`/`shift+tab`, scroll keys delegate to focused card's
    viewport
  - Per-card key bindings: `p` pause/resume, `m` marker, `[`/`]` jump markers,
    `y` clipboard copy, `g`/`G` top/bottom, `f` open filter bar
  - `esc` returns to FileList (cancel all contexts, close SSH sessions)
  - `View()` returns `string` (called by `AppModel.View()` which wraps in
    `tea.View`)

**Test:** Grid opens with real SSH hosts tailing a file. Lines appear in cards.
Resize terminal — cards reflow. Disconnect a host — badge updates, reconnect
happens. `p` pauses, lines buffer, resume flushes. `m` drops visible marker.
`y` copies line to clipboard. `esc` returns to FileList cleanly.

---

## Task 10 — Filtering and search mode

**Goal:** Complete the interactive filtering and historical grep search.

**Deliver:**
- **Per-card filter bar** (in `card.go`) — activate with `f`; `textinput`
  appears below header; real-time `rebuildViewport()` on every keystroke;
  `ctrl+r` toggles regex; `ctrl+i` toggles case; `!` prefix = exclude;
  `enter` locks; `esc` clears
- **Global filter** (in `grid.go`) — `F` opens footer-level `textinput`;
  `GridModel.globalFilter` applied before per-card filter in `rebuildViewport()`
- **Log-level quick filter** — `1`–`4` set `GridModel.levelFilter`; passed
  into `rebuildViewport()` (no-op until Phase 2 parser fills `ParsedLine.Level`)
- **Search mode** (`grid.go`) — `s` sets `GridModel.mode = ModeSearch`; shows
  search input bar at top; `enter` runs `ssh.StartGrep()` per host
  concurrently; results stream as `SearchResultMsg`; each card renders a
  `list.Model` of results; `enter` on result → `DetailOverlay.visible = true`
  (overlay renders placeholder for now); `esc` returns to tail mode

**Test:** Filter with plain text — matching lines stay, others hidden, matches
highlighted. Switch to regex — same. `!error` hides error lines. Global filter
applies across all cards simultaneously. `s` → search → results appear per
card. `esc` returns to tail, new lines continue arriving.

---

## Task 11 — Phase 2: log parsing

**Goal:** Full JSON and level detection, coloured rendering.

**Deliver:**
- `parser/level.go` — implement `DetectLevel()`: check JSON fields first
  (`"level"`, `"severity"`, `"lvl"`), then scan raw text for keywords
  (SPEC.md Section 13). Implement `LevelPrefix()` returning fixed-width
  coloured label.
- `parser/json.go` — implement `TryParseJSON()` and `PrettyRenderJSON()` with
  per-type colours (SPEC.md Section 13). Extract common fields:
  `level`, `msg`/`message`, `time`/`timestamp`, `error`/`err`, `caller`,
  `trace_id`, `request_id`.
- `parser/parser.go` — implement full `DefaultParser.Parse()`:
  try JSON → detect level → extract key=value fields → build coloured
  `Rendered` string with `LevelPrefix` prepended.
- Wire level filter in `rebuildViewport()` — `ParsedLine.Level` is now
  populated, so `1`–`4` quick filters now work.

**Test:** Unit tests for: JSON detection, level detection from raw text, level
detection from JSON fields, `LevelPrefix` output widths, `PrettyRenderJSON`
colour output. Live: JSON log lines render with coloured keys and extracted
fields.

---

## Task 12 — Phase 2: DetailOverlay and clipboard

**Goal:** Full detail view for selected log lines, clipboard copy everywhere.

**Deliver:**
- `ui/components/detail.go` — full `DetailOverlay` implementation (SPEC.md
  Section 13):
  - Centred modal with rounded `ColorPrimary` border
  - Header: level badge + timestamp (parsed or received)
  - For JSON: `PrettyRenderJSON` output in scrollable viewport
  - For plain text: key=value fields in two-column layout, raw line at top
  - Keys: `↑`/`k` scroll, `↓`/`j` scroll, `y` copy raw, `Y` copy JSON,
    `esc` close
  - Rendered via `lipgloss.Place` over the grid (SPEC.md Section 9)
- Clipboard in grid: `y` on focused card copies the currently visible top line
  or selected search result using `tea.SetClipboard()`
- Clipboard in detail overlay: `y` raw line, `Y` full pretty-printed JSON
- Wire `enter` in grid tail mode to open overlay for the line at viewport top
- Wire `enter` in search mode to open overlay for the selected search result

**Test:** `enter` on a JSON log line opens overlay with pretty-printed fields.
`enter` on a plain log line shows raw + extracted fields. `y` copies raw to
clipboard (verify paste works in terminal). `Y` copies JSON. `esc` closes and
returns to grid. Overlay scrolls on long JSON objects.

---

## Task 13 — Polish, edge cases, and known_hosts prompt

**Goal:** Handle the remaining error paths and UX rough edges.

**Deliver:**
- **Unknown host key modal** — when `Connect()` returns `FailHostKeyUnknown`,
  show a modal overlay in the FileList or Grid screen:
  `"Add host key for <hostname> to known_hosts? [y/N]"`. On `y`: append key to
  `~/.ssh/known_hosts` and retry connection. On `n`: mark host as failed.
- **Host key changed warning** — when `FailHostKeyChanged`, show a red
  warning card in the grid with the exact error message and the known_hosts
  file path and line number. No retry.
- **Auth failure UX** — when `FailAuthFailed`, if the error message indicates a
  passphrase-protected key, show: _"Key requires a passphrase. Run: ssh-add
  ~/.ssh/your_key"_ in the error card.
- **Empty project list** — show a helpful empty state: `"No projects yet.
  Press n to create one."` with a subtle style.
- **Terminal too small** — if `termW < 40` or `termH < 10`, show a single
  centred message: `"Terminal too small — please resize."` instead of the grid.
- **`esc` from FileList** — ensure all in-progress SSH connections are
  cancelled cleanly (context cancel) before switching back to ProjectList.
- **Graceful quit** — on `q`/`ctrl+c`, cancel all contexts, close all SSH
  sessions, then quit. Avoid leaving dangling goroutines.
- **Help bar content** — ensure the footer help bar shows contextually correct
  key hints for the current mode (tail / search / filter active / paused).

**Test:** Unknown host key → prompt appears → accept → connects. Changed key →
red card with message. Auth failure with passphrase key → shows `ssh-add` hint.
Terminal resize to very small → shows resize message, not a broken grid.
Quit → no goroutine leaks (use `go test -race` or manual inspection of
goroutine count).

---

## Dependency graph

```
Task 1  (config)
  └── Task 2  (ssh)
        └── Task 3  (parser) ──┐
              └── Task 4  (styles + app skeleton)
                    └── Task 5  (ProjectList)
                          └── Task 6  (ProjectCreator)
                                └── Task 7  (FileList)
                                      └── Task 8  (ServerCard)
                                            └── Task 9  (Grid - tail)
                                                  └── Task 10 (Filter + search)
                                                        └── Task 11 (Parser Phase 2)
                                                              └── Task 12 (Detail + clipboard)
                                                                    └── Task 13 (Polish)
```

Tasks 1–3 can be done in parallel if desired. All others are strictly sequential.