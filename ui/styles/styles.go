package styles

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Colour palette — lipgloss.Color() in v2 is a function returning color.Color.
var (
	ColorPrimary   = lipgloss.Color("#7B61FF")
	ColorSuccess   = lipgloss.Color("#2ECC71")
	ColorWarning   = lipgloss.Color("#F39C12")
	ColorError     = lipgloss.Color("#E74C3C")
	ColorMuted     = lipgloss.Color("#666666")
	ColorHighlight = lipgloss.Color("#F1C40F")
	ColorMarker    = lipgloss.Color("#3498DB")
	ColorBlack     color.Color = lipgloss.Color("#000000")
)

// Card borders.
var (
	CardBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorMuted)

	CardBorderFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorPrimary)
)

// Text styles.
var (
	MatchHighlight = lipgloss.NewStyle().
			Background(ColorHighlight).
			Foreground(ColorBlack)

	MarkerLine = lipgloss.NewStyle().
			Foreground(ColorMarker).
			Bold(true)
)

// Status badge strings.
const (
	BadgeConnected    = "●"
	BadgeConnecting   = "◌"
	BadgeDisconnected = "✗"
)

// Rendered badge styles.
var (
	BadgeConnectedStyle    = lipgloss.NewStyle().Foreground(ColorSuccess)
	BadgeConnectingStyle   = lipgloss.NewStyle().Foreground(ColorWarning)
	BadgeDisconnectedStyle = lipgloss.NewStyle().Foreground(ColorError)
	BadgePausedStyle       = lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)
)

// TitleStyle is used for screen headings.
var TitleStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(ColorPrimary)

// ErrorStyle is used for inline error messages.
var ErrorStyle = lipgloss.NewStyle().Foreground(ColorError)

// MutedStyle is used for secondary text.
var MutedStyle = lipgloss.NewStyle().Foreground(ColorMuted)
