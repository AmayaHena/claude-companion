// Package ui is the Bubble Tea model: a scrolling log of events that
// follows the transcript tail until the user scrolls up.
package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi" // the test package already names a regexp ansi

	"github.com/AmayaHena/claude-companion/internal/event"
	"github.com/AmayaHena/claude-companion/internal/render"
	"github.com/AmayaHena/claude-companion/internal/transcript"
)

// Config is everything the model needs; nothing is read from the environment.
type Config struct {
	SessionID string
	Path      string
	Lines     <-chan transcript.Line
	Loc       *time.Location
}

// Messages the model receives besides Bubble Tea's own.
type (
	lineMsg  transcript.Line   // one transcript line, or a read error
	linesMsg []transcript.Line // a batch of lines drained from the tailer at once
	eofMsg   struct{}          // the tailer closed its channel
)

// Model is the whole UI state. Fields are unexported; tests live in-package.
type Model struct {
	cfg      Config
	vp       viewport.Model
	pairer   *event.Pairer
	events   []event.Event
	rendered [][]string
	flat     []string        // rendered, flattened; rebuilt only when dirty
	dirty    bool            // flat must be rebuilt (replacement, resize, clear)
	index    map[string]int  // event id -> position, for in-place replacement
	tally    map[string]mark // event id -> glyph and marks, for the footer; reset by a prompt
	width    int
	height   int
	follow   bool
	lastErr  string
	eof      bool
	// copy target: 0 is the latest command, k the k-th before it; copied and
	// notice are footer feedback, cleared by the next new event.
	copyBack int
	copied   int // lines copied by the last c, 0 when nothing was copied since the last event
	notice   string
}

func New(cfg Config) Model {
	if cfg.Loc == nil {
		cfg.Loc = time.Local
	}
	return Model{
		cfg:    cfg,
		vp:     viewport.New(viewport.WithWidth(80), viewport.WithHeight(24)),
		pairer: event.NewPairer(),
		index:  map[string]int{},
		tally:  map[string]mark{},
		width:  80,
		height: 24,
		follow: true,
	}
}

func (m Model) Init() tea.Cmd {
	return waitLines(m.cfg.Lines)
}

// maxBatch bounds how many lines one message may carry, so a huge backlog is
// rendered in slices and the UI stays responsive.
const maxBatch = 256

// waitLines blocks for one line, then drains whatever else is already
// available so a backlog is ingested with one refresh instead of one per line.
func waitLines(ch <-chan transcript.Line) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		first, ok := <-ch
		if !ok {
			return eofMsg{}
		}
		batch := linesMsg{first}
		for len(batch) < maxBatch {
			select {
			case l, ok := <-ch:
				if !ok {
					return batch // eofMsg follows on the next wait
				}
				batch = append(batch, l)
			default:
				return batch
			}
		}
		return batch
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.vp.SetWidth(msg.Width)
		m.vp.SetHeight(max(msg.Height-1, 1)) // one line for the footer
		m.rerenderAll()
		return m, nil

	case lineMsg:
		m.take(transcript.Line(msg))
		m.refresh()
		return m, waitLines(m.cfg.Lines)

	case linesMsg:
		for _, l := range msg {
			m.take(l)
		}
		m.refresh()
		return m, waitLines(m.cfg.Lines)

	case eofMsg:
		m.eof = true
		m.refresh()
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "home":
			m.vp.GotoTop()
			m.follow = false
			return m, nil
		case "end":
			m.vp.GotoBottom()
			m.follow = true
			return m, nil
		case "c":
			return m.copyTarget()
		case "tab":
			if cmds := m.commands(); m.copyBack < len(cmds)-1 {
				m.copyBack++
			}
			m.copied = 0
			return m, nil
		case "shift+tab":
			if m.copyBack > 0 {
				m.copyBack--
			}
			m.copied = 0
			return m, nil
		}
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		m.follow = m.vp.AtBottom()
		return m, cmd

	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		m.follow = m.vp.AtBottom()
		return m, cmd
	}
	return m, nil
}

// take records an error line or ingests a text line.
func (m *Model) take(l transcript.Line) {
	if l.Err != nil {
		m.lastErr = l.Err.Error()
		return
	}
	m.ingest(l.Text)
}

// ingest feeds one transcript line and folds the resulting events into the
// log, replacing a running command in place when its result arrives.
func (m *Model) ingest(line string) {
	evs, err := m.pairer.Feed(line)
	if err != nil {
		return
	}
	for _, e := range evs {
		m.count(e)
		if _, isPrompt := e.(event.Prompt); isPrompt {
			// A new prompt starts a new screen: everything before it goes.
			m.events, m.rendered, m.flat = nil, nil, nil
			m.index = map[string]int{}
			m.tally = map[string]mark{}
			m.dirty = true
		}
		if i, ok := m.index[e.EventID()]; ok {
			m.events[i], m.rendered[i] = e, render.Event(e, m.width, m.cfg.Loc)
			m.dirty = true
			continue
		}
		lines := render.Event(e, m.width, m.cfg.Loc)
		m.copyBack, m.copied, m.notice = 0, 0, "" // a new event retargets the copy key
		m.index[e.EventID()] = len(m.events)
		m.events = append(m.events, e)
		m.rendered = append(m.rendered, lines)
		if !m.dirty {
			m.flat = append(m.flat, lines...)
		}
	}
}

func (m *Model) rerenderAll() {
	for i, e := range m.events {
		m.rendered[i] = render.Event(e, m.width, m.cfg.Loc)
	}
	m.dirty = true
	m.refresh()
}

// refresh hands the flattened lines to the viewport and keeps the tail in
// view when following. The flat slice is rebuilt only when something other
// than an append happened.
func (m *Model) refresh() {
	if m.dirty {
		m.flat = m.flat[:0]
		for _, lines := range m.rendered {
			m.flat = append(m.flat, lines...)
		}
		m.dirty = false
	}
	m.vp.SetContentLines(m.flat)
	if m.follow {
		m.vp.GotoBottom()
	}
}

var (
	footerStyle = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
	footerWarn  = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
	footerNum   = lipgloss.NewStyle().Foreground(lipgloss.White) // the tally numbers, readable against the grey
	// markStyles colour a mark's count like the event it counts: git writes
	// red, agents blue, skills green; the network count stays white.
	markStyles = map[string]lipgloss.Style{
		"⚠️":           lipgloss.NewStyle().Foreground(lipgloss.Red),
		"🤖":            lipgloss.NewStyle().Foreground(lipgloss.Blue),
		"\u2139\ufe0f": lipgloss.NewStyle().Foreground(lipgloss.Green),
	}
)

// mark is what the footer remembers about one event: its type glyph and,
// for a command, its warning and network marks.
type mark struct {
	glyph     string
	warn, net bool
}

// count records an event in the tally, by id so a running command replaced
// by its result is counted once, at its final state.
func (m *Model) count(e event.Event) {
	g := render.Glyph(e)
	if g == "" { // prompts are not actions
		return
	}
	mk := mark{glyph: g}
	if c, ok := e.(event.Command); ok {
		mk.warn, mk.net = render.Marks(c.Cmd)
	}
	m.tally[e.EventID()] = mk
}

// actionGlyphs are the type glyphs shown as a share of all actions, in order.
var actionGlyphs = []string{"⚒\ufe0f", "📁", "🆕", "❗"}

// footer is the single status line under the log: the share of each action
// type since the last prompt, then plain counts of the warning, network,
// agent and skill marks. Zero entries are omitted.
func (m Model) footer() string {
	counts := map[string]int{}
	total := 0
	for _, mk := range m.tally {
		counts[mk.glyph]++
		if mk.warn {
			counts["⚠️"]++
		}
		if mk.net {
			counts["🛜"]++
		}
	}
	for _, g := range actionGlyphs {
		total += counts[g]
	}
	var parts []string
	if total == 0 {
		parts = append(parts, footerStyle.Render("0 actions"))
	}
	for _, g := range actionGlyphs {
		if n := counts[g]; n > 0 {
			parts = append(parts, g+" "+footerNum.Render(fmt.Sprintf("%d%%", (n*100+total/2)/total)))
		}
	}
	for _, g := range []string{"⚠️", "🛜", "🤖", "\u2139\ufe0f"} {
		if n := counts[g]; n > 0 {
			style, ok := markStyles[g]
			if !ok {
				style = footerNum
			}
			parts = append(parts, g+" "+style.Render(fmt.Sprintf("%d", n)))
		}
	}
	if n := m.pairer.Skipped(); n > 0 {
		parts = append(parts, footerStyle.Render(fmt.Sprintf("%d skipped", n)))
	}
	switch {
	case m.notice != "":
		parts = append(parts, footerStyle.Render(m.notice))
	case m.copied > 1:
		parts = append(parts, footerNum.Render(fmt.Sprintf("copied %d lines", m.copied)))
	case m.copied == 1:
		parts = append(parts, footerNum.Render("copied"))
	case m.copyBack > 0:
		if cmds := m.commands(); m.copyBack < len(cmds) {
			c := cmds[len(cmds)-1-m.copyBack]
			parts = append(parts, footerStyle.Render("copy → ")+footerNum.Render(xansi.Truncate(render.Sanitize(firstLineOf(c.Cmd)), 30, "…")))
		}
	}
	line := strings.Join(parts, footerStyle.Render("  ·  "))
	if m.lastErr != "" {
		line += "  " + footerWarn.Render(render.Sanitize(m.lastErr)) // error text embeds a path from disk
	} else if m.eof {
		line += "  " + footerWarn.Render("stream closed")
	}
	return line
}

func (m Model) View() tea.View {
	v := tea.NewView(m.vp.View() + "\n" + m.footer())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeNone                                        // no mouse reporting: the terminal keeps text selection; wheel arrives as arrow keys
	v.WindowTitle = "claude-companion " + render.Sanitize(m.cfg.SessionID) // a filename, written raw into OSC 2 by Bubble Tea
	return v
}

// commands returns the commands of the current block, oldest first.
func (m Model) commands() []event.Command {
	var out []event.Command
	for _, e := range m.events {
		if c, ok := e.(event.Command); ok {
			out = append(out, c)
		}
	}
	return out
}

// copyTarget sends the targeted command to the clipboard (OSC 52, handled
// by Bubble Tea) or explains in the footer why nothing was copied.
func (m Model) copyTarget() (tea.Model, tea.Cmd) {
	cmds := m.commands()
	if len(cmds) == 0 {
		m.notice = "nothing to copy"
		return m, nil
	}
	if m.copyBack >= len(cmds) {
		m.copyBack = len(cmds) - 1
	}
	// The clipboard gets the whole command (the screen shows its first line
	// only), sanitised with its newlines and tabs kept; the footer says how
	// many lines went out so a multi-line copy is never silent.
	text := render.SanitizeText(cmds[len(cmds)-1-m.copyBack].Cmd)
	m.copied, m.notice = strings.Count(text, "\n")+1, ""
	return m, tea.SetClipboard(text)
}

func firstLineOf(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
