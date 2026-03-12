package screens

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/fluid-movement/log-tui/config"
	"github.com/fluid-movement/log-tui/ui/styles"
)

// ─── Key map ────────────────────────────────────────────────────────────────

type creatorKeyMap struct {
	Next   key.Binding
	Toggle key.Binding
	Up     key.Binding
	Down   key.Binding
	Back   key.Binding
	Quit   key.Binding
}

func (k creatorKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Next, k.Toggle, k.Back, k.Quit}
}
func (k creatorKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Next, k.Toggle},
		{k.Up, k.Down},
		{k.Back, k.Quit},
	}
}

var defaultCreatorKeys = creatorKeyMap{
	Next:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "next/confirm")),
	Toggle: key.NewBinding(key.WithKeys(" ", "space"), key.WithHelp("space", "select host")),
	Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Back:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Quit:   key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
}

// ─── Steps ───────────────────────────────────────────────────────────────────

type creatorStep int

const (
	stepName creatorStep = iota
	stepHosts
	stepPath
)

// ─── Model ───────────────────────────────────────────────────────────────────

type CreatorModel struct {
	cfg       *config.Config
	step      creatorStep
	nameInput textinput.Model
	hosts     []config.SSHEntry // available SSH hosts
	selected  map[int]bool
	hostCur   int
	pathInput textinput.Model
	editingID string
	errMsg    string
	keys      creatorKeyMap
	help      help.Model
	width     int
	height    int
}

func NewCreatorModel(cfg *config.Config, editing *config.Project) CreatorModel {
	nameIn := textinput.New()
	nameIn.Placeholder = "Project name"
	nameIn.Focus()

	pathIn := textinput.New()
	pathIn.Placeholder = "/var/log"
	pathIn.SetValue("/var/log")

	sshEntries, _ := config.ParseSSHConfig()

	m := CreatorModel{
		cfg:       cfg,
		step:      stepName,
		nameInput: nameIn,
		hosts:     sshEntries,
		selected:  make(map[int]bool),
		pathInput: pathIn,
		keys:      defaultCreatorKeys,
		help:      help.New(),
	}

	if editing != nil {
		m.editingID = editing.ID
		m.nameInput.SetValue(editing.Name)
		m.pathInput.SetValue(editing.LogPath)
		// Pre-select matching hosts
		for i, e := range sshEntries {
			for _, h := range editing.Hosts {
				if e.Alias == h.Name {
					m.selected[i] = true
				}
			}
		}
	}

	return m
}

func (m *CreatorModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.help.SetWidth(w)
}

func (m CreatorModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m CreatorModel) Update(msg tea.Msg) (CreatorModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		m.errMsg = ""
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, func() tea.Msg { return tea.QuitMsg{} }

		case key.Matches(msg, m.keys.Back):
			if m.step == stepName {
				return m, func() tea.Msg { return SwitchMsg{To: ScreenProjects} }
			}
			m.step--
			return m, nil

		case key.Matches(msg, m.keys.Next):
			switch m.step {
			case stepName:
				if strings.TrimSpace(m.nameInput.Value()) == "" {
					m.errMsg = "Project name cannot be empty"
					return m, nil
				}
				m.step = stepHosts
				m.nameInput.Blur()

			case stepHosts:
				if len(m.selected) == 0 {
					m.errMsg = "Select at least one host"
					return m, nil
				}
				m.step = stepPath
				m.pathInput.Focus()

			case stepPath:
				path := strings.TrimSpace(m.pathInput.Value())
				if !filepath.IsAbs(path) {
					m.errMsg = "Log path must be absolute (e.g. /var/log/app.log)"
					return m, nil
				}
				return m, m.save()
			}

		case key.Matches(msg, m.keys.Toggle):
			if m.step == stepHosts && len(m.hosts) > 0 {
				if m.selected[m.hostCur] {
					delete(m.selected, m.hostCur)
				} else {
					m.selected[m.hostCur] = true
				}
			}

		case key.Matches(msg, m.keys.Up):
			if m.step == stepHosts && m.hostCur > 0 {
				m.hostCur--
			}

		case key.Matches(msg, m.keys.Down):
			if m.step == stepHosts && m.hostCur < len(m.hosts)-1 {
				m.hostCur++
			}
		}
	}

	switch m.step {
	case stepName:
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		cmds = append(cmds, cmd)
	case stepPath:
		var cmd tea.Cmd
		m.pathInput, cmd = m.pathInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m CreatorModel) save() tea.Cmd {
	return func() tea.Msg {
		var hosts []config.Host
		for i, e := range m.hosts {
			if m.selected[i] {
				hosts = append(hosts, config.Host{
					Name:          e.Alias,
					Hostname:      e.Hostname,
					User:          e.User,
					Port:          e.Port,
					IdentityFiles: e.IdentityFiles,
				})
			}
		}
		proj := config.Project{
			ID:        m.editingID,
			Name:      strings.TrimSpace(m.nameInput.Value()),
			Hosts:     hosts,
			LogPath:   strings.TrimSpace(m.pathInput.Value()),
			CreatedAt: time.Now(),
		}
		if proj.ID == "" {
			proj.ID = fmt.Sprintf("%d", time.Now().UnixNano())
			_ = config.AddProject(m.cfg, proj)
		} else {
			_ = config.UpdateProject(m.cfg, proj)
		}
		return SwitchMsg{To: ScreenProjects}
	}
}

func (m CreatorModel) View() string {
	var b strings.Builder

	title := "New Project"
	if m.editingID != "" {
		title = "Edit Project"
	}
	b.WriteString(styles.TitleStyle.Render(title) + "\n\n")

	switch m.step {
	case stepName:
		b.WriteString("Step 1/3 — Project name\n\n")
		b.WriteString(m.nameInput.View() + "\n")

	case stepHosts:
		b.WriteString("Step 2/3 — Select hosts (space to toggle)\n\n")
		if len(m.hosts) == 0 {
			b.WriteString(styles.MutedStyle.Render("No hosts found in ~/.ssh/config") + "\n")
		} else {
			for i, e := range m.hosts {
				cursor := "  "
				if i == m.hostCur {
					cursor = "> "
				}
				check := "[ ]"
				if m.selected[i] {
					check = "[x]"
				}
				line := fmt.Sprintf("%s%s %s", cursor, check, e.Alias)
				if e.Hostname != e.Alias {
					line += styles.MutedStyle.Render(" ("+e.Hostname+")")
				}
				b.WriteString(line + "\n")
			}
		}

	case stepPath:
		b.WriteString("Step 3/3 — Log file path\n\n")
		b.WriteString(m.pathInput.View() + "\n")
	}

	if m.errMsg != "" {
		b.WriteString("\n" + styles.ErrorStyle.Render(m.errMsg) + "\n")
	}

	b.WriteString("\n" + m.help.View(m.keys))
	return b.String()
}

// Width / Height expose the current terminal dimensions for testing.
func (m CreatorModel) Width() int  { return m.width }
func (m CreatorModel) Height() int { return m.height }
