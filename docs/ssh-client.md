---
title: SSH Client
description: Client struct, Connect(), typed ConnectError/ConnectFailReason, knownhosts verification, and exponential backoff reconnection.
---

# SSH Client

```go
type Client struct {
    host   config.Host
    conn   *gossh.Client
    cancel context.CancelFunc
}

func Connect(ctx context.Context, host config.Host) (*Client, error)
```

## Auth order

1. `ssh-agent` via `SSH_AUTH_SOCK`
2. Identity files from `~/.ssh/config` for the host

## Host key verification

Use `golang.org/x/crypto/ssh/knownhosts`. Never skip.

## Typed errors

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

## Reconnection (exponential backoff)

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
