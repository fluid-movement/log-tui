package components

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/fluid-movement/log-tui/config"
	gossh "github.com/fluid-movement/log-tui/ssh"
	"github.com/fluid-movement/log-tui/parser"
	"github.com/fluid-movement/log-tui/ui/styles"
)

const maxLines = 2000

// ─── Filter ──────────────────────────────────────────────────────────────────

type FilterMode int

const (
	FilterNone FilterMode = iota
	FilterText
	FilterRegex
)

type FilterState struct {
	Mode       FilterMode
	Pattern    string
	Compiled   *regexp.Regexp
	Exclude    bool
	IgnoreCase bool
}

func (f FilterState) Active() bool {
	return f.Mode != FilterNone && f.Pattern != ""
}

func (f FilterState) Matches(line string) bool {
	if !f.Active() {
		return true
	}
	target := line
	if f.IgnoreCase {
		target = strings.ToLower(line)
	}
	pat := f.Pattern
	if f.IgnoreCase {
		pat = strings.ToLower(pat)
	}
	var match bool
	if f.Mode == FilterRegex && f.Compiled != nil {
		match = f.Compiled.MatchString(target)
	} else {
		match = strings.Contains(target, pat)
	}
	if f.Exclude {
		return !match
	}
	return match
}

func (f FilterState) Highlight(line string, matchStyle lipgloss.Style) string {
	if !f.Active() || f.Exclude {
		return line
	}
	if f.Mode == FilterRegex && f.Compiled != nil {
		return f.Compiled.ReplaceAllStringFunc(line, func(m string) string {
			return matchStyle.Render(m)
		})
	}
	pat := f.Pattern
	target := line
	if f.IgnoreCase {
		pat = strings.ToLower(pat)
		target = strings.ToLower(line)
	}
	idx := strings.Index(target, pat)
	if idx == -1 {
		return line
	}
	return line[:idx] + matchStyle.Render(line[idx:idx+len(pat)]) + line[idx+len(pat):]
}

// NewFilterState parses a pattern string into a FilterState.
func NewFilterState(pattern string, mode FilterMode, ignoreCase bool) FilterState {
	exclude := strings.HasPrefix(pattern, "!")
	if exclude {
		pattern = pattern[1:]
	}
	f := FilterState{
		Mode:       mode,
		Pattern:    pattern,
		Exclude:    exclude,
		IgnoreCase: ignoreCase,
	}
	if mode == FilterRegex && pattern != "" {
		flags := ""
		if ignoreCase {
			flags = "(?i)"
		}
		f.Compiled, _ = regexp.Compile(flags + pattern)
	}
	return f
}

// ─── ServerCard ───────────────────────────────────────────────────────────────

type ServerCard struct {
	Host      config.Host
	viewport  viewport.Model
	Status    gossh.HostStatus
	Filter       FilterState
	globalFilter FilterState
	levelFilter  parser.Level
	lines        []parser.ParsedLine
	paused       bool
	pauseBuf     []parser.ParsedLine
	following    bool
	markers      []int // line indices
	filterIn     textinput.Model
	filtering    bool
	lastRecv     time.Time
}

func NewServerCard(host config.Host) ServerCard {
	vp := viewport.New(viewport.WithWidth(40), viewport.WithHeight(20))
	vp.SoftWrap = true

	fi := textinput.New()
	fi.Placeholder = "filter…"

	return ServerCard{
		Host:      host,
		viewport:  vp,
		Status:    gossh.StatusConnecting,
		following: true,
		filterIn:  fi,
	}
}

func (c *ServerCard) SetSize(w, h int) {
	headerH := 1
	filterH := 0
	if c.filtering {
		filterH = 1
	}
	c.viewport.SetWidth(w)
	c.viewport.SetHeight(h - headerH - filterH)
}

// AddLine adds a new line to the card respecting the ring buffer and pause.
func (c *ServerCard) AddLine(line parser.ParsedLine) {
	c.lastRecv = line.Received
	if c.paused {
		c.pauseBuf = append(c.pauseBuf, line)
		return
	}
	if len(c.lines) >= maxLines {
		c.lines = c.lines[1:]
	}
	c.lines = append(c.lines, line)
	c.rebuildViewport()
	if c.following {
		c.viewport.GotoBottom()
	}
}

// Pause stops auto-scrolling and buffers incoming lines.
func (c *ServerCard) Pause() {
	c.paused = true
	c.following = false
}

// Resume flushes the pause buffer and resumes following.
func (c *ServerCard) Resume() {
	c.paused = false
	// Flush pauseBuf into lines, honouring maxLines.
	for _, line := range c.pauseBuf {
		if len(c.lines) >= maxLines {
			c.lines = c.lines[1:]
		}
		c.lines = append(c.lines, line)
	}
	c.pauseBuf = nil
	c.following = true
	c.rebuildViewport()
	c.viewport.GotoBottom()
}

// TogglePause flips between paused and running.
func (c *ServerCard) TogglePause() {
	if c.paused {
		c.Resume()
	} else {
		c.Pause()
	}
}

// AddMarker marks the current viewport bottom as a marker.
func (c *ServerCard) AddMarker() {
	idx := len(c.lines) - 1
	if idx < 0 {
		return
	}
	c.markers = append(c.markers, idx)
	c.rebuildViewport()
}

// JumpToPrevMarker scrolls to the previous marker.
func (c *ServerCard) JumpToPrevMarker() {
	if len(c.markers) == 0 {
		return
	}
	cur := c.viewport.YOffset()
	for i := len(c.markers) - 1; i >= 0; i-- {
		lineOffset := c.markerViewportOffset(c.markers[i])
		if lineOffset < cur {
			c.viewport.SetYOffset(lineOffset)
			return
		}
	}
}

// JumpToNextMarker scrolls to the next marker.
func (c *ServerCard) JumpToNextMarker() {
	if len(c.markers) == 0 {
		return
	}
	cur := c.viewport.YOffset()
	for _, m := range c.markers {
		lineOffset := c.markerViewportOffset(m)
		if lineOffset > cur {
			c.viewport.SetYOffset(lineOffset)
			return
		}
	}
}

func (c *ServerCard) markerViewportOffset(lineIdx int) int {
	// Each marker adds one line before it in the viewport.
	offset := lineIdx
	for _, m := range c.markers {
		if m < lineIdx {
			offset++
		}
	}
	return offset
}

// StartFiltering activates the filter input.
func (c *ServerCard) StartFiltering() {
	c.filtering = true
	c.filterIn.Focus()
	c.SetSize(c.viewport.Width(), c.viewport.Height()+1) // give back the filter row
}

// StopFiltering hides the filter input and clears the filter.
func (c *ServerCard) StopFiltering() {
	c.filtering = false
	c.filterIn.Reset()
	c.filterIn.Blur()
	c.Filter = FilterState{}
	c.rebuildViewport()
}

// SetFilter applies a new filter from the current input text.
func (c *ServerCard) SetFilter(mode FilterMode, ignoreCase bool) {
	c.Filter = NewFilterState(c.filterIn.Value(), mode, ignoreCase)
	c.rebuildViewport()
}

// Viewport returns the viewport for scrolling from the grid.
func (c *ServerCard) Viewport() *viewport.Model { return &c.viewport }

// Markers returns the marker list (used to enable/disable the key binding).
func (c *ServerCard) Markers() []int { return c.markers }

// Paused returns whether the card is paused.
func (c *ServerCard) Paused() bool { return c.paused }

// FilterInput returns the filter textinput for propagating key events.
func (c *ServerCard) FilterInput() *textinput.Model { return &c.filterIn }

// Filtering returns whether filter mode is active.
func (c *ServerCard) Filtering() bool { return c.filtering }

// Lines returns the buffered log lines (read-only view for testing/inspection).
func (c *ServerCard) Lines() []parser.ParsedLine { return c.lines }

// SetGlobalFilter updates the global filter and rebuilds the viewport.
func (c *ServerCard) SetGlobalFilter(f FilterState) {
	c.globalFilter = f
	c.rebuildViewport()
}

// SetLevelFilter sets the minimum level and rebuilds.
func (c *ServerCard) SetLevelFilter(l parser.Level) {
	c.levelFilter = l
	c.rebuildViewport()
}

func (c *ServerCard) rebuildViewport() {
	markerSet := make(map[int]bool)
	for _, m := range c.markers {
		markerSet[m] = true
	}

	var viewLines []string
	for i, line := range c.lines {
		if markerSet[i] {
			ts := ""
			if !line.Received.IsZero() {
				ts = line.Received.Format("15:04:05")
			}
			rule := styles.MarkerLine.Render(
				fmt.Sprintf("──── ◆ %s ────", ts),
			)
			viewLines = append(viewLines, rule)
		}

		// Level filter (no-op when LevelUnknown)
		if c.levelFilter != parser.LevelUnknown && line.Level < c.levelFilter {
			continue
		}

		// Per-card filter
		if !c.Filter.Matches(line.Raw) {
			continue
		}

		// Global filter
		if !c.globalFilter.Matches(line.Raw) {
			continue
		}

		timestamp := line.Received.Format("15:04:05.000")
		rendered := c.Filter.Highlight(line.Rendered, styles.MatchHighlight)
		rendered = c.globalFilter.Highlight(rendered, styles.MatchHighlight)
		viewLines = append(viewLines, timestamp+"  "+rendered)
	}

	c.viewport.SetContent(strings.Join(viewLines, "\n"))
}

// Render renders the card as a string.
func (c *ServerCard) Render(focused bool, w, h int) string {
	borderStyle := styles.CardBorder
	if focused {
		borderStyle = styles.CardBorderFocused
	}
	// Account for border (2 cols, 2 rows)
	innerW := w - 2
	innerH := h - 2

	header := c.renderHeader(innerW)
	var filterBar string
	if c.filtering {
		filterBar = c.filterIn.View() + "\n"
		innerH--
	}

	viewH := innerH - 1 // header row
	if c.filtering {
		viewH--
	}
	c.viewport.SetWidth(innerW)
	c.viewport.SetHeight(viewH)

	content := header + "\n" + filterBar + c.viewport.View()
	return borderStyle.Width(innerW).Height(innerH).Render(content)
}

func (c *ServerCard) renderHeader(w int) string {
	badge := ""
	switch c.Status {
	case gossh.StatusConnecting, gossh.StatusReconnecting:
		badge = styles.BadgeConnectingStyle.Render(styles.BadgeConnecting)
	case gossh.StatusConnected:
		badge = styles.BadgeConnectedStyle.Render(styles.BadgeConnected)
	case gossh.StatusError, gossh.StatusUnresolvable:
		badge = styles.BadgeDisconnectedStyle.Render(styles.BadgeDisconnected)
	}

	hostname := c.Host.Name
	var extras []string
	if c.paused {
		extras = append(extras, styles.BadgePausedStyle.Render("[PAUSED]"))
	}
	if c.Filter.Active() {
		extras = append(extras, styles.MutedStyle.Render(fmt.Sprintf("[FILTER: %s]", c.Filter.Pattern)))
	}
	lineCount := styles.MutedStyle.Render(fmt.Sprintf("%d lines", len(c.lines)))
	lastRecv := ""
	if !c.lastRecv.IsZero() {
		lastRecv = styles.MutedStyle.Render(c.lastRecv.Format("15:04:05"))
	}

	left := badge + " " + hostname
	if len(extras) > 0 {
		left += " " + strings.Join(extras, " ")
	}
	right := lineCount
	if lastRecv != "" {
		right += "  " + lastRecv
	}

	// Pad to fill width
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	pad := w - leftW - rightW
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right
}
