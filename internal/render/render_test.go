package render

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"claude-companion/internal/event"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plain(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = ansi.ReplaceAllString(l, "")
	}
	return out
}

var at = time.Date(2026, 9, 8, 19, 57, 41, 0, time.UTC)

func assertWidth(t *testing.T, lines []string, width int) {
	t.Helper()
	for i, l := range lines {
		if w := lipgloss.Width(l); w > width {
			t.Errorf("line %d is %d cells wide, max %d: %q", i, w, width, ansi.ReplaceAllString(l, ""))
		}
	}
}

func TestCommandOK(t *testing.T) {
	c := event.Command{ID: "t1", At: at, Cmd: "ls -la\n/tmp", Stdout: "total 0\ndrwxr-xr-x  2 amaya  staff  64 .", ExitKnown: true}
	got := plain(Event(c, 120, time.UTC))
	want := []string{
		"19:57:41 ⚒  ls -la",
		"          total 0",
		"          drwxr-xr-x  2 amaya  staff  64 .",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	assertWidth(t, Event(c, 120, time.UTC), 120)
}

func TestCommandFailedShowsExitAndStderr(t *testing.T) {
	c := event.Command{ID: "t1", At: at, Cmd: "false", Stderr: "boom", Exit: 2, ExitKnown: true}
	got := plain(Event(c, 120, time.UTC))
	if got[0] != "19:57:41 ❗ false  exit 2" {
		t.Fatalf("header = %q", got[0])
	}
	if got[1] != "          boom" {
		t.Fatalf("stderr line = %q", got[1])
	}
}

func TestCommandUnknownExit(t *testing.T) {
	c := event.Command{ID: "t1", At: at, Cmd: "x", Stdout: "y"}
	got := plain(Event(c, 120, time.UTC))
	if got[0] != "19:57:41 ❗ x  exit ?" {
		t.Fatalf("header = %q", got[0])
	}
}

func TestCommandRunningAndBackground(t *testing.T) {
	r := plain(Event(event.Command{ID: "t1", At: at, Cmd: "sleep 9", Running: true}, 120, time.UTC))
	if len(r) != 1 || r[0] != "19:57:41 ⚒  sleep 9  …" {
		t.Fatalf("running = %q", r)
	}
	b := plain(Event(event.Command{ID: "t1", At: at, Cmd: "du -sh ~", Background: true, ExitKnown: true, Stdout: "moved to background"}, 120, time.UTC))
	if b[0] != "19:57:41 ⚒  du -sh ~  bg" {
		t.Fatalf("background = %q", b[0])
	}
}

func TestCommandLongLineIsTruncatedToWidth(t *testing.T) {
	c := event.Command{ID: "t1", At: at, Cmd: strings.Repeat("x", 200), Stdout: strings.Repeat("y", 200), ExitKnown: true}
	lines := Event(c, 40, time.UTC)
	assertWidth(t, lines, 40)
	got := plain(lines)
	if !strings.HasSuffix(got[0], "…") || !strings.HasSuffix(got[1], "…") {
		t.Fatalf("truncated lines must end with …, got %q", got)
	}
}

func TestEditRendersHunks(t *testing.T) {
	fc := event.FileChange{ID: "t1", At: at, Path: "/Users/amaya/x/docs/investigation.md", Kind: event.Update, Hunks: []event.Hunk{{
		OldStart: 161, OldLines: 3, NewStart: 161, NewLines: 3,
		Lines: []string{" ctx", "-old line", "+new line"},
	}}}
	got := plain(Event(fc, 120, time.UTC))
	want := []string{
		"19:57:41 📁 /Users/amaya/x/docs/investigation.md",
		"          @@ -161,3 +161,3 @@",
		"           ctx",
		"          -old line",
		"          +new line",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	styled := Event(fc, 120, time.UTC)
	if !strings.Contains(styled[3], "\x1b[31m") || !strings.Contains(styled[4], "\x1b[32m") {
		t.Fatalf("removed must be red (31) and added green (32): %q %q", styled[3], styled[4])
	}
}

func TestCreateRendersContentAsAdded(t *testing.T) {
	fc := event.FileChange{ID: "t1", At: at, Path: "/tmp/new.yaml", Kind: event.Create, Content: "a: 1\nb: 2\n"}
	got := plain(Event(fc, 120, time.UTC))
	want := []string{"19:57:41 🆕 /tmp/new.yaml", "          +a: 1", "          +b: 2"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %q", got)
	}
}

func TestRejected(t *testing.T) {
	got := plain(Event(event.Rejected{ID: "t1", At: at, Tool: "Edit", Subject: "/tmp/a.go"}, 120, time.UTC))
	if len(got) != 1 || got[0] != "19:57:41 ❗ Edit /tmp/a.go  rejected" {
		t.Fatalf("got %q", got)
	}
}

func TestPromptIsASeparatorOfExactWidth(t *testing.T) {
	p := event.Prompt{ID: "u1", At: at, Text: "change Claude to Anthropic\nsecond line ignored"}
	lines := Event(p, 60, time.UTC)
	got := plain(lines)
	if len(got) != 1 {
		t.Fatalf("got %q", got)
	}
	if !strings.HasPrefix(got[0], "── 19:57:41  change Claude to Anthropic ──") || !strings.HasSuffix(got[0], "─") {
		t.Fatalf("got %q", got[0])
	}
	if w := lipgloss.Width(lines[0]); w != 60 {
		t.Fatalf("width = %d want 60", w)
	}
	narrow := Event(p, 40, time.UTC)
	if w := lipgloss.Width(narrow[0]); w != 40 {
		t.Fatalf("narrow width = %d want 40", w)
	}
	if n := plain(narrow)[0]; !strings.Contains(n, "…") || !strings.HasSuffix(n, "──") {
		t.Fatalf("narrow must truncate the text and still end with two dashes: %q", n)
	}
}

func TestGreetingReveal(t *testing.T) {
	started := time.Date(2026, 9, 8, 19, 47, 23, 0, time.UTC)
	full := plain(Greeting("amaya", "faeeadc5-5b2d", "/Users/amaya", started, time.UTC, 100))
	want := []string{
		"hello, amaya",
		"",
		"session  faeeadc5-5b2d",
		"cwd      /Users/amaya",
		"started  2026-09-08 19:47:23",
	}
	if strings.Join(full, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %q", full)
	}
	part := plain(Greeting("amaya", "faeeadc5-5b2d", "/Users/amaya", started, time.UTC, 3))
	if part[0] != "hel" || len(part) != 1 {
		t.Fatalf("partial reveal = %q", part)
	}
}
