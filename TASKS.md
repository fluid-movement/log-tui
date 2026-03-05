# logviewer — Task Breakdown

Each task is a single Claude Code session. Complete them in order — each one
leaves the codebase in a compilable state. Before starting any task, read
`CLAUDE.md`. Before any architectural work, read `SPEC.md`.

---

## Task 1 — Project scaffold and config package

**Goal:** Compilable project skeleton with working config load/save.

**Deliver:**
- `go.mod` and `go.sum` with all dependencies from SPEC.md Section 18
- `config/config.go`
  - `Host`, `Project`, `Config` structs (SPEC.md Section 1)
  - `configPath()` using `os.UserConfigDir()` — never hardcode `~/.config`
  - `Load()`, `Save()`, `AddProject()`, `DeleteProject()`, `UpdateProject()`
  - Schema version + migration stub (log and no-op for now)
  - `os.MkdirAll` to create config dir if missing
- `config/ssh.go`
  - `SSHEntry` struct
  - `ParseSSHConfig()` via `github.com/kevinburke/ssh_config`
  - `ValidateProjectHosts()` — exact alias match + soft hostname fallback
    (SPEC.md Section 2)
- `main.go` stub — loads config, prints "ok", exits

**Test:** `go build ./...` passes. `go run .` prints "ok".

---

## Task 2 — SSH client package

**Goal:** Reliable SSH connections with proper auth, host key handling, and
typed errors. No UI yet.

**Deliver:**
- `ssh/client.go`
  - `Client` struct
  - `Connect()` — auth: ssh-agent → identity files (SPEC.md Section 3)
  - `ConnectError` + `ConnectFailReason` enum (all five reasons)
  - `knownhosts` verification — never skip
  - `FailHostKeyUnknown`: return typed error with flag so caller can prompt
  - `FailAuthFailed`: include `ssh-add` hint if passphrase key detected
- `ssh/tail.go`
  - `LogLine`, `LogLineMsg`, `HostStatusMsg` types
  - `StartTail()` — `tail -F -n 0`, goroutine → `chan LogLineMsg`,
    returns `listenForLog(ch)`
  - `listenForLog()` — reads one message from channel (re-issue after each)
  - `StartGrep()` — `grep -n -E`, same pattern, for search mode
  - `scheduleReconnect()` — exponential backoff via `tea.Tick` (SPEC.md Section 3)

**Test:** `go build ./...` passes.

---

## Task 3 — Parser package (Phase 1 passthrough)

**Goal:** Parser wired in from day one; Phase 1 is a no-op passthrough.

**Deliver:**
- `parser/parser.go`
  - `Level` type + constants
  - `ParsedLine` struct (SPEC.md Section 15)
  - `Parser` interface
  - `DefaultParser.Parse()` — passthrough: `Raw`, `Rendered = raw`, `Received`
- `parser/level.go`
  - `DetectLevel()` stub → `LevelUnknown`
  - `LevelColors` map
  - `LevelPrefix()` stub → `""`
- `parser/json.go`
  - `TryParseJSON()` stub → `nil, false`
  - `PrettyRenderJSON()` stub → `raw`

**Test:** `go build ./...` passes. Unit test: `DefaultParser.Parse` returns
correct passthrough.

---

## Task 4 — Styles package and app skeleton

**Goal:** All styles defined; root Bubbletea app boots and responds to input.

**Deliver:**
- `ui/styles/styles.go` — all variables from SPEC.md Section 16. No styles
  defined anywhere else.
- `ui/app.go`
  - `AppModel` with screen enum, all sub-model fields
  - `View()` returns `tea.View` with `v.AltScreen = true` — NOT a string
  - `WindowSizeMsg` propagated to all sub-models (including `help.Width`)
  - `SwitchScreenMsg` handled for all transitions
  - Renders placeholder `"Loading..."` for now
- `main.go` — `tea.NewProgram(m)` with NO options
- `initLogger()` — file logger, only when `LOG_DEBUG=1`

**Test:** `go run .` opens full-screen TUI, `q` quits cleanly.

---

## Task 5 — ProjectList screen

**Goal:** First real screen with working key/help components.

**Deliver:**
- `ui/screens/projects.go`
  - `projectsKeyMap` satisfying `help.KeyMap` (SPEC.md Section 6)
  - `ProjectsModel` with embedded `keys projectsKeyMap` and `help help.Model`
  - `projectItem` implementing `list.Item` with `⚠` warning badge
  - All key handling via `key.Matches` — no `msg.String()` switches
  - `help.View(m.keys)` renders the footer — no manual help text
  - `?` toggles `m.help.ShowAll`
  - `help.Width` set on every `WindowSizeMsg`
  - Actions: `enter` → FileList, `n` → Creator, `e` → Creator (edit),
    `d` → inline confirm, `q` quit

**Test:** Project list renders. Footer shows short help. `?` expands to full
help. No manual string rendering of key hints anywhere.

---

## Task 6 — ProjectCreator screen

**Goal:** Three-step wizard with key/help components.

**Deliver:**
- `ui/screens/creator.go`
  - `creatorKeyMap` with `help.Model`
  - Steps: name → host multi-select → log path
  - Host list: `[x]`/`[ ]` checkboxes, `space` to toggle (via `key.Matches`)
  - Path pre-filled `/var/log`, validated as absolute on confirm
  - Save via `config.AddProject()` or `UpdateProject()`
  - Edit mode pre-populates all fields

**Test:** Full create/edit flow end-to-end. Persists after restart. Invalid
input (empty name, no hosts, relative path) shows inline error and does not
advance.

---

## Task 7 — FileList screen

**Goal:** Connect to hosts, aggregate files, handle partial failures.

**Deliver:**
- `ui/screens/filelist.go`
  - `filelistKeyMap` with `help.Model`
  - Receives `config.Project` via `SwitchScreenMsg.Payload`
  - On `Init()`: validate hosts → connect concurrently → `ls -1 <logPath>`
  - `spinner.Model` per host while connecting
  - `fileItem` with coloured availability dots in description
  - Failed hosts: error badge inline, do not block
  - Failed `ls`: warning badge for that host
  - Stores connected `[]*ssh.Client` for hand-off to GridModel
  - Key handling via `key.Matches`

**Test:** Opens against a project. Spinners appear. Files list after connect.
One failed host shows error badge; others work. `r` rescans.

---

## Task 8 — ServerCard component

**Goal:** Core rendering primitive with ring buffer, pause, markers, filter.

**Deliver:**
- `ui/components/card.go`
  - `ServerCard` struct (SPEC.md Section 12)
  - `addLine()` — ring buffer (max 2000), pause buffering, follow mode
  - `rebuildViewport()` — applies `FilterState`, inserts marker rules,
    prepends `15:04:05.000` timestamp per line
  - Pause/resume — flush `pauseBuf` on resume
  - Markers as line indices, rendered as coloured full-width rule
  - `FilterState` with `Matches()` and `Highlight()` (SPEC.md Section 13)
  - `renderHeader()` — status badge, hostname, badges, line count, time;
    all right-aligned with explicit widths
  - `renderFilterBar()` — empty string when inactive; uses `textinput.Model`
  - `Render(focused bool, w, h int) string`

Note: the card does NOT own a `help.Model`. Key handling is in `GridModel`.

**Test:** Unit tests for: `addLine` ring buffer rollover; `Matches()` with
plain/regex/exclude; marker insertion in `rebuildViewport()`.

---

## Task 9 — ServerGrid screen (tail mode)

**Goal:** Main live view with grid layout, streaming, and full key/help setup.

**Deliver:**
- `ui/screens/grid.go`
  - `gridKeyMap` satisfying `help.KeyMap` — full definition from SPEC.md Section 6
  - `GridModel` with `keys gridKeyMap` and `help help.Model`
  - `gridDimensions()` — ceil(sqrt(n)) layout (SPEC.md Section 11)
  - `Init()` — `StartTail()` per host, `tea.Batch(listenForLog...)` for all
  - `Update()`:
    - `LogLineMsg` → add to card, re-issue `listenForLog`
    - `HostStatusMsg` → update card status badge
    - `reconnectMsg` → reconnect + restart tail
    - `WindowSizeMsg` → recalculate card sizes, update `help.Width`
    - All key events via `key.Matches` against `m.keys`
    - `?` toggles `m.help.ShowAll`; recalculate `helpBarHeight` and card sizes
  - `renderGrid()` — `JoinHorizontal` rows, `JoinVertical` + `m.help.View(m.keys)` footer
  - Focus: `tab`/`shift+tab`; scroll via focused card's viewport
  - Disable `PrevMarker`/`NextMarker` when no markers exist
  - `esc` → cancel all contexts, close SSH sessions, back to FileList

**Test:** Grid opens with real hosts. Lines stream. Resize reflows cards.
`?` toggles help, cards resize to accommodate. `p` pauses. `m` drops marker.
`b`/`w` jump markers. `y` copies to clipboard. `esc` cleans up and returns.

---

## Task 10 — Filtering and search mode

**Goal:** Complete interactive filtering and historical grep.

**Deliver:**
- **Per-card filter** (`card.go`) — `f` activates `textinput` below header;
  real-time `rebuildViewport()` per keystroke; `ctrl+r` regex; `ctrl+i` case;
  `!` prefix excludes; `enter` locks; `esc` clears. Key matching via
  `key.Matches` in `GridModel.Update`.
- **Global filter** (`grid.go`) — `F` opens footer `textinput`;
  `GridModel.globalFilter` applied before per-card filter in `rebuildViewport()`
- **Level quick filter** — `1`–`4` set `GridModel.levelFilter`; passed into
  `rebuildViewport()` (no-op until Phase 2 parser is active)
- **Search mode** — `s` sets `ModeSearch`; search bar at top; `enter` runs
  `ssh.StartGrep()` concurrently; results stream as `SearchResultMsg`; card
  shows `list.Model` of results; `enter` on result → `DetailOverlay.visible = true`
  (placeholder overlay); `esc` returns to tail

Disable the `Search` binding when already in search mode:
```go
m.keys.Search.SetEnabled(m.mode == ModeTail)
```

**Test:** Filter → lines filter, matches highlighted. Regex mode works. `!err`
hides error lines. Global filter applies to all cards. `s` → search → results
per card. `esc` returns to tail with lines continuing.

---

## Task 11 — Phase 2: log parsing

**Goal:** Full JSON and level detection, coloured rendering.

**Deliver:**
- `parser/level.go` — implement `DetectLevel()`: JSON fields first, then raw
  text scan (SPEC.md Section 15). Implement `LevelPrefix()` fixed-width labels.
- `parser/json.go` — implement `TryParseJSON()` and `PrettyRenderJSON()` with
  per-type colours. Extract common fields (SPEC.md Section 15).
- `parser/parser.go` — implement `DefaultParser.Parse()`: JSON → level →
  key=value → coloured `Rendered` with `LevelPrefix` prepended.
- Wire level filter in `rebuildViewport()` — `1`–`4` now work.

**Test:** Unit tests: JSON detection, level from raw text, level from JSON,
`LevelPrefix` widths, `PrettyRenderJSON` output. Live: JSON lines render with
coloured fields. Level filter `4` hides INFO lines.

---

## Task 12 — Phase 2: DetailOverlay and clipboard

**Goal:** Full detail view, clipboard copy throughout.

**Deliver:**
- `ui/components/detail.go`
  - `detailKeyMap` with `help.Model` (SPEC.md Section 6)
  - All key handling via `key.Matches`
  - Centred modal, rounded `ColorPrimary` border
  - JSON: `PrettyRenderJSON` in scrollable viewport
  - Plain text: key=value two-column layout + raw line at top
  - `y` → `tea.SetClipboard(line.Raw)`
  - `Y` → `tea.SetClipboard(prettyJSON)`
  - Rendered via `lipgloss.Place` over grid (SPEC.md Section 11)
- Wire `enter` in tail mode → open overlay for viewport top line
- Wire `enter` in search mode → open overlay for selected result

**Test:** `enter` on JSON line → pretty-printed overlay. `enter` on plain
line → fields. `y` copies raw (verify paste). `Y` copies JSON. `esc` closes.
Overlay scrolls on long JSON.

---

## Task 13 — Polish, edge cases, known_hosts prompt

**Goal:** Remaining error paths, UX edge cases, graceful shutdown.

**Deliver:**
- **Unknown host key modal** — `FailHostKeyUnknown` → modal overlay with
  `key.Matches` for `y`/`n`; on accept append to `~/.ssh/known_hosts` and
  retry; on reject mark host failed
- **Host key changed** — `FailHostKeyChanged` → red error card with exact
  message + known_hosts location; no retry
- **Auth failure UX** — `FailAuthFailed` with passphrase key → show
  `"Key requires passphrase. Run: ssh-add ~/.ssh/your_key"` in error card
- **Empty project list** — styled empty state: `"No projects yet — press n"`
- **Terminal too small** — if `termW < 40` or `termH < 10`: show centred
  `"Terminal too small — please resize."` instead of grid
- **`esc` from FileList** — cancel all in-progress SSH connections cleanly
- **Graceful quit** — cancel all contexts, close all sessions, then
  `tea.Quit`; no dangling goroutines
- **Contextual help** — use `key.Binding.SetEnabled()` throughout so the
  help footer only shows bindings relevant to the current state

**Test:** Unknown host → prompt → accept → connects. Changed key → red card.
Auth failure with passphrase → `ssh-add` hint. Terminal resize to very small →
resize message. `go test -race` shows no data races. Quit → no leaks.

---

## Dependency graph

```
Task 1  (config)
Task 2  (ssh)              ← can run parallel with Task 1
Task 3  (parser stub)      ← can run parallel with Tasks 1-2
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

Tasks 1–3 are independent and can be done in any order or in parallel.
All others are strictly sequential.