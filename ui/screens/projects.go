package screens

import (
	"fmt"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fluid-movement/log-tui/config"
	"github.com/fluid-movement/log-tui/ui/styles"
)

// ─── Key map ────────────────────────────────────────────────────────────────

type projectsKeyMap struct {
	Open   key.Binding
	New    key.Binding
	Edit   key.Binding
	Delete key.Binding
	Help   key.Binding
	Quit   key.Binding
}

func (k projectsKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Open, k.New, k.Edit, k.Delete, k.Help, k.Quit}
}
func (k projectsKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Open, k.New, k.Edit, k.Delete},
		{k.Help, k.Quit},
	}
}

var defaultProjectsKeys = projectsKeyMap{
	Open:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
	New:    key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new project")),
	Edit:   key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
	Delete: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
	Help:   key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

// ─── List item ───────────────────────────────────────────────────────────────

type projectItem struct {
	proj       config.Project
	hasWarning bool
}

func (p projectItem) Title() string {
	title := p.proj.Name
	if p.hasWarning {
		title += " ⚠"
	}
	return title
}

func (p projectItem) Description() string {
	n := len(p.proj.Hosts)
	return fmt.Sprintf("%d host(s) · %s", n, p.proj.LogPath)
}

func (p projectItem) FilterValue() string { return p.proj.Name }

// ─── Model ───────────────────────────────────────────────────────────────────

type ProjectsModel struct {
	cfg        *config.Config
	list       list.Model
	keys       projectsKeyMap
	help       help.Model
	width      int
	height     int
	confirmDel string // project ID pending deletion
}

func NewProjectsModel(cfg *config.Config) ProjectsModel {
	items := projectsToItems(cfg)
	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = "logviewer — Projects"
	l.Styles.Title = styles.TitleStyle
	l.SetShowHelp(false) // we render our own footer
	l.SetFilteringEnabled(false)

	m := ProjectsModel{
		cfg:  cfg,
		list: l,
		keys: defaultProjectsKeys,
		help: help.New(),
	}
	return m
}

func projectsToItems(cfg *config.Config) []list.Item {
	items := make([]list.Item, 0, len(cfg.Projects))
	for _, p := range cfg.Projects {
		results := config.ValidateProjectHosts(p)
		hasWarn := false
		for _, r := range results {
			if !r.Resolved {
				hasWarn = true
				break
			}
		}
		items = append(items, projectItem{proj: p, hasWarning: hasWarn})
	}
	return items
}

func (m *ProjectsModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.help.SetWidth(w)
	footerH := 1
	if m.help.ShowAll {
		footerH = 4
	}
	m.list.SetSize(w, h-footerH-1)
}

func (m ProjectsModel) Init() tea.Cmd {
	return nil
}

func (m ProjectsModel) Update(msg tea.Msg) (ProjectsModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.confirmDel != "" {
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("y", "Y"))):
				_ = config.DeleteProject(m.cfg, m.confirmDel)
				m.confirmDel = ""
				m.list.SetItems(projectsToItems(m.cfg))
			default:
				m.confirmDel = ""
			}
			return m, nil
		}

		switch {
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			m.SetSize(m.width, m.height)

		case key.Matches(msg, m.keys.Quit):
			return m, func() tea.Msg { return tea.QuitMsg{} }

		case key.Matches(msg, m.keys.New):
			return m, func() tea.Msg {
				return SwitchMsg{To: ScreenCreator, Payload: nil}
			}

		case key.Matches(msg, m.keys.Edit):
			if item, ok := m.list.SelectedItem().(projectItem); ok {
				proj := item.proj
				return m, func() tea.Msg {
					return SwitchMsg{To: ScreenCreator, Payload: &proj}
				}
			}

		case key.Matches(msg, m.keys.Delete):
			if item, ok := m.list.SelectedItem().(projectItem); ok {
				m.confirmDel = item.proj.ID
			}

		case key.Matches(msg, m.keys.Open):
			if item, ok := m.list.SelectedItem().(projectItem); ok {
				proj := item.proj
				return m, func() tea.Msg {
					return SwitchMsg{To: ScreenFileList, Payload: proj}
				}
			}
		}
	}

	var listCmd tea.Cmd
	m.list, listCmd = m.list.Update(msg)
	cmds = append(cmds, listCmd)
	return m, tea.Batch(cmds...)
}

func (m ProjectsModel) View() string {
	footer := m.help.View(m.keys)
	if m.confirmDel != "" {
		footer = styles.ErrorStyle.Render("Delete project? [y/N]")
	}

	if len(m.cfg.Projects) == 0 {
		empty := lipgloss.NewStyle().
			Width(m.width).
			Height(m.height-2).
			AlignHorizontal(lipgloss.Center).
			AlignVertical(lipgloss.Center).
			Render(styles.MutedStyle.Render("No projects yet — press n"))
		return empty + "\n" + footer
	}

	return m.list.View() + "\n" + footer
}

// Width / Height expose the current terminal dimensions for testing.
func (m ProjectsModel) Width() int  { return m.width }
func (m ProjectsModel) Height() int { return m.height }
