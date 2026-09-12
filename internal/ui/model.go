// Package ui is the Bubble Tea model: a greeting, then a scrolling log of
// events that follows the transcript tail until the user scrolls up.
package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"claude-companion/internal/event"
	"claude-companion/internal/render"
	"claude-companion/internal/transcript"
)

// Config is everything the model needs; nothing is read from the environment.
type Config struct {
	User      string
	SessionID string
	Path      string
	Lines     <-chan transcript.Line
	Loc       *time.Location
}

// Messages the model receives besides Bubble Tea's own.
type (
	lineMsg     transcript.Line   // one transcript line, or a read error
	linesMsg    []transcript.Line // a batch of lines drained from the tailer at once
	tickMsg     time.Time         // reveal one more greeting character
	eofMsg      struct{}          // the tailer closed its channel
	holdDoneMsg struct{}          // the post-greeting hold elapsed; show the log
)

const (
	tickEvery = 40 * time.Millisecond  // one greeting character per tick
	holdAfter = 700 * time.Millisecond // the full greeting block stays alone this long
)

// Model is the whole UI state. Fields are unexported; tests live in-package.
type Model struct {
	cfg      Config
	vp       viewport.Model
	pairer   *event.Pairer
	events   []event.Event
	rendered [][]string
	shades   []int           // per event: 0 dark, 1 light; alternates between consecutive same-type events
	flat     []string        // greeting + rendered, flattened; rebuilt only when dirty
	dirty    bool            // flat must be rebuilt (replacement, resize, greeting change)
	index    map[string]int  // event id -> position, for in-place replacement
	tally    map[string]mark // event id -> glyph and marks, for the footer; reset by a prompt
	width    int
	height   int
	follow   bool
	revealed int
	held     bool // the post-greeting hold has elapsed; the log may show
	cleared  bool // a prompt has cleared the screen; the greeting is gone for good
	started  time.Time
	cwd      string
	lastErr  string
	eof      bool
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
	return tea.Batch(waitLines(m.cfg.Lines), tea.Tick(tickEvery, func(t time.Time) tea.Msg { return tickMsg(t) }))
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

func (m Model) greetingLen() int { return len([]rune("hello, " + m.cfg.User)) }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.vp.SetWidth(msg.Width)
		m.vp.SetHeight(max(msg.Height-1, 1)) // one line for the footer
		m.rerenderAll()
		return m, nil

	case tickMsg:
		if m.revealed >= m.greetingLen() {
			return m, nil
		}
		m.revealed++
		m.dirty = true
		m.refresh()
		if m.revealed >= m.greetingLen() {
			return m, tea.Tick(holdAfter, func(time.Time) tea.Msg { return holdDoneMsg{} })
		}
		return m, tea.Tick(tickEvery, func(t time.Time) tea.Msg { return tickMsg(t) })

	case holdDoneMsg:
		m.held = true
		m.dirty = true
		m.refresh()
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
	if m.started.IsZero() {
		m.started, m.cwd = firstEntryInfo(line)
		m.dirty = true // the greeting block changed
	}
	evs, err := m.pairer.Feed(line)
	if err != nil {
		return
	}
	for _, e := range evs {
		m.count(e)
		if _, isPrompt := e.(event.Prompt); isPrompt {
			// A new prompt starts a new screen: everything before it goes,
			// including the greeting.
			m.events, m.rendered, m.shades, m.flat = nil, nil, nil, nil
			m.index = map[string]int{}
			m.tally = map[string]mark{}
			m.cleared, m.dirty = true, true
		}
		if i, ok := m.index[e.EventID()]; ok {
			m.events[i], m.rendered[i] = e, render.Event(e, m.width, m.cfg.Loc, m.shades[i])
			m.dirty = true
			continue
		}
		shade := 0
		if n := len(m.events); n > 0 && sameType(m.events[n-1], e) {
			shade = 1 - m.shades[n-1]
		}
		lines := render.Event(e, m.width, m.cfg.Loc, shade)
		m.index[e.EventID()] = len(m.events)
		m.events = append(m.events, e)
		m.rendered = append(m.rendered, lines)
		m.shades = append(m.shades, shade)
		if !m.dirty {
			m.flat = append(m.flat, lines...)
		}
	}
}

func (m *Model) rerenderAll() {
	for i, e := range m.events {
		m.rendered[i] = render.Event(e, m.width, m.cfg.Loc, m.shades[i])
	}
	m.dirty = true
	m.refresh()
}

// refresh hands the flattened lines to the viewport and keeps the tail in
// view when following. The flat slice is rebuilt only when something other
// than an append happened.
func (m *Model) refresh() {
	if !m.held { // the log appears only after the greeting is typed and held for a beat
		m.vp.SetContentLines(m.greetingLines(true))
		return
	}
	if m.dirty {
		m.flat = m.flat[:0]
		m.flat = append(m.flat, m.greetingLines(false)...)
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

// greetingLines renders the greeting. During the reveal and hold it always
// shows (force); afterwards it is gone once a prompt has cleared the screen.
func (m Model) greetingLines(force bool) []string {
	if m.cleared && !force {
		return nil
	}
	lines := render.Greeting(m.cfg.User, m.cfg.SessionID, m.cwd, m.started, m.cfg.Loc, m.revealed)
	if m.revealed >= m.greetingLen() {
		lines = append(lines, "")
	}
	return lines
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
	line := strings.Join(parts, footerStyle.Render("  ·  "))
	if m.lastErr != "" {
		line += "  " + footerWarn.Render(m.lastErr)
	} else if m.eof {
		line += "  " + footerWarn.Render("stream closed")
	}
	return line
}

func (m Model) View() tea.View {
	v := tea.NewView(m.vp.View() + "\n" + m.footer())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeNone // no mouse reporting: the terminal keeps text selection; wheel arrives as arrow keys
	v.WindowTitle = "claude-companion " + m.cfg.SessionID
	return v
}

// sameType reports whether two events share a bar hue: commands (and
// rejected commands) together, file changes (and rejected edits) together.
func sameType(a, b event.Event) bool {
	return fmt.Sprintf("%T", a) == fmt.Sprintf("%T", b)
}
