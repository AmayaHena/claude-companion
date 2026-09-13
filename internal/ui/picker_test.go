package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"claude-companion/internal/transcript"
)

func pickerFixture() ([]transcript.Session, time.Time) {
	now := time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)
	return []transcript.Session{
		{ID: "cccccccc-0000-0000-0000-000000000000", Path: "/p/c.jsonl", Cwd: "/Users/amaya/AElys/claude-companion", FirstPrompt: "make me a poc under /Users/amaya/Work", ModTime: now.Add(-3 * time.Minute)},
		{ID: "bbbbbbbb-0000-0000-0000-000000000000", Path: "/p/b.jsonl", Cwd: "", FirstPrompt: "", ModTime: now.Add(-2 * time.Hour)}, // no cwd found: no folder shown, never "."
		{ID: "aaaaaaaa-0000-0000-0000-000000000000", Path: "/p/a.jsonl", Cwd: "/Users/amaya/old", FirstPrompt: strings.Repeat("long prompt ", 20), ModTime: now.Add(-49 * time.Hour)},
	}, now
}

func pressPicker(p Picker, k tea.Key) (Picker, tea.Cmd) {
	next, cmd := p.Update(tea.KeyPressMsg(k))
	return next.(Picker), cmd
}

func TestPickerShowsRecentSessions(t *testing.T) {
	sessions, now := pickerFixture()
	p := NewPicker(sessions, now)
	next, _ := p.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	p = next.(Picker)
	v := ansi.ReplaceAllString(p.View().Content, "")
	for _, want := range []string{"cccccccc", "3 min ago", "claude-companion", "make me a poc", "2 h ago", "2 d ago", "old", "(no prompt yet)"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	for _, line := range strings.Split(v, "\n") {
		if strings.Contains(line, "bbbbbbbb") && strings.Contains(line, " . ") {
			t.Errorf("empty cwd must not render as '.': %q", line)
		}
	}
	for _, line := range strings.Split(v, "\n") {
		if w := len([]rune(line)); w > 100 {
			t.Errorf("line wider than 100: %d %q", w, line)
		}
	}
}

func TestPickerKeysChooseAndQuit(t *testing.T) {
	sessions, now := pickerFixture()
	p := NewPicker(sessions, now)
	next, _ := p.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	p = next.(Picker)
	p, _ = pressPicker(p, tea.Key{Code: tea.KeyDown})
	p, _ = pressPicker(p, tea.Key{Code: 'j', Text: "j"})
	p, _ = pressPicker(p, tea.Key{Code: tea.KeyDown}) // stays on the last row
	p, _ = pressPicker(p, tea.Key{Code: 'k', Text: "k"})
	p, cmd := pressPicker(p, tea.Key{Code: tea.KeyEnter})
	if p.Chosen == nil || p.Chosen.ID != "bbbbbbbb-0000-0000-0000-000000000000" || !isQuit(cmd) {
		t.Fatalf("chosen = %+v quit=%v", p.Chosen, isQuit(cmd))
	}
	q := NewPicker(sessions, now)
	q, cmd = pressPicker(q, tea.Key{Code: 'q', Text: "q"})
	if q.Chosen != nil || !isQuit(cmd) {
		t.Fatalf("q must quit without a choice: %+v", q.Chosen)
	}
	e := NewPicker(nil, now)
	if v := e.View().Content; !strings.Contains(v, "no sessions") {
		t.Fatalf("empty picker view = %q", v)
	}
}
