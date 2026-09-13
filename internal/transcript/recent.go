package transcript

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"claude-companion/internal/event"
)

// Session describes one main-session transcript for the picker.
type Session struct {
	ID          string    // basename without .jsonl
	Path        string    // transcript file
	Cwd         string    // cwd of the first entry, "" when absent
	FirstPrompt string    // first line of the first user prompt, "" when none yet
	ModTime     time.Time // last write to the file
}

// headLines bounds how much of a transcript Recent reads to find the cwd and
// the first prompt: the first entries of a session hold both.
const headLines = 200

// Recent lists the main-session transcripts directly under the project
// folders of projectsDir, newest write first, at most n of them. A missing
// projectsDir is an error; an empty one is an empty list.
func Recent(projectsDir string, n int) ([]Session, error) {
	if _, err := os.Stat(projectsDir); err != nil {
		return nil, err
	}
	matches, err := filepath.Glob(filepath.Join(projectsDir, "*", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	var out []Session
	for _, p := range matches {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		out = append(out, Session{ID: strings.TrimSuffix(filepath.Base(p), ".jsonl"), Path: p, ModTime: info.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].ModTime.Equal(out[j].ModTime) {
			return out[i].ModTime.After(out[j].ModTime)
		}
		return out[i].Path < out[j].Path
	})
	if len(out) > n {
		out = out[:n]
	}
	for i := range out {
		out[i].Cwd, out[i].FirstPrompt = head(out[i].Path)
	}
	if out == nil {
		out = []Session{}
	}
	return out, nil
}

// head reads the first headLines lines of a transcript and returns the cwd of
// the first entry that carries one (real files open with header entries such
// as mode, permission-mode or bridge-session that have none) and the first
// line of the first user prompt.
func head(path string) (cwd, prompt string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	p := event.NewPairer()
	for i := 0; i < headLines && sc.Scan(); i++ {
		line := sc.Text()
		if cwd == "" {
			var e struct {
				Cwd string `json:"cwd"`
			}
			_ = json.Unmarshal([]byte(line), &e)
			cwd = e.Cwd
		}
		evs, _ := p.Feed(line)
		for _, ev := range evs {
			if pr, ok := ev.(event.Prompt); ok {
				if i := strings.IndexByte(pr.Text, '\n'); i >= 0 {
					return cwd, pr.Text[:i]
				}
				return cwd, pr.Text
			}
		}
	}
	return cwd, ""
}

// String is the one-line form used in error messages.
func (s Session) String() string { return fmt.Sprintf("%s  %s", s.ID, s.Cwd) }
