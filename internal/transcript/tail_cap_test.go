package transcript

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A line that never ends must not grow the reader's buffer without bound:
// past maxLine the partial line is dropped with an error, and reading
// resumes at the next newline.
func TestTailDropsAnEndlessLine(t *testing.T) {
	old := maxLine
	maxLine = 1024
	defer func() { maxLine = old }()
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(strings.Repeat("x", 5000)+"\nok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := Tail(ctx, p, 5*time.Millisecond)
	var got []Line
	timeout := time.After(2 * time.Second)
	for len(got) < 2 {
		select {
		case l := <-ch:
			got = append(got, l)
		case <-timeout:
			t.Fatalf("timed out with %+v", got)
		}
	}
	if got[0].Err == nil || !strings.Contains(got[0].Err.Error(), "longer than") {
		t.Fatalf("first = %+v, want an error about the dropped line", got[0])
	}
	if got[1].Text != "ok" || got[1].Err != nil {
		t.Fatalf("second = %+v, want the next complete line", got[1])
	}
}
