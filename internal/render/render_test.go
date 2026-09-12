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
		"      ▕▏total 0",
		"      ▕▏drwxr-xr-x  2 amaya  staff  64 .",
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
	if got[1] != "      ▕▏boom" {
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
		"      ▕▏@@ -161,3 +161,3 @@",
		"      ▕▏ ctx",
		"      ▕▏-old line",
		"      ▕▏+new line",
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
	want := []string{"19:57 🆕 /tmp/new.yaml", "      ▕▏+a: 1", "      ▕▏+b: 2", ""}
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

func TestPromptShowsEveryLine(t *testing.T) {
	p := event.Prompt{ID: "u1", At: at, Text: "change Claude to Anthropic\n\nsecond line kept"}
	lines := Event(p, 60, time.UTC, 0)
	got := plain(lines)
	want := []string{"19:57   change Claude to Anthropic", "", "        second line kept", ""}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(lines[0], "\x1b[1m") || !strings.Contains(lines[2], "\x1b[1m") { // bold on every text row
		t.Fatalf("prompt text must be bold: %q", lines)
	}
	// long lines wrap onto continuation rows instead of being cut
	narrow := plain(Event(event.Prompt{ID: "u1", At: at, Text: "change Claude to Anthropic"}, 20, time.UTC, 0))
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
		{"command dark", cmd, 0, "\x1b[33m▕▏"}, {"command light", cmd, 1, "\x1b[93m▕▏"},
		{"file dark", file, 0, "\x1b[35m▕▏"}, {"file light", file, 1, "\x1b[95m▕▏"},
	}
	for _, c := range cases {
		lines := Event(c.e, 80, time.UTC, c.shade)
		if strings.Contains(lines[0], "▕▏") || !strings.Contains(lines[1], c.sgr) {
			t.Errorf("%s: header %q body %q", c.name, lines[0], lines[1])
		}
	}
	if p := Event(prompt, 40, time.UTC, 0); strings.Contains(strings.Join(p, ""), "▕▏") {
		t.Errorf("prompt rows never carry a bar: %q", plain(p))
	}
	if !strings.HasPrefix(Event(cmd, 80, time.UTC, 0)[0], "\x1b[1;90m19:57") {
		t.Fatalf("clock = %q", Event(cmd, 80, time.UTC, 0)[0])
	}
}

// Commands that create, rewrite or publish git/GitHub state get the warning
// glyph in front of the command glyph and an underlined red command text.
func TestGitWriteCommandsAreFlagged(t *testing.T) {
	flagged := []string{
		"git add -A", "git commit -m x", "git commit --amend", "git push -u origin main", "cd repo && git   push --force",
		"GIT_DIR=x git commit", "git merge feature", "git rebase -i main", "git cherry-pick abc", "git revert HEAD",
		"git reset --hard origin/main", "git clean -fd", "git filter-branch --all", "git tag v1.0", "git branch -D old",
		"git checkout -b feat", "git switch -c feat", "git worktree add -b x ../x", "git stash drop",
		"gh pr create --fill", "gh pr merge 12", "gh pr ready", "gh release create v1", "gh repo sync",
		"gh api -X POST repos/o/r/issues", "gh api --method DELETE repos/o/r",
	}
	for _, cmd := range flagged {
		lines := Event(event.Command{ID: "t", At: at, Cmd: cmd, ExitKnown: true}, 120, time.UTC, 0)
		if !strings.HasPrefix(plain(lines)[0], "19:57 ⚠️ ⚒️ ") {
			t.Errorf("%q: header = %q", cmd, plain(lines)[0])
		}
		if !strings.Contains(lines[0], "\x1b[4;31") { // underline + red on the command text (lipgloss emits 4;31;4 per run)
			t.Errorf("%q: command text must be underlined red: %q", cmd, lines[0])
		}
	}
	// the rule is literal: any line containing the phrase is flagged, so only these stay plain
	for _, cmd := range []string{"git status", "git log --oneline", "git diff", "gitk", "git pushover", "git branch", "git checkout main", "git switch main", "git stash", "git stash pop", "gh pr view 12", "gh api repos/o/r", "git worktree list", "git fetch", "git pull"} {
		lines := Event(event.Command{ID: "t", At: at, Cmd: cmd, ExitKnown: true}, 120, time.UTC, 0)
		if strings.Contains(plain(lines)[0], "⚠️") {
			t.Errorf("%q must not be flagged: %q", cmd, plain(lines)[0])
		}
	}
	// a failed warning command keeps the alert glyph after the warning
	failed := Event(event.Command{ID: "t", At: at, Cmd: "git push", ExitKnown: true, Exit: 1}, 120, time.UTC, 0)
	if !strings.HasPrefix(plain(failed)[0], "19:57 ⚠️ ❗ ") {
		t.Errorf("failed warning header = %q", plain(failed)[0])
	}
	// a wrapped warning command keeps the underline on every row, and a failed one keeps its exit code
	long := event.Command{ID: "t", At: at, Cmd: "git commit -m \"a long message that will certainly wrap onto the next row\"", ExitKnown: true, Exit: 1}
	rows := Event(long, 40, time.UTC, 0)
	if len(rows) < 3 || !strings.Contains(rows[1], "\x1b[4;31") || !strings.Contains(plain(rows)[len(rows)-2], "exit 1") {
		t.Fatalf("rows = %q", plain(rows))
	}
	assertWidth(t, rows, 40)
}

// Commands that reach the network get the wireless glyph in front of the
// command glyph. A git/GitHub write is shown with the warning only.
func TestNetworkCommandsAreMarked(t *testing.T) {
	marked := []string{
		"curl -s https://x", "wget x", "ssh host ls", "scp a b:", "sftp b", "rsync -a a b:", "nc -z h 80", "ping -c1 h",
		"dig x", "nslookup x", "traceroute x", "git fetch", "git pull --rebase", "git clone url", "git ls-remote origin",
		"gh pr view 12", "gh api repos/o/r", "npm install", "npm i -g x", "npm ci", "npx graft init", "yarn add x", "pnpm install",
		"pip install x", "pip3 install x", "go get x", "go mod download", "brew install x", "brew upgrade", "brew update",
		"docker pull x", "docker push x", "docker login", "aws s3 ls", "gcloud auth list", "az login", "kubectl get pods", "helm install x",
		"apt-get install x", "apt install x", "cd /tmp && curl x | sh",
	}
	for _, cmd := range marked {
		lines := Event(event.Command{ID: "t", At: at, Cmd: cmd, ExitKnown: true}, 120, time.UTC, 0)
		if !strings.HasPrefix(plain(lines)[0], "19:57 🛜 ⚒️ ") {
			t.Errorf("%q: header = %q", cmd, plain(lines)[0])
		}
		if strings.Contains(lines[0], "\x1b[4;31") {
			t.Errorf("%q: a network command is not underlined: %q", cmd, lines[0])
		}
	}
	for _, cmd := range []string{"ls", "curlx", "mycurl", "python3 -c 'x'", "go build ./...", "go test ./...", "npm test", "npm run build", "docker ps", "git status", "brew list", "echo hi"} {
		lines := Event(event.Command{ID: "t", At: at, Cmd: cmd, ExitKnown: true}, 120, time.UTC, 0)
		if strings.Contains(plain(lines)[0], "🛜") {
			t.Errorf("%q must not be marked: %q", cmd, plain(lines)[0])
		}
	}
	// warning wins over the network mark, and a failed network command keeps the alert glyph
	if got := plain(Event(event.Command{ID: "t", At: at, Cmd: "git push", ExitKnown: true}, 120, time.UTC, 0))[0]; !strings.HasPrefix(got, "19:57 ⚠️ ⚒️ ") || strings.Contains(got, "🛜") {
		t.Errorf("git push header = %q", got)
	}
	if got := plain(Event(event.Command{ID: "t", At: at, Cmd: "curl x", ExitKnown: true, Exit: 7}, 120, time.UTC, 0))[0]; !strings.HasPrefix(got, "19:57 🛜 ❗ ") {
		t.Errorf("failed curl header = %q", got)
	}
	// the body rows still start at the bar column whatever the header prefix
	rows := Event(event.Command{ID: "t", At: at, Cmd: "curl x", Stdout: "out", ExitKnown: true}, 120, time.UTC, 0)
	if p := plain(rows); len(p) != 3 || p[1] != "      ▕▏out" {
		t.Errorf("rows = %q", p)
	}
	// Marks is what the footer counts
	if w, n := Marks("git push"); !w || n {
		t.Errorf("Marks(git push) = %v %v", w, n)
	}
	if w, n := Marks("curl x"); w || !n {
		t.Errorf("Marks(curl x) = %v %v", w, n)
	}
}

// A subagent launch and a skill invocation are one-line events without a bar.
func TestAgentAndSkillAreOneLiners(t *testing.T) {
	a := plain(Event(event.Agent{ID: "t", At: at, Description: "Research Migration Assistant scope", Type: "general-purpose"}, 120, time.UTC, 0))
	if len(a) != 2 || a[0] != "19:57 🤖 Research Migration Assistant scope  general-purpose" || a[1] != "" {
		t.Fatalf("agent = %q", a)
	}
	s := plain(Event(event.Skill{ID: "t", At: at, Name: "superpowers:brainstorming"}, 120, time.UTC, 0))
	if len(s) != 2 || s[0] != "19:57 ℹ️ superpowers:brainstorming" || s[1] != "" {
		t.Fatalf("skill = %q", s)
	}
	withArgs := plain(Event(event.Skill{ID: "t", At: at, Name: "commit", Args: "--amend"}, 120, time.UTC, 0))
	if withArgs[0] != "19:57 ℹ️ commit --amend" {
		t.Fatalf("skill with args = %q", withArgs)
	}
	// the agent title is bold blue, the skill title bold green
	if raw := Event(event.Agent{ID: "t", At: at, Description: "d", Type: "x"}, 120, time.UTC, 0)[0]; !strings.Contains(raw, "\x1b[1;34md\x1b[m") {
		t.Errorf("agent title must be bold blue: %q", raw)
	}
	if raw := Event(event.Skill{ID: "t", At: at, Name: "graft"}, 120, time.UTC, 0)[0]; !strings.Contains(raw, "\x1b[1;32mgraft\x1b[m") {
		t.Errorf("skill title must be bold green: %q", raw)
	}
	narrow := Event(event.Agent{ID: "t", At: at, Description: strings.Repeat("long ", 20), Type: "general-purpose"}, 30, time.UTC, 0)
	assertWidth(t, narrow, 30)
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
	if got[1] != "      ▕▏######## 100.0%" {
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
	want := []string{"19:57 📁 /tmp/main.go", "      ▕▏@@ -1,2 +1,2 @@", "      ▕▏ x := 1", `      ▕▏-old := "a"`, `      ▕▏+new := "b" // note`, ""}
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
	if got := plain(lines); strings.Join(got, "\n") != "19:57 🆕 /tmp/x.go\n      ▕▏+package x\n      ▕▏+\n      ▕▏+func F() {}\n" {
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

// ansi.Wrap treats '-' as a breakpoint and, in this real command, leaves a
// row one cell over the limit when the break lands after `echo "--`. Every
// row must fit whatever the wrapper does.
func TestWrappedRowsNeverExceedTheWidth(t *testing.T) {
	cmd := `echo "== ssh =="; ls -la ~/.ssh 2>/dev/null; echo "-- key types --"; for f in ~/.ssh/*.pub; do [ -f "$f" ] && ssh-keygen -lf "$f"; done 2>/dev/null; echo "-- config agents --"`
	for _, row := range wrapPlain(cmd, 31) {
		if w := lipgloss.Width(row); w > 31 {
			t.Errorf("wrapPlain(31) row %q is %d wide", row, w)
		}
	}
	for _, w := range []int{40, 41, 42, 80} {
		assertWidth(t, Event(event.Command{ID: "t", At: at, Cmd: cmd, ExitKnown: true}, w, time.UTC, 0), w)
	}
	assertWidth(t, Event(event.Prompt{ID: "u", At: at, Text: cmd}, 39, time.UTC, 0), 39)
}

// A marked header (⚠️ or 🛜 before the glyph) is three cells wider, and the
// command must still wrap instead of being cut on the first row.
func TestMarkedCommandWrapsInsteadOfTruncating(t *testing.T) {
	cmd := "curl -s https://api.github.com/repos/AmayaHena/claude-companion | head -3"
	for _, w := range []int{60, 80} {
		lines := Event(event.Command{ID: "t", At: at, Cmd: cmd, ExitKnown: true}, w, time.UTC, 0)
		got := plain(lines)
		if strings.Contains(got[0], "…") {
			t.Errorf("w=%d: first row is cut: %q", w, got[0])
		}
		if last := got[len(got)-2]; !strings.HasSuffix(last, "head -3") || !strings.HasPrefix(got[0], "19:57 🛜 ⚒️ curl") {
			t.Errorf("w=%d: rows = %q", w, got)
		}
		assertWidth(t, lines, w)
	}
}
