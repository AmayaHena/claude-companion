package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/AmayaHena/claude-companion/internal/transcript"
)

// The window title and the footer's error text are the two strings that
// do not go through the renderer; both must be sanitised.
func TestTitleAndErrorAreSanitised(t *testing.T) {
	m := New(Config{SessionID: "abc\x1b]0;EVIL\x07def", Path: "/tmp/x.jsonl", Loc: time.UTC})
	if v := m.View(); strings.ContainsAny(v.WindowTitle, "\x1b\x07") || !strings.Contains(v.WindowTitle, "abc␛]0;EVILdef") {
		t.Fatalf("title = %q", v.WindowTitle)
	}
	next, _ := m.Update(lineMsg(transcript.Line{Err: fmt.Errorf("open /tmp/\x1b[2Jx: gone")}))
	m = next.(Model)
	if f := ansi.ReplaceAllString(m.footer(), ""); strings.Contains(f, "\x1b[2J") || !strings.Contains(f, "␛[2Jx: gone") {
		t.Fatalf("footer = %q", f)
	}
}

// The clipboard receives the whole command, sanitised but with its
// newlines and tabs kept, and the footer says how many lines were copied.
func TestCopySanitisesAndCountsLines(t *testing.T) {
	m := sized(newModel(), 120, 30)
	line := `{"type":"assistant","uuid":"a1","timestamp":"2026-09-10T10:00:00.000Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"b1","name":"Bash","input":{"command":"echo a\u001b]0;T\n\tprintf b\nls"}}]}}`
	m = feed(m, line)
	m, cmd := key(m, tea.Key{Code: 'c', Text: "c"})
	if cmd == nil {
		t.Fatal("no clipboard command")
	}
	if got := fmt.Sprint(cmd()); got != "echo a␛]0;T\n\tprintf b\nls" {
		t.Fatalf("clipboard = %q", got)
	}
	if f := ansi.ReplaceAllString(m.footer(), ""); !strings.Contains(f, "copied 3 lines") {
		t.Fatalf("footer = %q", f)
	}
	u, r := bashPair(7, 1)
	m = feed(m, u, r)
	m, _ = key(m, tea.Key{Code: 'c', Text: "c"})
	if f := ansi.ReplaceAllString(m.footer(), ""); !strings.Contains(f, "copied") || strings.Contains(f, "lines") {
		t.Fatalf("one-line copy footer = %q", f)
	}
}
