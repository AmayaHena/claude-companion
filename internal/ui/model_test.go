package ui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"claude-companion/internal/event"
	"claude-companion/internal/transcript"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// content is the viewport text with styling removed.
func content(m Model) string { return ansi.ReplaceAllString(m.vp.GetContent(), "") }

func newModel() Model {
	return New(Config{SessionID: "faeeadc5", Path: "/tmp/x.jsonl", Loc: time.UTC})
}

func sized(m Model, w, h int) Model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(Model)
}

func promptLine(i int) string {
	return fmt.Sprintf(`{"type":"user","uuid":"u%d","timestamp":"2026-09-10T10:00:%02d.000Z","message":{"role":"user","content":"prompt number %d"}}`, i, i%60, i)
}

func feed(m Model, lines ...string) Model {
	for _, l := range lines {
		next, _ := m.Update(lineMsg{Text: l})
		m = next.(Model)
	}
	return m
}

func key(m Model, k tea.Key) (Model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyPressMsg(k))
	return next.(Model), cmd
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestQuitKeys(t *testing.T) {
	for _, k := range []tea.Key{{Code: 'q', Text: "q"}, {Code: tea.KeyEscape}, {Code: 'c', Mod: tea.ModCtrl}} {
		_, cmd := key(sized(newModel(), 80, 20), k)
		if !isQuit(cmd) {
			t.Errorf("%s must quit", tea.KeyPressMsg(k))
		}
	}
	_, cmd := key(sized(newModel(), 80, 20), tea.Key{Code: 'x', Text: "x"})
	if isQuit(cmd) {
		t.Error("x must not quit")
	}
}

func TestFollowsTailUntilScrolledUp(t *testing.T) {
	m := sized(newModel(), 80, 10)
	for i := 0; i < 30; i++ {
		u, r := bashPair(i, 1)
		m = feed(m, u, r)
	}
	if !m.follow || !m.vp.AtBottom() {
		t.Fatalf("must follow the tail: follow=%v atBottom=%v", m.follow, m.vp.AtBottom())
	}
	m, _ = key(m, tea.Key{Code: tea.KeyUp})
	if m.follow {
		t.Fatal("scrolling up must pause following")
	}
	before := m.vp.YOffset()
	u, r := bashPair(31, 1)
	m = feed(m, u, r)
	if m.vp.YOffset() != before {
		t.Fatalf("paused view must not move on new lines: %d -> %d", before, m.vp.YOffset())
	}
	m, _ = key(m, tea.Key{Code: tea.KeyEnd})
	if !m.follow || !m.vp.AtBottom() {
		t.Fatal("end must resume following at the bottom")
	}
	m, _ = key(m, tea.Key{Code: tea.KeyHome})
	if m.follow || m.vp.YOffset() != 0 {
		t.Fatalf("home must go to the top and pause: follow=%v y=%d", m.follow, m.vp.YOffset())
	}
}

func TestRunningCommandIsReplacedInPlace(t *testing.T) {
	m := sized(newModel(), 120, 20)
	use := `{"type":"assistant","uuid":"a1","timestamp":"2026-09-10T10:00:00.000Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"echo hi"}}]}}`
	res := `{"type":"user","uuid":"u1","timestamp":"2026-09-10T10:00:01.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"hi\n","is_error":false}]},"toolUseResult":{"stdout":"hi\n","stderr":"","interrupted":false}}`
	m = feed(m, use)
	if len(m.events) != 1 || !strings.Contains(content(m), "…") {
		t.Fatalf("want one running event, got %d events, content %q", len(m.events), content(m))
	}
	m = feed(m, res)
	if len(m.events) != 1 {
		t.Fatalf("result must replace the running command, got %d events", len(m.events))
	}
	if c := content(m); strings.Contains(c, "…") || !strings.Contains(c, "echo hi") || !strings.Contains(c, "      ▕▏hi") {
		t.Fatalf("content = %q", c)
	}
}

func TestSkippedAndErrorsReachTheFooter(t *testing.T) {
	m := sized(newModel(), 80, 20)
	m = feed(m, "{garbage")
	if !strings.Contains(m.footer(), "1 skipped") {
		t.Fatalf("footer = %q", m.footer())
	}
	next, _ := m.Update(lineMsg(transcript.Line{Err: fmt.Errorf("file gone")}))
	m = next.(Model)
	if !strings.Contains(m.footer(), "file gone") {
		t.Fatalf("footer = %q", m.footer())
	}
}

func TestPromptClearsEverythingBefore(t *testing.T) {
	m := sized(newModel(), 120, 20)
	for i := 0; i < 10; i++ {
		u, r := bashPair(i, 1)
		m = feed(m, u, r)
	}
	if len(m.events) != 10 {
		t.Fatalf("setup: %d events", len(m.events))
	}
	m = feed(m, promptLine(1))
	if len(m.events) != 1 {
		t.Fatalf("a prompt must clear the log; got %d events", len(m.events))
	}
	if _, ok := m.events[0].(event.Prompt); !ok {
		t.Fatalf("remaining event must be the prompt, got %#v", m.events[0])
	}
	c := content(m)
	if strings.Contains(c, "cmd 0") || !strings.Contains(c, "prompt number 1") {
		t.Fatalf("content = %q", c)
	}
	// and a running command started after the prompt is still replaced in place
	u, r := bashPair(99, 1)
	m = feed(m, u, r)
	if len(m.events) != 2 || m.events[1].(event.Command).Running {
		t.Fatalf("events = %#v", m.events)
	}
}

func TestTextStaysSelectable(t *testing.T) {
	if v := newModel().View(); v.MouseMode != tea.MouseModeNone {
		t.Fatalf("mouse reporting must be off so the terminal keeps text selection, got %v", v.MouseMode)
	}
}

// Consecutive commands share one bar colour; a file creation has its own.
func TestBarColourIsFixedPerActionType(t *testing.T) {
	m := sized(newModel(), 120, 30)
	u0, r0 := bashPair(0, 1)
	u1, r1 := bashPair(1, 1)
	write := `{"type":"assistant","uuid":"a9","timestamp":"2026-09-10T10:00:00.000Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"w9","name":"Write","input":{"file_path":"/tmp/n.txt","content":"x"}}]}}`
	wres := `{"type":"user","uuid":"u9","timestamp":"2026-09-10T10:00:01.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"w9","content":"ok"}]},"toolUseResult":{"type":"create","filePath":"/tmp/n.txt","content":"x","structuredPatch":[]}}`
	m = feed(m, u0, r0, u1, r1, write, wres)
	c := m.vp.GetContent()
	if strings.Count(c, "\x1b[33m▕▏") != 2 || strings.Contains(c, "\x1b[93m") || !strings.Contains(c, "\x1b[36m▕▏") {
		t.Fatalf("bars: yellow=%d bright=%v cyan=%v", strings.Count(c, "\x1b[33m▕▏"), strings.Contains(c, "\x1b[93m"), strings.Contains(c, "\x1b[36m▕▏"))
	}
}

// The footer is a tally of the work since the last prompt: the share of each
// action glyph in percent, then plain counts for the warning, network, agent
// and skill marks; zero entries are omitted. No session id, no follow state.
// A prompt resets the tally with the log.
func TestFooterShowsActionRatios(t *testing.T) {
	m := sized(newModel(), 120, 30)
	if !strings.Contains(m.footer(), "0 actions") {
		t.Fatalf("empty footer = %q", m.footer())
	}
	u0, r0 := bashPair(0, 1)
	u1, r1 := bashPair(1, 1)
	m = feed(m, u0, r0, u1, r1)
	failed := `{"type":"assistant","uuid":"a5","timestamp":"2026-09-10T10:00:00.000Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"b5","name":"Bash","input":{"command":"false"}}]}}`
	failedRes := `{"type":"user","uuid":"u5","timestamp":"2026-09-10T10:00:01.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"b5","is_error":true,"content":"Exit code 1\n"}]},"toolUseResult":"Error: Exit code 1"}`
	write := `{"type":"assistant","uuid":"a9","timestamp":"2026-09-10T10:00:00.000Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"w9","name":"Write","input":{"file_path":"/tmp/n.txt","content":"x"}}]}}`
	wres := `{"type":"user","uuid":"u9","timestamp":"2026-09-10T10:00:01.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"w9","content":"ok"}]},"toolUseResult":{"type":"create","filePath":"/tmp/n.txt","content":"x","structuredPatch":[]}}`
	m = feed(m, failed, failedRes, write, wres)
	f := m.footer()
	pf := ansi.ReplaceAllString(f, "") // the text, styling removed
	for _, want := range []string{"⚒️ 50%", "❗ 25%", "🆕 25%"} {
		if !strings.Contains(pf, want) {
			t.Errorf("footer %q lacks %q", pf, want)
		}
	}
	for _, no := range []string{"faeeadc5", "following", "paused", "📁", "⚠️", "🛜"} {
		if strings.Contains(pf, no) {
			t.Errorf("footer %q must not contain %q", pf, no)
		}
	}
	if !strings.Contains(pf, "  ·  ") {
		t.Errorf("footer keeps the dot separator: %q", pf)
	}
	// marks are plain counts; a running command is counted from its tool_use
	push := `{"type":"assistant","uuid":"a6","timestamp":"2026-09-10T10:00:00.000Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"b6","name":"Bash","input":{"command":"git push origin main"}}]}}`
	curl := `{"type":"assistant","uuid":"a7","timestamp":"2026-09-10T10:00:00.000Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"b7","name":"Bash","input":{"command":"curl -s https://example.test"}}]}}`
	agent := `{"type":"assistant","uuid":"a8","timestamp":"2026-09-10T10:00:00.000Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"g8","name":"Agent","input":{"subagent_type":"general-purpose","description":"Look around","prompt":"x"}}]}}`
	skill := `{"type":"assistant","uuid":"a10","timestamp":"2026-09-10T10:00:00.000Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"s10","name":"Skill","input":{"skill":"graft"}}]}}`
	m = feed(m, push, curl, agent, skill)
	f = m.footer()
	pf = ansi.ReplaceAllString(f, "")
	for _, want := range []string{"⚒️ 67%", "❗ 17%", "🆕 17%", "⚠️ 1", "🛜 1", "🤖 1", "ℹ️ 1"} {
		if !strings.Contains(pf, want) {
			t.Errorf("footer %q lacks %q", pf, want)
		}
	}
	// the result of the push replaces the running command without double counting
	pushRes := `{"type":"user","uuid":"u6","timestamp":"2026-09-10T10:00:01.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"b6","content":""}]},"toolUseResult":{"stdout":"","stderr":"","interrupted":false}}`
	m = feed(m, pushRes)
	if g := m.footer(); g != f {
		t.Errorf("replacement changed the tally:\n%q\n%q", f, g)
	}
	// numbers are white; the warning count red, the agent count blue, the skill count green
	for _, want := range []string{"\x1b[37m67%", "\x1b[37m17%", "\x1b[31m1\x1b[m", "\x1b[34m1\x1b[m", "\x1b[32m1\x1b[m"} {
		if !strings.Contains(f, want) {
			t.Errorf("footer %q lacks %q", f, want)
		}
	}
	// a prompt resets the tally with the log
	m = feed(m, promptLine(1))
	if len(m.events) != 1 || !strings.Contains(m.footer(), "0 actions") || strings.Contains(m.footer(), "%") {
		t.Errorf("after prompt: events=%d footer=%q", len(m.events), m.footer())
	}
	// a skipped line still shows
	m = feed(m, "{garbage")
	if !strings.Contains(m.footer(), "1 skipped") {
		t.Errorf("footer = %q", m.footer())
	}
}

// c copies the targeted command to the clipboard (OSC 52 through Bubble
// Tea); tab / shift+tab move the target to older commands; the footer names
// the target when it is not the latest and confirms a copy.
func TestCopyCommandKeys(t *testing.T) {
	m := sized(newModel(), 120, 30)
	m, cmd := key(m, tea.Key{Code: 'c', Text: "c"})
	if cmd != nil || !strings.Contains(m.footer(), "nothing to copy") {
		t.Fatalf("empty: cmd=%v footer=%q", cmd, m.footer())
	}
	u0, r0 := bashPair(0, 1)
	u1, r1 := bashPair(1, 1)
	m = feed(m, u0, r0, u1, r1)
	m, cmd = key(m, tea.Key{Code: 'c', Text: "c"})
	if cmd == nil {
		t.Fatal("c must return a clipboard command")
	}
	if msg := cmd(); fmt.Sprintf("%T", msg) != "tea.setClipboardMsg" || fmt.Sprint(msg) != "cmd 1" {
		t.Fatalf("clipboard msg = %T %q", msg, fmt.Sprint(msg))
	}
	if !strings.Contains(m.footer(), "copied") {
		t.Fatalf("footer after copy = %q", m.footer())
	}
	m, _ = key(m, tea.Key{Code: tea.KeyTab})
	if f := ansi.ReplaceAllString(m.footer(), ""); !strings.Contains(f, "copy → cmd 0") {
		t.Fatalf("footer after tab = %q", f)
	}
	m, _ = key(m, tea.Key{Code: tea.KeyTab}) // stays on the oldest
	m, cmd = key(m, tea.Key{Code: 'c', Text: "c"})
	if msg := cmd(); fmt.Sprint(msg) != "cmd 0" {
		t.Fatalf("copied %q, want the targeted older command", fmt.Sprint(msg))
	}
	m, _ = key(m, tea.Key{Code: tea.KeyTab, Mod: tea.ModShift})
	if f := ansi.ReplaceAllString(m.footer(), ""); strings.Contains(f, "copy →") {
		t.Fatalf("back on the latest, no target shown: %q", f)
	}
	// a new command resets the target to the latest and clears the confirmation
	m, _ = key(m, tea.Key{Code: tea.KeyTab})
	u2, r2 := bashPair(2, 1)
	m = feed(m, u2, r2)
	if f := ansi.ReplaceAllString(m.footer(), ""); strings.Contains(f, "copy →") || strings.Contains(f, "copied") {
		t.Fatalf("footer after new command = %q", f)
	}
	m, cmd = key(m, tea.Key{Code: 'c', Text: "c"})
	if msg := cmd(); fmt.Sprint(msg) != "cmd 2" {
		t.Fatalf("copied %q, want the latest", fmt.Sprint(msg))
	}
}
