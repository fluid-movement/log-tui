# logviewer

A terminal UI for tailing and searching log files across multiple remote servers simultaneously, over SSH.

```
┌─ server-prod-1 ──────────────────────┐ ┌─ server-prod-2 ──────────────────────┐
│ 14:23:01.042  INFO  app started      │ │ 14:23:01.103  INFO  app started      │
│ 14:23:04.817  INFO  request POST /api│ │ 14:23:05.221  INFO  request GET /api │
│ 14:23:04.819  INFO  200 OK 42ms      │ │ 14:23:05.224  INFO  200 OK 38ms      │
│ 14:23:07.334  ERROR db timeout       │ │ 14:23:07.891  WARN  high memory      │
│ 14:23:07.335  ERROR retrying (1/3)   │ │                                      │
└──────────────────────────────────────┘ └──────────────────────────────────────┘
  [FILTERED: error]   5 lines  14:23:07    42 lines  14:23:07
f filter  F global filter  s search  p pause  m marker  ?  help  q quit
```

---

## Installation

Requires Go 1.23+.

```bash
git clone https://github.com/yourname/logviewer
cd logviewer
go build -o logviewer .
# optionally move to your PATH
mv logviewer /usr/local/bin/
```

---

## Prerequisites

- SSH access to your servers configured in `~/.ssh/config`
- `ssh-agent` running with your keys added (`ssh-add ~/.ssh/your_key`) if your keys have passphrases
- `tail` available on remote servers (standard on all Unix systems)

---

## Quick start

**1. Create a project**

Press `n` on the project list screen. You'll be asked for:
- A project name
- Which servers to include — picked from your `~/.ssh/config` entries
- The log directory path on those servers (e.g. `/var/log/myapp`)

**2. Open a project**

Select it and press `enter`. logviewer connects to all servers, lists the
available log files, and shows which files exist on which servers.

**3. Tail a log file**

Select a file and press `enter`. A live grid opens — one panel per server,
updating in real time.

---

## Grid view

Each server gets a panel. The layout adapts to how many servers you have:

| Servers | Layout      |
|---------|-------------|
| 1       | Full screen |
| 2       | Side by side |
| 3–4     | 2 × 2 grid  |
| 5–6     | 3 × 2 grid  |

Press `?` to toggle a full help view listing all available keys.

### Navigation

| Key | Action |
|-----|--------|
| `tab` / `shift+tab` | Move focus between panels |
| `↑` `↓` or `j` `k` | Scroll the focused panel |
| `g` / `G` | Jump to top / bottom |

### Filtering

| Key | Action |
|-----|--------|
| `f` | Filter the focused panel |
| `F` | Filter all panels at once |
| `1`–`4` | Minimum log level: DEBUG / INFO / WARN / ERROR |

While the filter bar is open:
- Type to filter in real time — matching text is highlighted
- Prefix with `!` to exclude instead: `!health` hides health check lines
- `ctrl+r` to switch between plain text and regex
- `ctrl+i` to toggle case sensitivity
- `enter` to lock the filter, `esc` to clear it

### Searching history

Press `s` to search the log file rather than tail it. Results from all servers
are shown side by side. Press `enter` on any result to inspect it in detail,
`esc` to return to live tail.

### Other actions

| Key | Action |
|-----|--------|
| `p` | Pause / resume a panel (lines buffer while paused) |
| `m` | Drop a marker — useful when reproducing a bug |
| `b` / `w` | Jump to previous / next marker |
| `y` | Copy the current line to clipboard |
| `enter` | Open detail view for the selected line |
| `esc` | Back to file list |
| `q` | Quit |

### Detail view

Press `enter` on any line to open a full detail panel. For JSON log lines,
fields are pretty-printed with syntax highlighting. Press `y` to copy the raw
line, `Y` to copy the formatted JSON, `esc` to close.

---

## Projects

Projects are stored in:

| OS | Path |
|----|------|
| macOS | `~/Library/Application Support/logviewer/projects.json` |
| Linux | `~/.config/logviewer/projects.json` |
| Windows | `%AppData%\logviewer\projects.json` |

**If you rename a server alias in `~/.ssh/config`**, logviewer detects this
automatically by matching the hostname — your projects will continue to work.
A warning badge `⚠` appears on any project where a server can no longer be
resolved, so you know to update it.

---

## Troubleshooting

**"auth failed: no valid key found"**
Your key may require a passphrase and `ssh-agent` is not running. Fix:
```bash
eval $(ssh-agent)
ssh-add ~/.ssh/your_key
```

**"Add host key to known_hosts?"**
The server is not in your `~/.ssh/known_hosts`. logviewer will prompt you —
press `y` to add it and connect, `n` to skip that server.

**Host key has changed**
logviewer will refuse to connect and show the exact error. Investigate before
proceeding — this can indicate a security issue. If you are sure the key is
legitimate, update your `known_hosts` manually with `ssh-keyscan`.

**One server fails, others work fine**
logviewer continues with the servers that are reachable. The failed server shows
an error badge in its panel. Reconnection is attempted automatically with
exponential backoff.

---

## Debug logging

```bash
LOG_DEBUG=1 ./logviewer
# in another terminal:
tail -f /tmp/logviewer-debug.log
```