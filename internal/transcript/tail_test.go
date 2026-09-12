package transcript

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const poll = 10 * time.Millisecond

func next(t *testing.T, ch <-chan Line, wait time.Duration) (Line, bool) {
	t.Helper()
	select {
	case l, ok := <-ch:
		return l, ok
	case <-time.After(wait):
		return Line{}, false
	}
}

func mustNext(t *testing.T, ch <-chan Line) string {
	t.Helper()
	l, ok := next(t, ch, time.Second)
	if !ok {
		t.Fatal("no line within 1s")
	}
	if l.Err != nil {
		t.Fatal(l.Err)
	}
	return l.Text
}

func TestTailEmitsExistingLinesThenNewOnes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte("{\"a\":1}\n{\"a\":2}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := Tail(ctx, p, poll)
	if got := mustNext(t, ch); got != `{"a":1}` {
		t.Fatalf("got %q", got)
	}
	if got := mustNext(t, ch); got != `{"a":2}` {
		t.Fatalf("got %q", got)
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{\"a\":3}\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if got := mustNext(t, ch); got != `{"a":3}` {
		t.Fatalf("got %q", got)
	}
}

func TestTailHoldsPartialLineUntilNewline(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte("{\"a\":1}\n{\"par"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := Tail(ctx, p, poll)
	if got := mustNext(t, ch); got != `{"a":1}` {
		t.Fatalf("got %q", got)
	}
	if l, ok := next(t, ch, 5*poll); ok {
		t.Fatalf("partial line must not be emitted, got %q", l.Text)
	}
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("tial\":true}\n")
	f.Close()
	if got := mustNext(t, ch); got != `{"partial":true}` {
		t.Fatalf("got %q", got)
	}
}

func TestTailRestartsAfterTruncation(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte("{\"old\":1}\n{\"old\":2}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := Tail(ctx, p, poll)
	mustNext(t, ch)
	mustNext(t, ch)
	if err := os.WriteFile(p, []byte("{\"new\":1}\n"), 0o600); err != nil { // truncates to a shorter file
		t.Fatal(err)
	}
	if got := mustNext(t, ch); got != `{"new":1}` {
		t.Fatalf("got %q", got)
	}
}

func TestTailClosesOnCancel(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch := Tail(ctx, p, poll)
	mustNext(t, ch)
	cancel()
	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("channel not closed after cancel")
		}
	}
}

func TestTailReportsOpenError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := Tail(ctx, filepath.Join(t.TempDir(), "missing.jsonl"), poll)
	l, ok := next(t, ch, time.Second)
	if !ok || l.Err == nil {
		t.Fatalf("want an error line, got ok=%v line=%+v", ok, l)
	}
}
