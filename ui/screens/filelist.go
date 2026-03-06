package screens

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/fluid-movement/log-tui/clog"
	"github.com/fluid-movement/log-tui/config"
	gossh "github.com/fluid-movement/log-tui/ssh"
	cryptossh "golang.org/x/crypto/ssh"
	"github.com/fluid-movement/log-tui/ui/styles"
)

// ─── Key map ────────────────────────────────────────────────────────────────

type filelistKeyMap struct {
	Open   key.Binding
	Rescan key.Binding
	Back   key.Binding
	Help   key.Binding
	Quit   key.Binding
}

func (k filelistKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Open, k.Rescan, k.Back, k.Help, k.Quit}
}
func (k filelistKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Open, k.Rescan},
		{k.Back, k.Help, k.Quit},
	}
}

var defaultFilelistKeys = filelistKeyMap{
	Open:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "tail file")),
	Rescan: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rescan")),
	Back:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Help:   key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

// ─── Internal messages ────────────────────────────────────────────────────────

type hostConnectedMsg struct {
	host   string
	client *gossh.Client
	err    error
}

type filesListedMsg struct {
	host  string
	files []string
	err   error
}

// ─── List item ────────────────────────────────────────────────────────────────

type fileItem struct {
	name      string
	available map[string]bool // host name → available?
}

func (f fileItem) Title() string { return f.name }
func (f fileItem) Description() string {
	var ok, fail []string
	for h, avail := range f.available {
		if avail {
			ok = append(ok, h)
		} else {
			fail = append(fail, h)
		}
	}
	sort.Strings(ok)
	sort.Strings(fail)
	parts := []string{}
	for _, h := range ok {
		parts = append(parts, styles.BadgeConnectedStyle.Render("●")+" "+h)
	}
	for _, h := range fail {
		parts = append(parts, styles.BadgeDisconnectedStyle.Render("✗")+" "+h)
	}
	return strings.Join(parts, "  ")
}
func (f fileItem) FilterValue() string { return f.name }

// ─── Model ───────────────────────────────────────────────────────────────────

type hostState struct {
	status gossh.HostStatus
	client *gossh.Client
	err    error
}

// unknownKeyPrompt holds state for the "Add host key? [y/N]" modal.
type unknownKeyPrompt struct {
	active     bool
	host       config.Host
	hostname   string
	serverKey  cryptossh.PublicKey
	remoteAddr net.Addr
}

type FileListModel struct {
	project    config.Project
	hostStates map[string]*hostState
	files      map[string][]string // host → file list
	list       list.Model
	spinner    spinner.Model
	keys       filelistKeyMap
	help       help.Model
	width      int
	height     int
	ctx        context.Context
	cancel     context.CancelFunc
	errMsg     string
	keyPrompt  unknownKeyPrompt
	ready      bool
}

func NewFileListModel(proj config.Project) FileListModel {
	delegate := list.NewDefaultDelegate()
	l := list.New(nil, delegate, 0, 0)
	l.SetShowTitle(false)
	l.SetShowHelp(false)

	ctx, cancel := context.WithCancel(context.Background())

	return FileListModel{
		project:    proj,
		hostStates: make(map[string]*hostState),
		files:      make(map[string][]string),
		list:       l,
		spinner:    spinner.New(spinner.WithSpinner(spinner.Dot)),
		keys:       defaultFilelistKeys,
		help:       help.New(),
		ctx:        ctx,
		cancel:     cancel,
		ready:      true,
	}
}

func (m *FileListModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	if !m.ready {
		return
	}
	m.help.SetWidth(w)
	footerH := 1
	if m.help.ShowAll {
		footerH = 4
	}
	// header = title(1) + blank(1) + host status lines + blank(1) = len(hosts)+3
	// separator "\n" after list.View() = 1
	headerH := len(m.project.Hosts) + 3
	m.list.SetSize(w, h-headerH-footerH-1)
}

func (m FileListModel) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spinner.Tick}
	// Connect to each host concurrently.
	for _, h := range m.project.Hosts {
		h := h
		m.hostStates[h.Name] = &hostState{status: gossh.StatusConnecting}
		cmds = append(cmds, func() tea.Msg {
			return connectAndList(m.ctx, h, m.project.LogPath)
		})
	}
	return tea.Batch(cmds...)
}

func connectAndList(ctx context.Context, host config.Host, logPath string) tea.Msg {
	client, err := gossh.Connect(ctx, host)
	if err != nil {
		return hostConnectedMsg{host: host.Name, err: err}
	}
	// List files in the log path directory.
	files, lsErr := listFiles(client, logPath)
	return struct {
		connected hostConnectedMsg
		listed    filesListedMsg
	}{
		connected: hostConnectedMsg{host: host.Name, client: client},
		listed:    filesListedMsg{host: host.Name, files: files, err: lsErr},
	}
}

func listFiles(client *gossh.Client, path string) ([]string, error) {
	sess, err := client.SSHClient().NewSession()
	if err != nil {
		return nil, err
	}
	defer sess.Close()

	out, err := sess.Output(fmt.Sprintf("ls -1 %s 2>/dev/null || echo ''", path))
	if err != nil {
		return nil, err
	}
	var files []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

func (m FileListModel) Update(msg tea.Msg) (FileListModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case hostConnectedMsg:
		hs := m.getOrCreateHostState(msg.host)
		if msg.err != nil {
			// Check if this is an unknown host key error — show prompt.
			var ce *gossh.ConnectError
			if errors.As(msg.err, &ce) && ce.Reason == gossh.FailHostKeyUnknown {
				// Find the host config for the prompt.
				for _, h := range m.project.Hosts {
					if h.Name == msg.host {
						m.keyPrompt = unknownKeyPrompt{
							active:     true,
							host:       h,
							hostname:   msg.host,
							serverKey:  ce.ServerKey,
							remoteAddr: ce.RemoteAddr,
						}
						break
					}
				}
				hs.status = gossh.StatusError
				hs.err = msg.err
				return m, nil
			}
			hs.status = gossh.StatusError
			hs.err = msg.err
			clog.Debug("connect error", "host", msg.host, "err", msg.err)
		} else {
			hs.status = gossh.StatusConnected
			hs.client = msg.client
		}
		m.rebuildList()

	case filesListedMsg:
		if msg.err == nil {
			m.files[msg.host] = msg.files
		}
		m.rebuildList()

	// Handle the compound message from connectAndList
	case struct {
		connected hostConnectedMsg
		listed    filesListedMsg
	}:
		hs := m.getOrCreateHostState(msg.connected.host)
		if msg.connected.err != nil {
			hs.status = gossh.StatusError
			hs.err = msg.connected.err
		} else {
			hs.status = gossh.StatusConnected
			hs.client = msg.connected.client
			if msg.listed.err == nil {
				m.files[msg.listed.host] = msg.listed.files
			}
		}
		m.rebuildList()

	case tea.KeyPressMsg:
		// Handle unknown host key prompt.
		if m.keyPrompt.active {
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("y", "Y"))):
				kp := m.keyPrompt
				m.keyPrompt = unknownKeyPrompt{}
				// Persist the server host key, then retry the connection.
				return m, func() tea.Msg {
					if kp.serverKey != nil && kp.remoteAddr != nil {
						addr := kp.remoteAddr.String() // "host:port"
						if err := gossh.AddKnownHost(addr, kp.remoteAddr, kp.serverKey); err != nil {
							clog.Error("failed to add known host", "err", err)
						}
					}
					return connectAndList(m.ctx, kp.host, m.project.LogPath)
				}
			default:
				// Decline — keep the error state.
				m.keyPrompt = unknownKeyPrompt{}
			}
			return m, nil
		}

		switch {
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			m.SetSize(m.width, m.height)

		case key.Matches(msg, m.keys.Quit):
			m.cancel()
			return m, func() tea.Msg { return tea.QuitMsg{} }

		case key.Matches(msg, m.keys.Back):
			m.cancel()
			return m, func() tea.Msg { return SwitchMsg{To: ScreenProjects} }

		case key.Matches(msg, m.keys.Rescan):
			// Rescan: cancel old connections, start fresh
			m.cancel()
			newCtx, newCancel := context.WithCancel(context.Background())
			m.ctx = newCtx
			m.cancel = newCancel
			m.files = make(map[string][]string)
			m.hostStates = make(map[string]*hostState)
			return m, m.Init()

		case key.Matches(msg, m.keys.Open):
			if item, ok := m.list.SelectedItem().(fileItem); ok {
				// Find connected clients
				var clients []*gossh.Client
				for _, hs := range m.hostStates {
					if hs.client != nil {
						clients = append(clients, hs.client)
					}
				}
				filePath := item.name
				proj := m.project
				return m, func() tea.Msg {
					return SwitchMsg{
						To: ScreenGrid,
						Payload: GridPayload{
							Project:  proj,
							FilePath: filePath,
							Clients:  clients,
						},
					}
				}
			}
		}
	}

	var listCmd tea.Cmd
	m.list, listCmd = m.list.Update(msg)
	cmds = append(cmds, listCmd)
	return m, tea.Batch(cmds...)
}

func (m *FileListModel) getOrCreateHostState(name string) *hostState {
	if m.hostStates[name] == nil {
		m.hostStates[name] = &hostState{}
	}
	return m.hostStates[name]
}

func (m *FileListModel) rebuildList() {
	// Merge files from all hosts into a unified sorted list.
	fileset := make(map[string]map[string]bool)
	for _, h := range m.project.Hosts {
		for _, f := range m.files[h.Name] {
			if fileset[f] == nil {
				fileset[f] = make(map[string]bool)
			}
			fileset[f][h.Name] = true
		}
	}

	var names []string
	for name := range fileset {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([]list.Item, 0, len(names))
	for _, name := range names {
		items = append(items, fileItem{name: name, available: fileset[name]})
	}
	m.list.SetItems(items)
}

func (m FileListModel) View() string {
	var b strings.Builder
	b.WriteString(styles.TitleStyle.Render(m.project.Name+" — Files") + "\n\n")

	// Status line per host
	for _, h := range m.project.Hosts {
		hs := m.hostStates[h.Name]
		if hs == nil {
			continue
		}
		switch hs.status {
		case gossh.StatusConnecting:
			b.WriteString(m.spinner.View() + " " + h.Name + " connecting…\n")
		case gossh.StatusConnected:
			b.WriteString(styles.BadgeConnectedStyle.Render("●") + " " + h.Name + "\n")
		case gossh.StatusError:
			errStr := ""
			if hs.err != nil {
				errStr = ": " + hs.err.Error()
			}
			b.WriteString(styles.BadgeDisconnectedStyle.Render("✗") + " " + h.Name + errStr + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(m.list.View() + "\n")

	// Unknown host key prompt overlay.
	if m.keyPrompt.active {
		prompt := styles.ErrorStyle.Render("⚠ Unknown host key for " + m.keyPrompt.hostname)
		prompt += "\n" + styles.MutedStyle.Render("Add to ~/.ssh/known_hosts? [y/N]")
		b.WriteString("\n" + prompt + "\n")
	} else {
		b.WriteString(m.help.View(m.keys))
	}
	return b.String()
}
