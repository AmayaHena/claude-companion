# claude-companion

A read-only terminal viewer for one Claude Code session. It shows what Claude
*does*: the commands it runs with their output, and the files it changes with
their diffs, live, as the session goes. It never shows the assistant's answers.

```
claude-companion <session-id | unique prefix>
```

Keys: `q`, `esc`, `ctrl+c` quit. `↑ ↓`, `j k`, `pgup pgdn`, `home end` and the
mouse wheel scroll. Newest at the bottom; the view follows the tail until you
scroll up, and resumes when you scroll back down or press `end`. The footer
shows the session id, the event count, whether the view is following, and any
lines that could not be parsed.

## What it reads

The session transcript Claude Code writes at
`~/.claude/projects/<slug>/<session-id>.jsonl` (or under `$CLAUDE_CONFIG_DIR`
when set, the same override Claude Code honours). The file is opened read-only
and polled every 250 ms. Nothing is written anywhere, nothing touches the
network. Subagents write to separate files and are not shown.

| line | meaning |
|---|---|
| `⚒` command | a Bash tool call; output below it, stderr in red |
| `❗` with `exit N` | the command failed; `exit ?` when the code is unknown |
| `⚒ … ` | still running (result not in the transcript yet); `bg` = moved to background |
| `📁` path | an Edit or a Write to an existing file, followed by its hunks |
| `🆕` path | a Write that created the file; its content shown as added lines |
| `❗` `rejected` | a tool use you refused in Claude Code |
| `── HH:MM:SS  text ──` | one of your prompts, as a separator |

Colours are the terminal's own palette, so your Ghostty theme is what you see.

## Build

```
go build ./cmd/claude-companion
go test ./...
```

Go 1.27, `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `charm.land/bubbles/v2`.

An opt-in replay against a real transcript:

```
CLAUDE_COMPANION_E2E_FILE=~/.claude/projects/<slug>/<id>.jsonl \
CLAUDE_COMPANION_E2E_EXPECT=<commands>,<files>,<rejected>,<prompts> \
go test ./internal/ui/ -run TestReplayRealSession -v
```

Design: `docs/superpowers/specs/2026-09-10-claude-companion-design.md`.
Architecture: `docs/architecture.md`.
