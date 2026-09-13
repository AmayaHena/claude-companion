// claude-companion shows what a Claude Code session does: the commands it
// runs and the files it changes, live, without the assistant's prose.
//
// Usage: claude-companion [session-id | unique prefix]
//
// Without an argument a picker lists the most recent sessions.
//
// Transcripts are read from $CLAUDE_CONFIG_DIR/projects when that variable is
// set (the same override Claude Code honours), otherwise ~/.claude/projects.
// The tool only reads; it never writes and never touches the network.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"claude-companion/internal/transcript"
	"claude-companion/internal/ui"
)

func main() {
	if len(os.Args) > 2 || (len(os.Args) == 2 && (os.Args[1] == "-h" || os.Args[1] == "--help")) {
		fmt.Fprintln(os.Stderr, "usage: claude-companion [session-id | unique prefix]")
		fmt.Fprintln(os.Stderr, "without an argument, pick one of the most recent sessions")
		fmt.Fprintln(os.Stderr, "reads $CLAUDE_CONFIG_DIR/projects when set, else ~/.claude/projects")
		os.Exit(2)
	}
	configDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "claude-companion:", err)
			os.Exit(1)
		}
		configDir = filepath.Join(home, ".claude")
	}
	projects := filepath.Join(configDir, "projects")
	var path string
	if len(os.Args) == 2 {
		p, err := transcript.Resolve(projects, os.Args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "claude-companion:", err)
			os.Exit(1)
		}
		path = p
	} else {
		path = pick(projects)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := ui.Config{
		SessionID: strings.TrimSuffix(filepath.Base(path), ".jsonl"),
		Path:      path,
		Lines:     transcript.Tail(ctx, path, 100*time.Millisecond),
		Loc:       time.Local,
	}
	if _, err := tea.NewProgram(ui.New(cfg)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "claude-companion:", err)
		os.Exit(1)
	}
}

// recentLimit is how many sessions the picker lists.
const recentLimit = 10

// pick runs the session picker and returns the chosen transcript path; it
// exits the process when there is nothing to pick or the user quits.
func pick(projects string) string {
	sessions, err := transcript.Recent(projects, recentLimit)
	if err != nil {
		fmt.Fprintln(os.Stderr, "claude-companion:", err)
		os.Exit(1)
	}
	if len(sessions) == 0 {
		fmt.Fprintln(os.Stderr, "claude-companion: no sessions under", projects)
		os.Exit(1)
	}
	final, err := tea.NewProgram(ui.NewPicker(sessions, time.Now())).Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "claude-companion:", err)
		os.Exit(1)
	}
	chosen := final.(ui.Picker).Chosen
	if chosen == nil {
		os.Exit(0)
	}
	return chosen.Path
}
