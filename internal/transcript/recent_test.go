package transcript

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeSession creates <dir>/<slug>/<id>.jsonl with a first entry carrying cwd,
// an optional prompt line, and the given modification time.
func writeSession(t *testing.T, dir, slug, id, cwd, prompt string, mod time.Time) string {
	t.Helper()
	p := filepath.Join(dir, slug, id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	// Real transcripts often open with an entry that carries no cwd (a
	// snapshot or summary line); the cwd comes from a later entry.
	body := `{"type":"file-history-snapshot","messageId":"s0","snapshot":{"trackedFileBackups":{}},"isSnapshotUpdate":false}` + "\n"
	body += `{"type":"assistant","uuid":"a0","timestamp":"2026-09-10T10:00:00.000Z","cwd":"` + cwd + `","isSidechain":false,"message":{"role":"assistant","content":[{"type":"text","text":"hi"}]}}` + "\n"
	body += `{"type":"user","uuid":"m0","timestamp":"2026-09-10T10:00:01.000Z","isMeta":true,"message":{"role":"user","content":"<local-command-caveat>x"}}` + "\n"
	if prompt != "" {
		body += `{"type":"user","uuid":"u0","timestamp":"2026-09-10T10:00:02.000Z","message":{"role":"user","content":"` + prompt + `"}}` + "\n"
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mod, mod); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRecentListsMainSessionsNewestFirst(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 9, 13, 9, 0, 0, 0, time.UTC)
	old := writeSession(t, dir, "-Users-amaya-old", "aaaaaaaa-0000-0000-0000-000000000000", "/Users/amaya/old", "first prompt of old\\nsecond line", base.Add(-2*time.Hour))
	mid := writeSession(t, dir, "-Users-amaya-mid", "bbbbbbbb-0000-0000-0000-000000000000", "/Users/amaya/mid", "", base.Add(-time.Hour))
	newest := writeSession(t, dir, "-Users-amaya-new", "cccccccc-0000-0000-0000-000000000000", "/Users/amaya/new", "make me a poc", base)
	// a subagent transcript one level deeper is never a candidate
	sub := filepath.Join(dir, "-Users-amaya-new", "cccccccc-0000-0000-0000-000000000000", "subagents", "agent-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(sub), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sub, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Recent(dir, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Path != newest || got[1].Path != mid || got[2].Path != old {
		t.Fatalf("order = %+v", got)
	}
	if got[0].ID != "cccccccc-0000-0000-0000-000000000000" || got[0].Cwd != "/Users/amaya/new" || got[0].FirstPrompt != "make me a poc" || !got[0].ModTime.Equal(base) {
		t.Fatalf("newest = %+v", got[0])
	}
	if got[1].FirstPrompt != "" { // no prompt yet: empty, not an error
		t.Fatalf("mid = %+v", got[1])
	}
	if got[2].FirstPrompt != "first prompt of old" { // first line only
		t.Fatalf("old prompt = %q", got[2].FirstPrompt)
	}
	// the limit applies after sorting
	two, err := Recent(dir, 2)
	if err != nil || len(two) != 2 || two[1].Path != mid {
		t.Fatalf("limit: %v %+v", err, two)
	}
}

func TestRecentEmptyAndMissingDir(t *testing.T) {
	got, err := Recent(t.TempDir(), 5)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty dir: %v %+v", err, got)
	}
	if _, err := Recent(filepath.Join(t.TempDir(), "missing"), 5); err == nil {
		t.Fatal("a missing projects dir must be an error, not an empty list")
	}
}
