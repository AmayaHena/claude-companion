package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi" // the test package already names a regexp ansi

	"github.com/AmayaHena/claude-companion/internal/render"
	"github.com/AmayaHena/claude-companion/internal/transcript"
)

// Picker lists recent sessions and lets the user open one. It runs as its
// own program before the viewer; Chosen is set when enter was pressed.
type Picker struct {
	sessions []transcript.Session
	now      time.Time
	cursor   int
	width    int
	Chosen   *transcript.Session
}

func NewPicker(sessions []transcript.Session, now time.Time) Picker {
	return Picker{sessions: sessions, now: now, width: 80}
}

func (p Picker) Init() tea.Cmd { return nil }

func (p Picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.width = msg.Width
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return p, tea.Quit
		case "up", "k":
			if p.cursor > 0 {
				p.cursor--
			}
		case "down", "j":
			if p.cursor < len(p.sessions)-1 {
				p.cursor++
			}
		case "enter":
			if len(p.sessions) > 0 {
				s := p.sessions[p.cursor]
				p.Chosen = &s
				return p, tea.Quit
			}
		}
	}
	return p, nil
}

var (
	pickerTitle  = lipgloss.NewStyle().Foreground(lipgloss.Blue).Bold(true)
	pickerID     = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
	pickerAgo    = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
	pickerDir    = lipgloss.NewStyle().Foreground(lipgloss.Cyan)
	pickerPrompt = lipgloss.NewStyle()
	pickerEmpty  = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
	pickerCursor = lipgloss.NewStyle().Bold(true)
)

// ago renders a duration since now in the coarsest unit that is not zero.
func ago(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d/time.Minute))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h ago", int(d/time.Hour))
	}
	return fmt.Sprintf("%d d ago", int(d/(24*time.Hour)))
}

func (p Picker) View() tea.View {
	var b strings.Builder
	b.WriteString(pickerTitle.Render("recent sessions") + "\n\n")
	if len(p.sessions) == 0 {
		b.WriteString(pickerEmpty.Render("no sessions found") + "\n")
	}
	for i, s := range p.sessions {
		mark := "  "
		if i == p.cursor {
			mark = pickerCursor.Render("▸ ")
		}
		id := render.Sanitize(s.ID) // a filename from disk
		if r := []rune(id); len(r) > 8 {
			id = string(r[:8])
		}
		prompt := render.Sanitize(s.FirstPrompt) // transcript text is untrusted
		if prompt == "" {
			prompt = pickerEmpty.Render("(no prompt yet)")
		} else {
			prompt = pickerPrompt.Render(prompt)
		}
		dir := ""
		if s.Cwd != "" { // filepath.Base("") would be "."
			dir = render.Sanitize(filepath.Base(s.Cwd))
		}
		line := mark + pickerID.Render(id) + "  " + pickerAgo.Render(fmt.Sprintf("%-11s", ago(p.now, s.ModTime))) + "  " + pickerDir.Render(dir) + "  " + prompt
		b.WriteString(xansi.Truncate(line, p.width, "…") + "\n")
	}
	b.WriteString("\n" + pickerEmpty.Render("↑ ↓ move  ·  enter open  ·  q quit"))
	v := tea.NewView(b.String())
	v.AltScreen = true
	v.WindowTitle = "claude-companion"
	return v
}
