package screens

import (
	"context"
	"fmt"
	"math"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fluid-movement/log-tui/clog"
	"github.com/fluid-movement/log-tui/config"
	"github.com/fluid-movement/log-tui/parser"
	gossh "github.com/fluid-movement/log-tui/ssh"
	"github.com/fluid-movement/log-tui/ui/components"
	"github.com/fluid-movement/log-tui/ui/styles"
)

// ─── Key map ────────────────────────────────────────────────────────────────

type gridKeyMap struct {
	Up           key.Binding
	Down         key.Binding
	Top          key.Binding
	Bottom       key.Binding
	FocusNext    key.Binding
	FocusPrev    key.Binding
	Filter       key.Binding
	GlobalFilter key.Binding
	LevelDebug   key.Binding
	LevelInfo    key.Binding
	LevelWarn    key.Binding
	LevelError   key.Binding
	Search       key.Binding
	Pause        key.Binding
	Marker       key.Binding
	PrevMarker   key.Binding
	NextMarker   key.Binding
	Detail       key.Binding
	Copy         key.Binding
	Back         key.Binding
	Help         key.Binding
	Quit         key.Binding
}

func (k gridKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Filter, k.Search, k.Pause, k.Marker, k.Help, k.Quit}
}

func (k gridKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom, k.FocusNext, k.FocusPrev},
		{k.Filter, k.GlobalFilter, k.LevelDebug, k.LevelInfo, k.LevelWarn, k.LevelError},
		{k.Search, k.Pause, k.Marker, k.PrevMarker, k.NextMarker},
		{k.Detail, k.Copy, k.Back, k.Help, k.Quit},
	}
}

var defaultGridKeys = gridKeyMap{
	Up:           key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "scroll up")),
	Down:         key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "scroll down")),
	Top:          key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
	Bottom:       key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
	FocusNext:    key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next panel")),
	FocusPrev:    key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev panel")),
	Filter:       key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "filter panel")),
	GlobalFilter: key.NewBinding(key.WithKeys("F"), key.WithHelp("F", "filter all")),
	LevelDebug:   key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "show all")),
	LevelInfo:    key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "info+")),
	LevelWarn:    key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "warn+")),
	LevelError:   key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "error+")),
	Search:       key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "search")),
	Pause:        key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pause")),
	Marker:       key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "marker")),
	PrevMarker:   key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "prev marker")),
	NextMarker:   key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "next marker")),
	Detail:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "detail")),
	Copy:         key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy line")),
	Back:         key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Help:         key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Quit:         key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

// ─── Grid mode ────────────────────────────────────────────────────────────────

type GridMode int

const (
	ModeTail GridMode = iota
	ModeSearch
	ModeGlobalFilter
)

// ─── Model ───────────────────────────────────────────────────────────────────

// SearchResultMsg carries grep results for one host.
type SearchResultMsg struct {
	Host  string
	Lines []string
}

type GridModel struct {
	project        config.Project
	filePath       string
	cards          []components.ServerCard
	focusedIdx     int
	mode           GridMode
	globalFilter   components.FilterState
	globalInput    textinput.Model
	searchInput    textinput.Model
	filterMode     components.FilterMode
	filterCase     bool
	levelFilter    parser.Level
	detail         components.DetailOverlay
	searchResults  map[string][]string // host → result lines
	width          int
	height         int
	clients        []*gossh.Client
	logChans       []chan gossh.LogLineMsg
	ctxs           []context.Context
	cancels        []context.CancelFunc
	searchCancels  []context.CancelFunc
	keys           gridKeyMap
	help           help.Model
	parse          parser.DefaultParser
}

func NewGridModel(payload GridPayload) GridModel {
	keys := defaultGridKeys

	gi := textinput.New()
	gi.Placeholder = "global filter…"

	si := textinput.New()
	si.Placeholder = "search pattern (grep -E)…"

	m := GridModel{
		project:       payload.Project,
		filePath:      payload.FilePath,
		clients:       payload.Clients,
		keys:          keys,
		help:          help.New(),
		levelFilter:   parser.LevelUnknown,
		detail:        components.NewDetailOverlay(),
		globalInput:   gi,
		searchInput:   si,
		searchResults: make(map[string][]string),
	}

	// Build one card per client.
	for _, client := range payload.Clients {
		card := components.NewServerCard(client.Host)
		m.cards = append(m.cards, card)

		ctx, cancel := context.WithCancel(context.Background())
		m.ctxs = append(m.ctxs, ctx)
		m.cancels = append(m.cancels, cancel)

		ch := make(chan gossh.LogLineMsg, 256)
		m.logChans = append(m.logChans, ch)
	}

	m.updateKeyStates()
	return m
}

func (m *GridModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.help.SetWidth(w)
	m.resizeCards()
}

func (m *GridModel) resizeCards() {
	if len(m.cards) == 0 || m.width == 0 || m.height == 0 {
		return
	}
	cols, rows, cardW, cardH := gridDimensions(len(m.cards), m.width, m.height, m.helpBarHeight())
	_ = rows
	for i := range m.cards {
		col := i % cols
		_ = col
		m.cards[i].SetSize(cardW, cardH)
	}
}

func (m GridModel) helpBarHeight() int {
	if m.help.ShowAll {
		return 5
	}
	return 1
}

func gridDimensions(n, termW, termH, helpBarH int) (cols, rows, cardW, cardH int) {
	// Reserve 1 line for the "\n" separator between the grid and the footer.
	effective := termH - helpBarH - 1
	if effective < 1 {
		effective = 1
	}
	if n == 0 {
		return 1, 1, termW, effective
	}
	cols = int(math.Ceil(math.Sqrt(float64(n))))
	if cols == 0 {
		cols = 1
	}
	rows = int(math.Ceil(float64(n) / float64(cols)))
	if rows == 0 {
		rows = 1
	}
	cardW = termW / cols
	cardH = effective / rows
	return
}

func (m GridModel) Init() tea.Cmd {
	var cmds []tea.Cmd
	for i, client := range m.clients {
		i := i
		client := client
		ch := m.logChans[i]
		ctx := m.ctxs[i]
		// Start tail and listen.
		cmds = append(cmds, gossh.StartTail(ctx, client, m.filePath, ch))
	}
	return tea.Batch(cmds...)
}

func (m GridModel) Update(msg tea.Msg) (GridModel, tea.Cmd) {
	var cmds []tea.Cmd

	// Detail overlay intercepts all input when visible.
	if m.detail.Visible {
		if cmd := m.detail.Update(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}

	// Global filter input mode
	if m.mode == ModeGlobalFilter {
		switch msg := msg.(type) {
		case tea.KeyPressMsg:
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
				m.globalFilter = components.NewFilterState(
					m.globalInput.Value(), m.filterMode, m.filterCase)
				m.mode = ModeTail
				m.globalInput.Blur()
				for i := range m.cards {
					m.cards[i].SetGlobalFilter(m.globalFilter)
				}
			case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
				m.mode = ModeTail
				m.globalInput.Reset()
				m.globalInput.Blur()
				m.globalFilter = components.FilterState{}
				for i := range m.cards {
					m.cards[i].SetGlobalFilter(components.FilterState{})
				}
			case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+r"))):
				if m.filterMode == components.FilterRegex {
					m.filterMode = components.FilterText
				} else {
					m.filterMode = components.FilterRegex
				}
			case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+i"))):
				m.filterCase = !m.filterCase
			default:
				var cmd tea.Cmd
				m.globalInput, cmd = m.globalInput.Update(msg)
				cmds = append(cmds, cmd)
				// Live filter update
				m.globalFilter = components.NewFilterState(
					m.globalInput.Value(), m.filterMode, m.filterCase)
				for i := range m.cards {
					m.cards[i].SetGlobalFilter(m.globalFilter)
				}
			}
		}
		return m, tea.Batch(cmds...)
	}

	// Per-card filter input mode
	if len(m.cards) > 0 && m.cards[m.focusedIdx].Filtering() {
		card := &m.cards[m.focusedIdx]
		switch msg := msg.(type) {
		case tea.KeyPressMsg:
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
				card.SetFilter(m.filterMode, m.filterCase)
				card.FilterInput().Blur()
				m.rebuildFocusedCard()
			case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
				card.StopFiltering()
				m.resizeCards()
			case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+r"))):
				if m.filterMode == components.FilterRegex {
					m.filterMode = components.FilterText
				} else {
					m.filterMode = components.FilterRegex
				}
				card.SetFilter(m.filterMode, m.filterCase)
			case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+i"))):
				m.filterCase = !m.filterCase
				card.SetFilter(m.filterMode, m.filterCase)
			default:
				var cmd tea.Cmd
				fi := card.FilterInput()
				*fi, cmd = fi.Update(msg)
				cmds = append(cmds, cmd)
				card.SetFilter(m.filterMode, m.filterCase)
			}
		}
		return m, tea.Batch(cmds...)
	}

	switch msg := msg.(type) {
	case gossh.LogLineMsg:
		// Find which card this belongs to.
		for i, card := range m.cards {
			if card.Host.Name == msg.Host {
				parsed := m.parse.Parse(msg.Raw, msg.Received)
				m.cards[i].AddLine(parsed)
				break
			}
		}
		// Find the channel for this host and re-issue listen.
		for i, client := range m.clients {
			if client.Host.Name == msg.Host {
				cmds = append(cmds, gossh.ListenForLog(m.logChans[i]))
				break
			}
		}

	case SearchResultMsg:
		m.searchResults[msg.Host] = append(m.searchResults[msg.Host], msg.Lines...)

	case gossh.HostStatusMsg:
		for i, card := range m.cards {
			if card.Host.Name == msg.Host {
				m.cards[i].Status = msg.Status
				if msg.Err != nil {
					clog.Debug("host status error", "host", msg.Host, "err", msg.Err)
				}
				break
			}
		}

	case gossh.ReconnectMsg:
		for i, client := range m.clients {
			if client.Host.Name == msg.Host.Name {
				ctx, cancel := context.WithCancel(context.Background())
				m.ctxs[i] = ctx
				m.cancels[i] = cancel
				ch := m.logChans[i]
				cmds = append(cmds, func() tea.Msg {
					newClient, err := gossh.Connect(ctx, msg.Host)
					if err != nil {
						if msg.Attempt < 10 {
							return gossh.ReconnectMsg{Host: msg.Host, Attempt: msg.Attempt + 1}
						}
						return gossh.HostStatusMsg{Host: msg.Host.Name, Status: gossh.StatusError, Err: err}
					}
					m.clients[i] = newClient
					return gossh.HostStatusMsg{Host: msg.Host.Name, Status: gossh.StatusConnected}
				})
				cmds = append(cmds, gossh.StartTail(ctx, client, m.filePath, ch))
				break
			}
		}

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			m.resizeCards()

		case key.Matches(msg, m.keys.Quit):
			m.shutdown()
			return m, func() tea.Msg { return tea.QuitMsg{} }

		case key.Matches(msg, m.keys.Back):
			m.shutdown()
			return m, func() tea.Msg { return SwitchMsg{To: ScreenFileList, Payload: m.project} }

		case key.Matches(msg, m.keys.FocusNext):
			m.focusedIdx = (m.focusedIdx + 1) % len(m.cards)
			m.updateKeyStates()

		case key.Matches(msg, m.keys.FocusPrev):
			m.focusedIdx = (m.focusedIdx - 1 + len(m.cards)) % len(m.cards)
			m.updateKeyStates()

		case key.Matches(msg, m.keys.Up):
			if len(m.cards) > 0 {
				m.cards[m.focusedIdx].Viewport().ScrollUp(1)
			}

		case key.Matches(msg, m.keys.Down):
			if len(m.cards) > 0 {
				m.cards[m.focusedIdx].Viewport().ScrollDown(1)
			}

		case key.Matches(msg, m.keys.Top):
			if len(m.cards) > 0 {
				m.cards[m.focusedIdx].Viewport().GotoTop()
			}

		case key.Matches(msg, m.keys.Bottom):
			if len(m.cards) > 0 {
				m.cards[m.focusedIdx].Viewport().GotoBottom()
			}

		case key.Matches(msg, m.keys.Pause):
			if len(m.cards) > 0 {
				m.cards[m.focusedIdx].TogglePause()
				m.updateKeyStates()
			}

		case key.Matches(msg, m.keys.Marker):
			if len(m.cards) > 0 {
				m.cards[m.focusedIdx].AddMarker()
				m.updateKeyStates()
			}

		case key.Matches(msg, m.keys.PrevMarker):
			if len(m.cards) > 0 {
				m.cards[m.focusedIdx].JumpToPrevMarker()
			}

		case key.Matches(msg, m.keys.NextMarker):
			if len(m.cards) > 0 {
				m.cards[m.focusedIdx].JumpToNextMarker()
			}

		case key.Matches(msg, m.keys.Filter):
			if len(m.cards) > 0 {
				m.cards[m.focusedIdx].StartFiltering()
				m.resizeCards()
			}

		case key.Matches(msg, m.keys.GlobalFilter):
			m.mode = ModeGlobalFilter
			m.globalInput.Focus()

		case key.Matches(msg, m.keys.LevelDebug):
			m.levelFilter = parser.LevelUnknown
			for i := range m.cards {
				m.cards[i].SetLevelFilter(parser.LevelUnknown)
			}
		case key.Matches(msg, m.keys.LevelInfo):
			m.levelFilter = parser.LevelInfo
			for i := range m.cards {
				m.cards[i].SetLevelFilter(parser.LevelInfo)
			}
		case key.Matches(msg, m.keys.LevelWarn):
			m.levelFilter = parser.LevelWarn
			for i := range m.cards {
				m.cards[i].SetLevelFilter(parser.LevelWarn)
			}
		case key.Matches(msg, m.keys.LevelError):
			m.levelFilter = parser.LevelError
			for i := range m.cards {
				m.cards[i].SetLevelFilter(parser.LevelError)
			}

		case key.Matches(msg, m.keys.Copy):
			if len(m.cards) > 0 {
				// Copy the top visible line of the focused card.
				content := m.cards[m.focusedIdx].Viewport().GetContent()
				lines := strings.SplitN(content, "\n", 2)
				if len(lines) > 0 {
					cmds = append(cmds, tea.SetClipboard(lines[0]))
				}
			}

		case key.Matches(msg, m.keys.Detail):
			if len(m.cards) > 0 {
				content := m.cards[m.focusedIdx].Viewport().GetContent()
				lines := strings.SplitN(content, "\n", 2)
				if len(lines) > 0 {
					line := parser.ParsedLine{Raw: lines[0], Rendered: lines[0]}
					m.detail.Open(line, m.cards[m.focusedIdx].Host.Name)
				}
			}

		case key.Matches(msg, m.keys.Search):
			m.mode = ModeSearch
			m.keys.Search.SetEnabled(false)
			m.searchInput.Reset()
			m.searchInput.Focus()
			m.searchResults = make(map[string][]string)
		}
	}

	// Search mode input handling
	if m.mode == ModeSearch {
		switch msg := msg.(type) {
		case tea.KeyPressMsg:
			switch {
			case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
				m.mode = ModeTail
				m.searchInput.Blur()
				m.searchInput.Reset()
				m.searchResults = make(map[string][]string)
				m.updateKeyStates()
				return m, tea.Batch(cmds...)

			case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
				pattern := m.searchInput.Value()
				if pattern != "" {
					m.searchResults = make(map[string][]string)
					// Cancel previous search contexts
					for _, c := range m.searchCancels {
						c()
					}
					m.searchCancels = nil
					for _, client := range m.clients {
						client := client
						ch := make(chan gossh.LogLineMsg, 1024)
						ctx, cancel := context.WithCancel(context.Background())
						m.searchCancels = append(m.searchCancels, cancel)
						gossh.StartGrep(ctx, client, pattern, m.filePath, ch)
						cmds = append(cmds, collectSearchResults(client.Host.Name, ch))
					}
				}
				return m, tea.Batch(cmds...)

			default:
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				cmds = append(cmds, cmd)
				return m, tea.Batch(cmds...)
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *GridModel) rebuildFocusedCard() {
	// nothing needed; card rebuilds on SetFilter
}

// collectSearchResults drains a grep result channel and returns a SearchResultMsg.
func collectSearchResults(host string, ch chan gossh.LogLineMsg) tea.Cmd {
	return func() tea.Msg {
		var lines []string
		for msg := range ch {
			lines = append(lines, msg.Raw)
		}
		return SearchResultMsg{Host: host, Lines: lines}
	}
}

func (m *GridModel) updateKeyStates() {
	if len(m.cards) == 0 {
		return
	}
	hasMarkers := len(m.cards[m.focusedIdx].Markers()) > 0
	m.keys.PrevMarker.SetEnabled(hasMarkers)
	m.keys.NextMarker.SetEnabled(hasMarkers)
	m.keys.Search.SetEnabled(m.mode == ModeTail)
}

func (m *GridModel) shutdown() {
	for _, cancel := range m.cancels {
		cancel()
	}
	for _, client := range m.clients {
		if client != nil {
			client.Close()
		}
	}
}

func (m GridModel) View() string {
	grid := m.renderGrid()
	if m.detail.Visible {
		return m.detail.Render(m.width, m.height)
	}
	return grid
}

func (m GridModel) renderGrid() string {
	if len(m.cards) == 0 {
		return styles.MutedStyle.Render("No cards to display")
	}

	cols, rows, cardW, cardH := gridDimensions(len(m.cards), m.width, m.height, m.helpBarHeight())

	var rowStrings []string
	for row := 0; row < rows; row++ {
		var cells []string
		for col := 0; col < cols; col++ {
			idx := row*cols + col
			if idx >= len(m.cards) {
				// Empty cell
				cells = append(cells, strings.Repeat("\n", cardH))
				continue
			}
			focused := idx == m.focusedIdx
			cells = append(cells, m.cards[idx].Render(focused, cardW, cardH))
		}
		rowStrings = append(rowStrings, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}

	grid := lipgloss.JoinVertical(lipgloss.Left, rowStrings...)

	// Footer / status bar
	footer := ""
	switch m.mode {
	case ModeGlobalFilter:
		footer = "Global filter: " + m.globalInput.View()
	case ModeSearch:
		resultCount := 0
		for _, lines := range m.searchResults {
			resultCount += len(lines)
		}
		footer = styles.TitleStyle.Render("SEARCH") + "  " + m.searchInput.View()
		if resultCount > 0 {
			footer += "  " + styles.MutedStyle.Render(fmt.Sprintf("(%d results)", resultCount))
		}
		footer += "  " + styles.MutedStyle.Render("[esc] back  [enter] run")
	default:
		footer = m.help.View(m.keys)
	}

	return grid + "\n" + footer
}
