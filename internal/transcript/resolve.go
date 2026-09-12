// Package transcript locates a Claude Code session transcript and follows it
// as it grows. It never writes.
package transcript

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Resolve finds the transcript for a session id or a unique id prefix under
// projectsDir (normally ~/.claude/projects). Only files directly inside a
// project folder are candidates; subagent transcripts live one level deeper
// and are excluded by construction of the glob.
func Resolve(projectsDir, arg string) (string, error) {
	if arg == "" {
		return "", fmt.Errorf("no session id given")
	}
	matches, err := filepath.Glob(filepath.Join(projectsDir, "*", arg+"*.jsonl"))
	if err != nil {
		return "", err
	}
	var candidates []string
	for _, m := range matches {
		if strings.HasPrefix(filepath.Base(m), arg) {
			candidates = append(candidates, m)
		}
	}
	switch len(candidates) {
	case 0:
		return "", fmt.Errorf("no session matches %q under %s", arg, projectsDir)
	case 1:
		return candidates[0], nil
	}
	sort.Strings(candidates)
	names := make([]string, len(candidates))
	for i, c := range candidates {
		names[i] = strings.TrimSuffix(filepath.Base(c), ".jsonl")
	}
	return "", fmt.Errorf("%q matches %d sessions, give more of the id:\n  %s", arg, len(candidates), strings.Join(names, "\n  "))
}
