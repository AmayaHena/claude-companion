package render

import "charm.land/lipgloss/v2"

// Colours are terminal palette indices, so the active terminal theme decides
// the actual shades. Never hex.
var (
	styleTime    = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
	styleCmd     = lipgloss.NewStyle().Bold(true)
	styleOut     = lipgloss.NewStyle().Faint(true)
	styleErr     = lipgloss.NewStyle().Foreground(lipgloss.Red)
	styleMeta    = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
	stylePath    = lipgloss.NewStyle().Foreground(lipgloss.Blue).Bold(true)
	styleHunk    = lipgloss.NewStyle().Foreground(lipgloss.Cyan).Faint(true)
	styleDel     = lipgloss.NewStyle().Foreground(lipgloss.Red)
	styleAdd     = lipgloss.NewStyle().Foreground(lipgloss.Green)
	styleCtx     = lipgloss.NewStyle()
	stylePrompt  = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
	styleHello   = lipgloss.NewStyle().Foreground(lipgloss.Blue).Bold(true)
	styleLabel   = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
	styleValue   = lipgloss.NewStyle()
	styleWarning = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
)

const (
	glyphCommand = "⚒"
	glyphEdit    = "📁"
	glyphCreate  = "🆕"
	glyphAlert   = "❗"
)
