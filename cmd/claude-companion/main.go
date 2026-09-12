// claude-companion shows what a Claude Code session does: the commands it
// runs and the files it changes, live, without the assistant's prose.
//
// Usage: claude-companion <session-id | unique prefix>
//
// Transcripts are read from $CLAUDE_CONFIG_DIR/projects when that variable is
// set (the same override Claude Code honours), otherwise ~/.claude/projects.
// The tool only reads; it never writes and never touches the network.
package main

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"claude-companion/internal/transcript"
	"claude-companion/internal/ui"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] == "-h" || os.Args[1] == "--help" {
		fmt.Fprintln(os.Stderr, "usage: claude-companion <session-id | unique prefix>")
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
	path, err := transcript.Resolve(filepath.Join(configDir, "projects"), os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "claude-companion:", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := ui.Config{
		User:      userName(),
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

func userName() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return os.Getenv("USER")
}
