---
title: Tail Streaming
description: tail -F streaming pattern, LogLine/LogLineMsg/HostStatusMsg types, listenForLog re-issue pattern, and receive-time timestamps.
---

# Tail Streaming

## Always `tail -F` (capital F)

```go
const tailCmd = "tail -F -n 0 %s"
const grepCmd = "grep -n -E %q %s"
```

## Message types

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

## Streaming pattern

```go
func listenForLog(ch chan LogLineMsg) tea.Cmd {
    return func() tea.Msg { return <-ch }
}
// Re-issue from Update after every LogLineMsg.
```

## Receive-time timestamps

Prepend to every viewport line: `15:04:05.000  <log content>`
