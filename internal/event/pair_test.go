package event

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixture returns the lines of testdata/<name>.jsonl.
func fixture(t *testing.T, name string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, l := range strings.Split(string(b), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// feedAll feeds every line and returns the events in order.
func feedAll(t *testing.T, p *Pairer, lines []string) []Event {
	t.Helper()
	var out []Event
	for _, l := range lines {
		evs, err := p.Feed(l)
		if err != nil {
			t.Fatalf("Feed(%.40s…): %v", l, err)
		}
		out = append(out, evs...)
	}
	return out
}

func entryTime(t *testing.T, line string) time.Time {
	t.Helper()
	var e struct {
		Timestamp time.Time `json:"timestamp"`
	}
	if err := json.Unmarshal([]byte(line), &e); err != nil {
		t.Fatal(err)
	}
	return e.Timestamp
}

func TestBashOKIsRunningThenDone(t *testing.T) {
	lines := fixture(t, "bash_ok")
	p := NewPairer()
	first, err := p.Feed(lines[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 {
		t.Fatalf("want 1 event after tool_use, got %d", len(first))
	}
	c, ok := first[0].(Command)
	if !ok || !c.Running || c.ID != "toolu_016K52KZmNdpVgy3YEinJpkS" {
		t.Fatalf("want running Command toolu_016K52KZmNdpVgy3YEinJpkS, got %#v", first[0])
	}
	if !strings.HasPrefix(c.Cmd, `plutil -p ~/Library/Preferences/MobileMeAccounts.plist`) {
		t.Fatalf("Cmd = %q", c.Cmd)
	}
	if !c.When().Equal(entryTime(t, lines[0])) {
		t.Fatalf("When() = %v, want the tool_use entry timestamp %v", c.When(), entryTime(t, lines[0]))
	}
	second, err := p.Feed(lines[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 {
		t.Fatalf("want 1 event after tool_result, got %d", len(second))
	}
	d := second[0].(Command)
	if d.ID != c.ID || d.Running || !d.ExitKnown || d.Exit != 0 || d.Background {
		t.Fatalf("got %#v", d)
	}
	if !strings.HasPrefix(d.Stdout, `          "Name" => "KEYCHAIN_SYNC"`) || d.Stderr != "" {
		t.Fatalf("Stdout=%q Stderr=%q", d.Stdout, d.Stderr)
	}
}

func TestBashExitNonZero(t *testing.T) {
	evs := feedAll(t, NewPairer(), fixture(t, "bash_exit_nonzero"))
	c := evs[len(evs)-1].(Command)
	if c.Running || !c.ExitKnown || c.Exit != 2 {
		t.Fatalf("got %#v", c)
	}
	if !strings.HasPrefix(c.Stdout, "== homebrew ==\n") {
		t.Fatalf("the 'Exit code 2' line must be stripped from output, got %q", c.Stdout[:40])
	}
}

func TestBashRejected(t *testing.T) {
	evs := feedAll(t, NewPairer(), fixture(t, "bash_rejected"))
	r, ok := evs[len(evs)-1].(Rejected)
	if !ok {
		t.Fatalf("want Rejected, got %#v", evs[len(evs)-1])
	}
	if r.Tool != "Bash" || r.ID != "toolu_01LaUowDbsUNG69kvtAUWYQb" || !strings.HasPrefix(r.Subject, "F=/tmp/it-hang.log") {
		t.Fatalf("got %#v", r)
	}
}

func TestBashBackground(t *testing.T) {
	evs := feedAll(t, NewPairer(), fixture(t, "bash_background"))
	c := evs[len(evs)-1].(Command)
	if !c.Background || c.Running || !c.ExitKnown || c.Exit != 0 {
		t.Fatalf("got %#v", c)
	}
	if !strings.HasPrefix(c.Stdout, "Command did not complete within its 120s timeout") {
		t.Fatalf("empty stdout/stderr must fall back to the result text, got %q", c.Stdout)
	}
}

func TestEditUpdate(t *testing.T) {
	lines := fixture(t, "edit_update")
	p := NewPairer()
	first, _ := p.Feed(lines[0])
	if len(first) != 0 {
		t.Fatalf("an Edit tool_use emits nothing until its result, got %d", len(first))
	}
	evs := feedAll(t, p, lines[1:])
	fc, ok := evs[0].(FileChange)
	if !ok {
		t.Fatalf("want FileChange, got %#v", evs[0])
	}
	if fc.Kind != Update || fc.Path != "/Users/amaya/example/repo/docs/investigation.md" || fc.ID != "toolu_01YRTFb7ZG8zCyRQdw4YHfDJ" {
		t.Fatalf("got %#v", fc)
	}
	if len(fc.Hunks) != 1 {
		t.Fatalf("want 1 hunk, got %d", len(fc.Hunks))
	}
	h := fc.Hunks[0]
	if h.OldStart != 161 || h.OldLines != 7 || h.NewStart != 161 || h.NewLines != 7 || len(h.Lines) != 8 {
		t.Fatalf("got %#v", h)
	}
	if !strings.HasPrefix(h.Lines[3], "-| 18:26 | 278") || !strings.HasPrefix(h.Lines[4], "+| 18:26 | 452") {
		t.Fatalf("lines[3]=%q lines[4]=%q", h.Lines[3], h.Lines[4])
	}
	if !fc.When().Equal(entryTime(t, lines[0])) {
		t.Fatalf("When() must be the tool_use time")
	}
}

func TestEditRejected(t *testing.T) {
	evs := feedAll(t, NewPairer(), fixture(t, "edit_rejected"))
	r, ok := evs[0].(Rejected)
	if !ok || r.Tool != "Edit" || r.Subject != "/Users/amaya/example/repo/hooks/test_guard.py" {
		t.Fatalf("got %#v", evs[0])
	}
}

func TestWriteCreateCarriesContent(t *testing.T) {
	evs := feedAll(t, NewPairer(), fixture(t, "write_create"))
	fc := evs[0].(FileChange)
	if fc.Kind != Create || !strings.HasSuffix(fc.Path, "/scratchpad/vars_probe.yaml") {
		t.Fatalf("got %#v", fc)
	}
	if len(fc.Hunks) != 0 {
		t.Fatalf("create has no hunks, got %d", len(fc.Hunks))
	}
	if n := strings.Count(fc.Content, "\n") + 1; n != 8 {
		t.Fatalf("content lines = %d, want 8", n)
	}
}

func TestWriteUpdateHasHunks(t *testing.T) {
	evs := feedAll(t, NewPairer(), fixture(t, "write_update"))
	fc := evs[0].(FileChange)
	if fc.Kind != Update || len(fc.Hunks) == 0 || fc.Hunks[0].OldStart != 1 || fc.Hunks[0].OldLines != 19 || fc.Hunks[0].NewLines != 11 {
		t.Fatalf("got %#v", fc)
	}
	if fc.Content != "" {
		t.Fatalf("update must not carry the whole content, got %d bytes", len(fc.Content))
	}
}

func TestPromptsIncludedAndExcluded(t *testing.T) {
	cases := []struct {
		fixture string
		want    bool
	}{
		{"prompt_plain", true},
		{"prompt_textblock", true},
		{"prompt_task_notification", false},
		{"prompt_command_name", false},
		{"prompt_local_command_stdout", false},
		{"prompt_meta", false},
	}
	for _, c := range cases {
		evs := feedAll(t, NewPairer(), fixture(t, c.fixture))
		got := len(evs) == 1
		if got != c.want {
			t.Errorf("%s: emitted=%v want %v (%#v)", c.fixture, got, c.want, evs)
			continue
		}
		if c.want {
			p, ok := evs[0].(Prompt)
			if !ok || strings.TrimSpace(p.Text) == "" || strings.HasPrefix(strings.TrimSpace(p.Text), "<") {
				t.Errorf("%s: got %#v", c.fixture, evs[0])
			}
		}
	}
}

func TestCompactSummaryIsNotAPrompt(t *testing.T) {
	line := `{"type":"user","isCompactSummary":true,"uuid":"u1","timestamp":"2026-09-10T10:00:00.000Z","message":{"role":"user","content":"This session is being continued from a previous conversation"}}`
	evs, err := NewPairer().Feed(line)
	if err != nil || len(evs) != 0 {
		t.Fatalf("got %v %v", evs, err)
	}
}

func TestGarbageIsSkippedAndCounted(t *testing.T) {
	p := NewPairer()
	evs, err := p.Feed("{not json")
	if err == nil || evs != nil {
		t.Fatalf("want error and no events, got %v %v", evs, err)
	}
	if p.Skipped() != 1 {
		t.Fatalf("Skipped() = %d", p.Skipped())
	}
	if evs, err := p.Feed(""); err != nil || len(evs) != 0 {
		t.Fatalf("blank line must be ignored silently, got %v %v", evs, err)
	}
	if p.Skipped() != 1 {
		t.Fatalf("blank line must not count, Skipped() = %d", p.Skipped())
	}
}

func TestUnknownToolsAndOtherEntryTypesAreIgnored(t *testing.T) {
	lines := []string{
		`{"type":"assistant","uuid":"a1","timestamp":"2026-09-10T10:00:00.000Z","message":{"role":"assistant","content":[{"type":"text","text":"prose"},{"type":"tool_use","id":"t1","name":"Read","input":{"file_path":"/x"}}]}}`,
		`{"type":"user","uuid":"u1","timestamp":"2026-09-10T10:00:01.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"..."}]},"toolUseResult":{"type":"text","file":{}}}`,
		`{"type":"ai-title","sessionId":"s","aiTitle":"x"}`,
		`{"type":"system","subtype":"turn_duration","uuid":"s1"}`,
	}
	evs := feedAll(t, NewPairer(), lines)
	if len(evs) != 0 {
		t.Fatalf("want nothing, got %#v", evs)
	}
}
