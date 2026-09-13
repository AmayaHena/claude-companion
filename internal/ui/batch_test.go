package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/AmayaHena/claude-companion/internal/event"
	"github.com/AmayaHena/claude-companion/internal/transcript"
)

// bashPair returns a tool_use line and its result for command i with a
// stdout of `outLines` lines, mimicking the transcript shape.
func bashPair(i, outLines int) (string, string) {
	var sb strings.Builder
	for j := 0; j < outLines; j++ {
		fmt.Fprintf(&sb, "output line %d of command %d\\n", j, i)
	}
	use := fmt.Sprintf(`{"type":"assistant","uuid":"a%d","timestamp":"2026-09-10T10:00:00.000Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"t%d","name":"Bash","input":{"command":"cmd %d"}}]}}`, i, i, i)
	res := fmt.Sprintf(`{"type":"user","uuid":"u%d","timestamp":"2026-09-10T10:00:01.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t%d","content":"","is_error":false}]},"toolUseResult":{"stdout":"%s","stderr":"","interrupted":false}}`, i, i, sb.String())
	return use, res
}

func TestBatchedLinesAreIngestedTogether(t *testing.T) {
	m := sized(newModel(), 120, 20)
	var batch linesMsg
	for i := 0; i < 5; i++ {
		u, r := bashPair(i, 2)
		batch = append(batch, transcript.Line{Text: u}, transcript.Line{Text: r})
	}
	next, _ := m.Update(batch)
	m = next.(Model)
	if len(m.events) != 5 {
		t.Fatalf("want 5 commands from one batch, got %d", len(m.events))
	}
	c := content(m)
	if !strings.Contains(c, "cmd 0") || !strings.Contains(c, "cmd 4") || !strings.HasSuffix(strings.TrimRight(c, "\n"), "output line 1 of command 4") {
		t.Fatalf("content = %q", tailOf(c, 300))
	}
	if !m.vp.AtBottom() {
		t.Fatal("still following after a batch")
	}
}

func TestBatchWithErrorLineKeepsGoing(t *testing.T) {
	m := sized(newModel(), 120, 20)
	u, r := bashPair(1, 1)
	next, _ := m.Update(linesMsg{{Text: u}, {Err: fmt.Errorf("boom")}, {Text: r}})
	m = next.(Model)
	if len(m.events) != 1 || m.events[0].(event.Command).Running {
		t.Fatalf("events = %#v", m.events)
	}
	if !strings.Contains(m.footer(), "boom") {
		t.Fatalf("footer = %q", m.footer())
	}
}

// BenchmarkIngest measures feeding 2,000 lines (1,000 commands with 40
// output lines each) one message at a time, the shape of a long session.
func BenchmarkIngest(b *testing.B) {
	lines := make([]string, 0, 2000)
	for i := 0; i < 1000; i++ {
		u, r := bashPair(i, 40)
		lines = append(lines, u, r)
	}
	for b.Loop() {
		m := sized(newModel(), 160, 50)
		for _, l := range lines {
			next, _ := m.Update(lineMsg{Text: l})
			m = next.(Model)
		}
		if len(m.events) != 1000 {
			b.Fatal(len(m.events))
		}
	}
	_ = time.Now
}

// BenchmarkIngestBatched feeds the same 2,000 lines in batches of maxBatch,
// which is how waitLines delivers a backlog.
func BenchmarkIngestBatched(b *testing.B) {
	lines := make([]string, 0, 2000)
	for i := 0; i < 1000; i++ {
		u, r := bashPair(i, 40)
		lines = append(lines, u, r)
	}
	for b.Loop() {
		m := sized(newModel(), 160, 50)
		for start := 0; start < len(lines); start += maxBatch {
			end := min(start+maxBatch, len(lines))
			batch := make(linesMsg, 0, end-start)
			for _, l := range lines[start:end] {
				batch = append(batch, transcript.Line{Text: l})
			}
			next, _ := m.Update(batch)
			m = next.(Model)
		}
		if len(m.events) != 1000 {
			b.Fatal(len(m.events))
		}
	}
}
