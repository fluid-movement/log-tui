---
title: Application Screens
description: Screen enum, SwitchScreenMsg, AppModel routing, and per-screen model specs for ProjectList, ProjectCreator, FileList, and ServerGrid.
---

# Application Screens

## Screen enum

```go
type screen int
const (
    screenProjects screen = iota
    screenCreator
    screenFileList
    screenGrid
)
```

## SwitchScreenMsg

```go
type SwitchScreenMsg struct {
    To      screen
    Payload any
}
```

## AppModel (ui/app.go)

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

## ProjectList Screen

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

## ProjectCreator Screen

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

## FileList Screen

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

## ServerGrid Screen

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
