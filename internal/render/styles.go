package render

import "charm.land/lipgloss/v2"

// Colours are terminal palette indices, so the active terminal theme decides
// the actual shades. Never hex.
// Each type of action has its own hue, in a dark and a light shade. The
// caller alternates the shade between consecutive actions of the same type,
// so neighbours differ while the hue still names the type. Yellow, magenta
// and cyan are far apart from each other and from the red of failures.
var (
	accentCommand = [2]lipgloss.Style{lipgloss.NewStyle().Foreground(lipgloss.Yellow), lipgloss.NewStyle().Foreground(lipgloss.BrightYellow)}
	accentFile    = [2]lipgloss.Style{lipgloss.NewStyle().Foreground(lipgloss.Magenta), lipgloss.NewStyle().Foreground(lipgloss.BrightMagenta)}
)

// styleWarn marks a command that writes git state (add, commit, push):
// underlined red over the whole command text, no syntax colouring.
var styleWarn = lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Red)

// bar is U+258E LEFT ONE QUARTER BLOCK, one cell wide.
const bar = "▎"

var (
	styleTime    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.BrightBlack) // bold faded grey
	styleCmd     = lipgloss.NewStyle().Bold(true)
	styleOut     = lipgloss.NewStyle().Faint(true)
	styleErr     = lipgloss.NewStyle().Foreground(lipgloss.Red)
	styleMeta    = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
	stylePath    = lipgloss.NewStyle().Foreground(lipgloss.Blue).Bold(true)
	styleHunk    = lipgloss.NewStyle().Foreground(lipgloss.Cyan).Faint(true)
	styleSignDel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Red)   // the "-" of a removed line
	styleSignAdd = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Green) // the "+" of an added line
	stylePrompt  = lipgloss.NewStyle().Bold(true)
	styleHello   = lipgloss.NewStyle().Foreground(lipgloss.Blue).Bold(true)
	styleLabel   = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
	styleValue   = lipgloss.NewStyle()
	styleWarning = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
)

const (
	glyphCommand = "⚒\ufe0f" // U+2692 + VS16: measured and drawn as two cells like the others
	glyphEdit    = "📁"
	glyphCreate  = "🆕"
	glyphAlert   = "❗"
	glyphWarn    = "⚠️" // U+26A0 + VS16, two cells
)
