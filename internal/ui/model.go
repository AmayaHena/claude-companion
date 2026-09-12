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
	flat     []string       // greeting + rendered, flattened; rebuilt only when dirty
	dirty    bool           // flat must be rebuilt (replacement, resize, greeting change)
	index    map[string]int // event id -> position, for in-place replacement
	width    int
	height   int
	follow   bool
	revealed int
	held     bool // the post-greeting hold has elapsed; the log may show
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
		lines := render.Event(e, m.width, m.cfg.Loc)
		if i, ok := m.index[e.EventID()]; ok {
			m.events[i], m.rendered[i] = e, lines
			m.dirty = true
			continue
		}
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
	if !m.held { // the log appears only after the greeting is typed and held for a beat
		m.vp.SetContentLines(m.greeting())
		return
	}
	if m.dirty {
		m.flat = m.flat[:0]
		m.flat = append(m.flat, m.greeting()...)
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

func (m Model) greeting() []string {
	lines := render.Greeting(m.cfg.User, m.cfg.SessionID, m.cwd, m.started, m.cfg.Loc, m.revealed)
	if m.revealed >= m.greetingLen() {
		lines = append(lines, "")
	}
	return lines
}

var (
	footerStyle = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack)
	footerWarn  = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
)

// footer is the single status line under the log.
func (m Model) footer() string {
	state := "⇣ following"
	if !m.follow {
		state = "⇡ paused"
	}
	parts := []string{m.cfg.SessionID, fmt.Sprintf("%d events", len(m.events)), state}
	if n := m.pairer.Skipped(); n > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", n))
	}
	line := footerStyle.Render(strings.Join(parts, "  ·  "))
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
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "claude-companion " + m.cfg.SessionID
	return v
}
