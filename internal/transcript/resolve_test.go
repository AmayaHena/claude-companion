package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// projects lays out ~/.claude/projects-like folders: each key is a slug, each
// value the session basenames (without .jsonl) that folder contains.
func projects(t *testing.T, layout map[string][]string) string {
	t.Helper()
	root := t.TempDir()
	for slug, ids := range layout {
		dir := filepath.Join(root, slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, id := range ids {
			if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}

func TestResolveFullID(t *testing.T) {
	root := projects(t, map[string][]string{"-Users-amaya": {"faeeadc5-5b2d-4f2b-8b67-03973b1049d2"}})
	got, err := Resolve(root, "faeeadc5-5b2d-4f2b-8b67-03973b1049d2")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "-Users-amaya", "faeeadc5-5b2d-4f2b-8b67-03973b1049d2.jsonl")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestResolveUniquePrefix(t *testing.T) {
	root := projects(t, map[string][]string{
		"-Users-amaya":         {"faeeadc5-5b2d-4f2b-8b67-03973b1049d2"},
		"-Users-amaya-example": {"1a932618-8f31-444b-91f6-826342a8cb90"},
	})
	got, err := Resolve(root, "faee")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "faeeadc5-5b2d-4f2b-8b67-03973b1049d2.jsonl" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveNoMatch(t *testing.T) {
	root := projects(t, map[string][]string{"-Users-amaya": {"faeeadc5-5b2d-4f2b-8b67-03973b1049d2"}})
	_, err := Resolve(root, "zzz")
	if err == nil || !strings.Contains(err.Error(), "no session") {
		t.Fatalf("want 'no session' error, got %v", err)
	}
}

func TestResolveAmbiguousListsCandidates(t *testing.T) {
	root := projects(t, map[string][]string{
		"-Users-amaya":         {"1a000000-0000-0000-0000-000000000001"},
		"-Users-amaya-example": {"1a000000-0000-0000-0000-000000000002"},
	})
	_, err := Resolve(root, "1a")
	if err == nil {
		t.Fatal("want error")
	}
	for _, id := range []string{"1a000000-0000-0000-0000-000000000001", "1a000000-0000-0000-0000-000000000002"} {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("error %q does not list %s", err, id)
		}
	}
}

func TestResolveIgnoresSubagentFiles(t *testing.T) {
	// subagent transcripts live one level deeper: <slug>/<id>/subagents/agent-x.jsonl
	root := projects(t, map[string][]string{"-Users-amaya": {"agent-aaaa"}})
	deep := filepath.Join(root, "-Users-amaya", "aa808670", "subagents")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deep, "agent-bbbb.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(root, "agent-")
	if err != nil {
		t.Fatalf("subagent file must not count as a candidate: %v", err)
	}
	if filepath.Base(got) != "agent-aaaa.jsonl" {
		t.Fatalf("got %q", got)
	}
}
