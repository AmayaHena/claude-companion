package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/AmayaHena/claude-companion/internal/event"
	"github.com/AmayaHena/claude-companion/internal/transcript"
)

// drain feeds every line the tailer delivers within `quiet` of silence.
func drain(t *testing.T, m Model, ch <-chan transcript.Line, quiet time.Duration) Model {
	t.Helper()
	for {
		select {
		case l, ok := <-ch:
			if !ok {
				return m
			}
			next, _ := m.Update(lineMsg(l))
			m = next.(Model)
		case <-time.After(quiet):
			return m
		}
	}
}

func count(m Model) (cmds, files, rejected, prompts int) {
	for _, e := range m.events {
		switch e.(type) {
		case event.Command:
			cmds++
		case event.FileChange:
			files++
		case event.Rejected:
			rejected++
		case event.Prompt:
			prompts++
		}
	}
	return
}

// TestReplayFixturesThroughTail composes Tail, Pairer, render and the model
// over every fixture concatenated into one file, then appends more while the
// tailer is running. Prompts come first because each prompt clears the log:
// two prompt fixtures (plain, text block) leave 1 prompt on screen, then
// 3 commands (ok, exit≠0, background), 3 file changes (edit, create, update),
// 2 rejected (bash, edit); the system-injected prompt fixtures are ignored.
func TestReplayFixturesThroughTail(t *testing.T) {
	names := []string{"prompt_task_notification", "prompt_command_name", "prompt_local_command_stdout", "prompt_meta",
		"prompt_plain", "prompt_textblock",
		"bash_ok", "bash_exit_nonzero", "bash_background", "edit_update", "write_create", "write_update",
		"bash_rejected", "edit_rejected"}
	var all []byte
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join("..", "event", "testdata", n+".jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, b...)
	}
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, all, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := transcript.Tail(ctx, path, 10*time.Millisecond)
	m := sized(newModel(), 120, 30)
	m = drain(t, m, ch, 200*time.Millisecond)
	if c, f, r, p := count(m); c != 3 || f != 3 || r != 2 || p != 1 {
		t.Fatalf("got commands=%d files=%d rejected=%d prompts=%d", c, f, r, p)
	}
	if !m.follow || !m.vp.AtBottom() {
		t.Fatal("must be following at the bottom")
	}
	// Live append: a running command, then its result.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"type":"assistant","uuid":"a9","timestamp":"2026-09-10T10:00:00.000Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"t9","name":"Bash","input":{"command":"echo LIVE-MARKER"}}]}}` + "\n")
	f.Close()
	m = drain(t, m, ch, 200*time.Millisecond)
	if c, _, _, _ := count(m); c != 4 || !strings.Contains(content(m), "echo LIVE-MARKER  …") {
		t.Fatalf("appended running command not shown: commands=%d tail=%q", c, tailOf(content(m), 200))
	}
	f, _ = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"type":"user","uuid":"u9","timestamp":"2026-09-10T10:00:01.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t9","content":"LIVE-MARKER\n","is_error":false}]},"toolUseResult":{"stdout":"LIVE-MARKER\n","stderr":"","interrupted":false}}` + "\n")
	f.Close()
	m = drain(t, m, ch, 200*time.Millisecond)
	if c, _, _, _ := count(m); c != 4 || strings.Contains(content(m), "echo LIVE-MARKER  …") || !strings.HasSuffix(strings.TrimRight(content(m), "\n"), "      ▕▏LIVE-MARKER") {
		t.Fatalf("result must replace the running command in place: commands=%d tail=%q", c, tailOf(content(m), 200))
	}
	if !m.vp.AtBottom() {
		t.Fatal("still following after live appends")
	}
}

func tailOf(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// TestReplayRealSession is opt-in: CLAUDE_COMPANION_E2E_FILE=<transcript>
// CLAUDE_COMPANION_E2E_EXPECT=<commands>,<files>,<rejected>,<prompts>.
func TestReplayRealSession(t *testing.T) {
	path := os.Getenv("CLAUDE_COMPANION_E2E_FILE")
	if path == "" {
		t.Skip("set CLAUDE_COMPANION_E2E_FILE to run")
	}
	var want [4]int
	if _, err := parseCounts(os.Getenv("CLAUDE_COMPANION_E2E_EXPECT"), &want); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := transcript.Tail(ctx, path, 10*time.Millisecond)
	m := sized(newModel(), 160, 50)
	start := time.Now()
	m = drain(t, m, ch, 500*time.Millisecond)
	c, f, r, p := count(m)
	t.Logf("%s: commands=%d files=%d rejected=%d prompts=%d skipped=%d in %s", filepath.Base(path), c, f, r, p, m.pairer.Skipped(), time.Since(start).Round(time.Millisecond))
	if got := [4]int{c, f, r, p}; got != want {
		t.Fatalf("got %v want %v", got, want)
	}
	_ = tea.Quit
}

func parseCounts(s string, out *[4]int) (int, error) {
	n := 0
	for i, part := range strings.Split(s, ",") {
		if i >= 4 {
			break
		}
		v := 0
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				return n, os.ErrInvalid
			}
			v = v*10 + int(ch-'0')
		}
		out[i] = v
		n++
	}
	return n, nil
}
