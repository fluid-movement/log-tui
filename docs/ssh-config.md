---
title: SSH Config Parsing and Host Validation
description: ParseSSHConfig reads ~/.ssh/config; ValidateProjectHosts resolves hosts via exact alias then soft hostname fallback.
---

# SSH Config Parsing and Host Validation

## ParseSSHConfig

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

## Host validation on project open

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
