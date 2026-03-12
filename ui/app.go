package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/fluid-movement/log-tui/config"
	"github.com/fluid-movement/log-tui/ui/screens"
)

// AppModel is the root Bubbletea model. It owns the current screen and routes messages.
type AppModel struct {
	current  screens.ScreenID
	cfg      *config.Config
	width    int
	height   int
	projects screens.ProjectsModel
	creator  screens.CreatorModel
	filelist screens.FileListModel
	grid     screens.GridModel
}

// New returns a ready-to-run AppModel.
func New(cfg *config.Config) AppModel {
	return AppModel{
		cfg:      cfg,
		current:  screens.ScreenProjects,
		projects: screens.NewProjectsModel(cfg),
	}
}

func (m AppModel) Init() tea.Cmd {
	return m.projects.Init()
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Propagate to all sub-models, including inactive ones.
		m.projects.SetSize(msg.Width, msg.Height)
		m.creator.SetSize(msg.Width, msg.Height)
		m.filelist.SetSize(msg.Width, msg.Height)
		m.grid.SetSize(msg.Width, msg.Height)

	case screens.SwitchMsg:
		return m.handleSwitch(msg)
	}

	// Route to the current screen.
	switch m.current {
	case screens.ScreenProjects:
		updated, cmd := m.projects.Update(msg)
		m.projects = updated
		cmds = append(cmds, cmd)

	case screens.ScreenCreator:
		updated, cmd := m.creator.Update(msg)
		m.creator = updated
		cmds = append(cmds, cmd)

	case screens.ScreenFileList:
		updated, cmd := m.filelist.Update(msg)
		m.filelist = updated
		cmds = append(cmds, cmd)

	case screens.ScreenGrid:
		updated, cmd := m.grid.Update(msg)
		m.grid = updated
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m AppModel) handleSwitch(msg screens.SwitchMsg) (tea.Model, tea.Cmd) {
	m.current = msg.To
	switch msg.To {
	case screens.ScreenProjects:
		m.projects = screens.NewProjectsModel(m.cfg)
		m.projects.SetSize(m.width, m.height)
		return m, m.projects.Init()

	case screens.ScreenCreator:
		var editProj *config.Project
		if p, ok := msg.Payload.(*config.Project); ok {
			editProj = p
		}
		m.creator = screens.NewCreatorModel(m.cfg, editProj)
		m.creator.SetSize(m.width, m.height)
		return m, m.creator.Init()

	case screens.ScreenFileList:
		proj, _ := msg.Payload.(config.Project)
		m.filelist = screens.NewFileListModel(proj)
		m.filelist.SetSize(m.width, m.height)
		return m, m.filelist.Init()

	case screens.ScreenGrid:
		payload, _ := msg.Payload.(screens.GridPayload)
		m.grid = screens.NewGridModel(payload)
		m.grid.SetSize(m.width, m.height)
		return m, m.grid.Init()
	}
	return m, nil
}

func (m AppModel) View() tea.View {
	// Terminal too small guard
	if m.width > 0 && m.height > 0 && (m.width < 40 || m.height < 10) {
		v := tea.NewView("Terminal too small — please resize.")
		v.AltScreen = true
		return v
	}

	var content string
	switch m.current {
	case screens.ScreenProjects:
		content = m.projects.View()
	case screens.ScreenCreator:
		content = m.creator.View()
	case screens.ScreenFileList:
		content = m.filelist.View()
	case screens.ScreenGrid:
		content = m.grid.View()
	default:
		content = "Loading…"
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}
