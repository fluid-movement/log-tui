---
title: Data Models
description: Core config structs — Host, Project, Config — with JSON tags and schema migration notes.
---

# Data Models

## config.Host

```go
type Host struct {
    Name     string `json:"name"`     // alias from ~/.ssh/config
    Hostname string `json:"hostname"` // resolved IP/FQDN — used for soft match
    User     string `json:"user"`
    Port     int    `json:"port"`
}
```

## config.Project

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

## config.Config

```go
type Config struct {
    Version  int       `json:"version"`
    Projects []Project `json:"projects"`
}
```

On load: if `Config.Version` < current schema version, run migration and
re-save. Use `json:",omitempty"` on all optional fields.
