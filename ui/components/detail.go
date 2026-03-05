package components

import (
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/fluid-movement/log-tui/parser"
	"github.com/fluid-movement/log-tui/ui/styles"
)

// ─── Key map ────────────────────────────────────────────────────────────────

type detailKeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Copy     key.Binding
	CopyJSON key.Binding
	Close    key.Binding
}

func (k detailKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Copy, k.CopyJSON, k.Close}
}
func (k detailKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down},
		{k.Copy, k.CopyJSON, k.Close},
	}
}

var defaultDetailKeys = detailKeyMap{
	Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "scroll up")),
	Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "scroll down")),
	Copy:     key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy raw")),
	CopyJSON: key.NewBinding(key.WithKeys("Y"), key.WithHelp("Y", "copy JSON")),
	Close:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
}

// ─── Overlay ──────────────────────────────────────────────────────────────────

// DetailOverlay shows a single log line in detail.
type DetailOverlay struct {
	Visible  bool
	line     parser.ParsedLine
	host     string
	viewport viewport.Model
	keys     detailKeyMap
	help     help.Model
}

func NewDetailOverlay() DetailOverlay {
	return DetailOverlay{
		keys: defaultDetailKeys,
		help: help.New(),
	}
}

func (d *DetailOverlay) Open(line parser.ParsedLine, host string) {
	d.Visible = true
	d.line = line
	d.host = host
	d.viewport = viewport.New()
	d.viewport.SoftWrap = true
	d.buildContent()
}

func (d *DetailOverlay) Close() {
	d.Visible = false
}

func (d *DetailOverlay) buildContent() {
	var b strings.Builder

	b.WriteString(styles.MutedStyle.Render("Host: "+d.host) + "\n")
	b.WriteString(styles.MutedStyle.Render("Time: "+d.line.Received.Format("2006-01-02 15:04:05.000")) + "\n\n")

	if d.line.IsJSON {
		b.WriteString(parser.PrettyRenderJSON(d.line.Raw, d.line.JSONData))
	} else {
		b.WriteString(d.line.Raw)
	}

	d.viewport.SetContent(b.String())
}

func (d *DetailOverlay) Update(msg tea.Msg) tea.Cmd {
	if !d.Visible {
		return nil
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, d.keys.Close):
			d.Close()
			return nil

		case key.Matches(msg, d.keys.Up):
			d.viewport.ScrollUp(1)

		case key.Matches(msg, d.keys.Down):
			d.viewport.ScrollDown(1)

		case key.Matches(msg, d.keys.Copy):
			return tea.SetClipboard(d.line.Raw)

		case key.Matches(msg, d.keys.CopyJSON):
			content := d.line.Raw
			if d.line.IsJSON {
				content = parser.PrettyRenderJSON(d.line.Raw, d.line.JSONData)
			}
			return tea.SetClipboard(content)
		}
	}
	return nil
}

func (d *DetailOverlay) Render(termW, termH int) string {
	overlayW := termW * 3 / 4
	overlayH := termH * 3 / 4
	if overlayW < 40 {
		overlayW = 40
	}
	if overlayH < 10 {
		overlayH = 10
	}

	d.viewport.SetWidth(overlayW - 4)
	d.viewport.SetHeight(overlayH - 4)
	d.help.SetWidth(overlayW - 4)

	footer := d.help.View(d.keys)
	content := d.viewport.View() + "\n" + footer

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorPrimary).
		Width(overlayW - 2).
		Height(overlayH - 2).
		Render(content)

	return lipgloss.Place(termW, termH, lipgloss.Center, lipgloss.Center, box)
}
