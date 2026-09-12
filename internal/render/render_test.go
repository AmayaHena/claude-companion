package render

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"claude-companion/internal/event"
)

var sgr = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plain(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = sgr.ReplaceAllString(l, "")
	}
	return out
}

var at = time.Date(2026, 9, 8, 19, 57, 41, 0, time.UTC)

func assertWidth(t *testing.T, lines []string, width int) {
	t.Helper()
	for i, l := range lines {
		if w := lipgloss.Width(l); w > width {
			t.Errorf("line %d is %d cells wide, max %d: %q", i, w, width, sgr.ReplaceAllString(l, ""))
		}
	}
}

func TestCommandOK(t *testing.T) {
	c := event.Command{ID: "t1", At: at, Cmd: "ls -la\n/tmp", Stdout: "total 0\ndrwxr-xr-x  2 amaya  staff  64 .", ExitKnown: true}
	got := plain(Event(c, 120, time.UTC, 0))
	want := []string{
		"19:57 ⚒️ ls -la",
		"      ▎ total 0",
		"      ▎ drwxr-xr-x  2 amaya  staff  64 .",
		"",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	assertWidth(t, Event(c, 120, time.UTC, 0), 120)
}

func TestCommandFailedShowsExitAndStderr(t *testing.T) {
	c := event.Command{ID: "t1", At: at, Cmd: "false", Stderr: "boom", Exit: 2, ExitKnown: true}
	got := plain(Event(c, 120, time.UTC, 0))
	if got[0] != "19:57 ❗ false  exit 2" {
		t.Fatalf("header = %q", got[0])
	}
	if got[1] != "      ▎ boom" {
		t.Fatalf("stderr line = %q", got[1])
	}
}

func TestCommandUnknownExit(t *testing.T) {
	c := event.Command{ID: "t1", At: at, Cmd: "x", Stdout: "y"}
	got := plain(Event(c, 120, time.UTC, 0))
	if got[0] != "19:57 ❗ x  exit ?" {
		t.Fatalf("header = %q", got[0])
	}
}

func TestCommandRunningAndBackground(t *testing.T) {
	r := plain(Event(event.Command{ID: "t1", At: at, Cmd: "sleep 9", Running: true}, 120, time.UTC, 0))
	if len(r) != 2 || r[0] != "19:57 ⚒️ sleep 9  …" || r[1] != "" {
		t.Fatalf("running = %q", r)
	}
	b := plain(Event(event.Command{ID: "t1", At: at, Cmd: "du -sh ~", Background: true, ExitKnown: true, Stdout: "moved to background"}, 120, time.UTC, 0))
	if b[0] != "19:57 ⚒️ du -sh ~  bg" {
		t.Fatalf("background = %q", b[0])
	}
}

func TestCommandWrapsButOutputIsTruncatedToWidth(t *testing.T) {
	c := event.Command{ID: "t1", At: at, Cmd: strings.Repeat("x", 200), Stdout: strings.Repeat("y", 200), ExitKnown: true}
	lines := Event(c, 40, time.UTC, 0)
	assertWidth(t, lines, 40)
	got := plain(lines)
	joined := strings.Join(got[:len(got)-2], "")
	if strings.Contains(joined, "…") || strings.Count(joined, "x") != 200 {
		t.Fatalf("the command must wrap without loss: %q", got)
	}
	if out := got[len(got)-2]; !strings.HasSuffix(out, "…") {
		t.Fatalf("output lines are still truncated: %q", out)
	}
}

func TestEditRendersHunks(t *testing.T) {
	fc := event.FileChange{ID: "t1", At: at, Path: "/Users/amaya/x/docs/investigation.md", Kind: event.Update, Hunks: []event.Hunk{{
		OldStart: 161, OldLines: 3, NewStart: 161, NewLines: 3,
		Lines: []string{" ctx", "-old line", "+new line"},
	}}}
	got := plain(Event(fc, 120, time.UTC, 0))
	want := []string{
		"19:57 📁 /Users/amaya/x/docs/investigation.md",
		"      ▎ @@ -161,3 +161,3 @@",
		"      ▎  ctx",
		"      ▎ -old line",
		"      ▎ +new line",
		"",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	styled := Event(fc, 120, time.UTC, 0)
	if !strings.Contains(styled[3], "\x1b[1;31m-") || !strings.Contains(styled[4], "\x1b[1;32m+") {
		t.Fatalf("removed sign must be bold red and added sign bold green: %q %q", styled[3], styled[4])
	}
}

func TestCreateRendersContentAsAdded(t *testing.T) {
	fc := event.FileChange{ID: "t1", At: at, Path: "/tmp/new.yaml", Kind: event.Create, Content: "a: 1\nb: 2\n"}
	got := plain(Event(fc, 120, time.UTC, 0))
	want := []string{"19:57 🆕 /tmp/new.yaml", "      ▎ +a: 1", "      ▎ +b: 2", ""}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %q", got)
	}
}

func TestRejected(t *testing.T) {
	got := plain(Event(event.Rejected{ID: "t1", At: at, Tool: "Edit", Subject: "/tmp/a.go"}, 120, time.UTC, 0))
	if len(got) != 2 || got[0] != "19:57 ❗ Edit /tmp/a.go  rejected" || got[1] != "" {
		t.Fatalf("got %q", got)
	}
}

func TestPromptIsAProminentBlock(t *testing.T) {
	p := event.Prompt{ID: "u1", At: at, Text: "change Claude to Anthropic\nsecond line ignored"}
	lines := Event(p, 60, time.UTC, 0)
	got := plain(lines)
	want := []string{"19:57   change Claude to Anthropic", ""}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(lines[0], "\x1b[1m") { // bold
		t.Fatalf("prompt text must be bold: %q", lines[0])
	}
	// long prompts wrap onto continuation rows instead of being cut
	narrow := plain(Event(p, 20, time.UTC, 0))
	if len(narrow) != 4 || narrow[0] != "19:57   change" || narrow[1] != "        Claude to" || narrow[2] != "        Anthropic" || narrow[3] != "" {
		t.Fatalf("narrow prompt = %q", narrow)
	}
}

// Each type of action has its own hue, in two shades selected by the caller
// (0 dark, 1 light), so two actions of the same type in a row differ while
// the type stays readable. The bar is on body rows only.
func TestAccentHuePerTypeAndShade(t *testing.T) {
	cmd := event.Command{ID: "t", At: at, Cmd: "ls", Stdout: "x", ExitKnown: true}
	file := event.FileChange{ID: "t", At: at, Path: "/a", Kind: event.Create, Content: "x"}
	prompt := event.Prompt{ID: "t", At: at, Text: strings.Repeat("word ", 30)} // wraps: continuation rows are bar-free
	cases := []struct {
		name  string
		e     event.Event
		shade int
		sgr   string
	}{
		{"command dark", cmd, 0, "\x1b[33m▎"}, {"command light", cmd, 1, "\x1b[93m▎"},
		{"file dark", file, 0, "\x1b[35m▎"}, {"file light", file, 1, "\x1b[95m▎"},
	}
	for _, c := range cases {
		lines := Event(c.e, 80, time.UTC, c.shade)
		if strings.Contains(lines[0], "▎") || !strings.Contains(lines[1], c.sgr) {
			t.Errorf("%s: header %q body %q", c.name, lines[0], lines[1])
		}
	}
	if p := Event(prompt, 40, time.UTC, 0); strings.Contains(strings.Join(p, ""), "▎") {
		t.Errorf("prompt rows never carry a bar: %q", plain(p))
	}
	if !strings.HasPrefix(Event(cmd, 80, time.UTC, 0)[0], "\x1b[1;90m19:57") {
		t.Fatalf("clock = %q", Event(cmd, 80, time.UTC, 0)[0])
	}
}

func TestGitWriteCommandsAreFlagged(t *testing.T) {
	for _, cmd := range []string{"git add -A", "git commit -m x", "git push -u origin main", "cd repo && git   push --force", "GIT_DIR=x git commit"} {
		lines := Event(event.Command{ID: "t", At: at, Cmd: cmd, ExitKnown: true}, 120, time.UTC, 0)
		if !strings.HasPrefix(plain(lines)[0], "19:57 ⚠️ ") {
			t.Errorf("%q: header = %q", cmd, plain(lines)[0])
		}
		if !strings.Contains(lines[0], "\x1b[4;31") { // underline + red on the command text (lipgloss emits 4;31;4 per run)
			t.Errorf("%q: command text must be underlined red: %q", cmd, lines[0])
		}
	}
	// the rule is literal: any line containing the phrase is flagged, so only these stay plain
	for _, cmd := range []string{"git status", "git log --oneline", "git diff", "gitk", "git pushover"} {
		lines := Event(event.Command{ID: "t", At: at, Cmd: cmd, ExitKnown: true}, 120, time.UTC, 0)
		if strings.Contains(plain(lines)[0], "⚠️") {
			t.Errorf("%q must not be flagged: %q", cmd, plain(lines)[0])
		}
	}
	// a wrapped warning command keeps the underline on every row, and a failed one keeps its exit code
	long := event.Command{ID: "t", At: at, Cmd: "git commit -m \"a long message that will certainly wrap onto the next row\"", ExitKnown: true, Exit: 1}
	rows := Event(long, 40, time.UTC, 0)
	if len(rows) < 3 || !strings.Contains(rows[1], "\x1b[4;31") || !strings.Contains(plain(rows)[len(rows)-2], "exit 1") {
		t.Fatalf("rows = %q", plain(rows))
	}
}

func TestLongCommandWrapsInsteadOfTruncating(t *testing.T) {
	c := event.Command{ID: "t", At: at, Cmd: `go test ./internal/ui/ 2>&1 | grep -E "^ok" | head -5`, ExitKnown: true, Exit: 2}
	got := plain(Event(c, 30, time.UTC, 0))
	want := []string{ // word-wrapped at 30-9 = 21 cells; failed command, so ❗; continuation rows carry no bar
		"19:57 ❗ go test",
		"         ./internal/ui/ 2>&1 |",
		"         grep -E \"^ok\" | head",
		"         -5  exit 2",
		"",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	assertWidth(t, Event(c, 30, time.UTC, 0), 30)
	if !strings.Contains(Event(c, 30, time.UTC, 0)[2], "\x1b[32m\"^ok\"") { // row 2 holds the string literal
		t.Fatal("continuation rows are highlighted too")
	}
}

func TestTabsAndCarriageReturnsNeverWidenALine(t *testing.T) {
	const width = 60
	cases := []event.Event{
		event.Command{ID: "t", At: at, Cmd: "go test", ExitKnown: true,
			Stdout: "goroutine 1 [running]:\n\t/Users/amaya/go/pkg/mod/github.com/some/very/long/module/path/file.go:123 +0x1c\n\t\t\tdeep"},
		event.Command{ID: "t", At: at, Cmd: "brew install x", ExitKnown: true,
			Stdout: "#=#=#      \r##O#-#     \r####  12.3%\r######## 100.0%\nDone"},
		event.FileChange{ID: "t", At: at, Path: "/tmp/Makefile", Kind: event.Update, Hunks: []event.Hunk{{
			OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1, Lines: []string{"-build:\n", "+build:\tgo build ./..."}}}},
		event.FileChange{ID: "t", At: at, Path: "/tmp/new.txt", Kind: event.Create, Content: "a\tb\tc\td\te\tf\tg\th\ti\tj\tk\tl\tm\tn\to\tp\r\nend"},
	}
	for _, e := range cases {
		for i, line := range Event(e, width, time.UTC, 0) {
			if w := lipgloss.Width(line); w > width {
				t.Errorf("%T line %d: width %d > %d: %q", e, i, w, width, sgr.ReplaceAllString(line, ""))
			}
			if strings.ContainsAny(line, "\t\r") {
				t.Errorf("%T line %d still contains a tab or CR: %q", e, i, sgr.ReplaceAllString(line, ""))
			}
		}
	}
	// The terminal shows what comes after the last CR of a progress line.
	got := plain(Event(cases[1], width, time.UTC, 0))
	if got[1] != "      ▎ ######## 100.0%" {
		t.Fatalf("progress line = %q", got[1])
	}
}

func TestDiffLinesAreSyntaxHighlightedWithColouredSigns(t *testing.T) {
	fc := event.FileChange{ID: "t", At: at, Path: "/tmp/main.go", Kind: event.Update, Hunks: []event.Hunk{{
		OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2,
		Lines: []string{` x := 1`, `-old := "a"`, `+new := "b" // note`},
	}}}
	lines := Event(fc, 120, time.UTC, 0)
	got := plain(lines)
	want := []string{"19:57 📁 /tmp/main.go", "      ▎ @@ -1,2 +1,2 @@", "      ▎  x := 1", `      ▎ -old := "a"`, `      ▎ +new := "b" // note`, ""}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("text changed:\n%s", strings.Join(got, "\n"))
	}
	if !strings.Contains(lines[3], "\x1b[1;31m-") || !strings.Contains(lines[4], "\x1b[1;32m+") {
		t.Fatalf("signs must be bold red / bold green: %q %q", lines[3], lines[4])
	}
	if !strings.Contains(lines[4], "\x1b[32m\"b\"") || !strings.Contains(lines[4], "\x1b[2;90m// note") || !strings.Contains(lines[2], "\x1b[36m1") {
		t.Fatalf("code must be highlighted on +, - and context lines: %q %q", lines[2], lines[4])
	}
	assertWidth(t, lines, 120)
}

func TestCreatedGoFileIsHighlighted(t *testing.T) {
	fc := event.FileChange{ID: "t", At: at, Path: "/tmp/x.go", Kind: event.Create, Content: "package x\n\nfunc F() {}\n"}
	lines := Event(fc, 80, time.UTC, 0)
	if got := plain(lines); strings.Join(got, "\n") != "19:57 🆕 /tmp/x.go\n      ▎ +package x\n      ▎ +\n      ▎ +func F() {}\n" {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(lines[1], "\x1b[1;32m+") || !strings.Contains(lines[1], "\x1b[35mpackage") {
		t.Fatalf("sign green, keyword magenta: %q", lines[1])
	}
}

func TestUnknownFileTypeStaysPlain(t *testing.T) {
	fc := event.FileChange{ID: "t", At: at, Path: "/tmp/notes.unknownext", Kind: event.Create, Content: "func is not a keyword here"}
	if l := Event(fc, 80, time.UTC, 0)[1]; strings.Contains(l, "\x1b[35mfunc") || strings.Count(l, "\x1b[") != 4 { // bar on/off, sign on/off: nothing else
		t.Fatalf("no lexer, no colours in the content: %q", l)
	}
}

func TestCommandTextIsHighlightedAndTruncatedSafely(t *testing.T) {
	c := event.Command{ID: "t", At: at, Cmd: `grep -rn "needle" . || echo "none found in the haystack at all"`, ExitKnown: true}
	full := Event(c, 120, time.UTC, 0)[0]
	if !strings.Contains(full, "\x1b[32m\"needle\"") || plain([]string{full})[0] != `19:57 ⚒️ grep -rn "needle" . || echo "none found in the haystack at all"` {
		t.Fatalf("got %q", full)
	}
	narrow := Event(c, 40, time.UTC, 0)
	assertWidth(t, narrow, 40)
	if p := plain(narrow); len(p) < 3 || strings.Contains(strings.Join(p, ""), "…") {
		t.Fatalf("a long command wraps, never cut: %q", p)
	}
}
