# claude-companion

A read-only terminal viewer for one Claude Code session. It shows what Claude
*does*: the commands it runs with their output, and the files it changes with
their diffs, live, as the session goes. It never shows the assistant's answers.

```
claude-companion <session-id | unique prefix>
```

Keys: `q`, `esc`, `ctrl+c` quit. `↑ ↓`, `j k`, `pgup pgdn`, `home end` scroll.
The mouse wheel scrolls too: the tool does not capture the mouse, so the
terminal keeps text selection and translates wheel motion into arrow keys. Newest at the bottom; the view follows the tail until you
scroll up, and resumes when you scroll back down or press `end`. The footer
is a tally of the whole session: the share of each action glyph in percent,
then plain counts of git warnings `⚠️` (red), network commands `🛜`, subagent
launches `🤖` (blue) and skills `ℹ️` (green), and any lines that could not be
parsed. A new prompt clears the screen and the tally.

## What it reads

The session transcript Claude Code writes at
`~/.claude/projects/<slug>/<session-id>.jsonl` (or under `$CLAUDE_CONFIG_DIR`
when set, the same override Claude Code honours). The file is opened read-only
and polled every 250 ms. Nothing is written anywhere, nothing touches the
network. Subagents write to separate files and are not shown.

Each action starts with a header row: a faded clock (`HH:MM`), the glyph and
the title. Output and diff rows below it carry a coloured bar under the
glyph: yellow for commands, magenta for file changes, each in a dark and a
light shade that alternate when two actions of the same type follow each
other, so the hue names the type and the shade separates neighbours. Long commands and prompts wrap onto
bar-free continuation rows; output and diff lines are cut at the width. A blank line ends each action. A new prompt clears
the screen and starts a new block, so what you see is always the work done
since your last message.

| row | meaning |
|---|---|
| `⚒` command | a Bash tool call, the command syntax-highlighted; output below it, stderr in red |
| `❗` with `exit N` | the command failed; `exit ?` when the code is unknown |
| `⚒ … ` | still running (result not in the transcript yet); `bg` = moved to background |
| `📁` path | an Edit or a Write to an existing file, followed by its hunks |
| `🆕` path | a Write that created the file; its content shown as added lines |
| `❗` `rejected` | a tool use you refused in Claude Code |
| `⚠️ ⚒` underlined red | a command that writes git or GitHub state: `git add/commit/merge/rebase/cherry-pick/revert/reset/clean/tag/push…`, `gh pr create/merge`, `gh release create`, `gh api -X POST…` |
| `🛜 ⚒` | a command that reaches the network: `curl`, `wget`, `ssh`, `git fetch/pull/clone`, `gh`, `npm install`, `go get`, `brew install`, `docker pull`, `aws`, `kubectl`… |
| `🤖` description | a subagent launched by Claude, in blue, with its type; the subagent's own work is not shown |
| `ℹ️` skill | a skill invoked by Claude, in green, with its arguments |
| `HH:MM   text` | one of your prompts, bold, every line shown and wrapped |

Diff and file lines are syntax-highlighted from the file's extension, with
the `+` and `-` signs kept bold green and bold red. Highlighting uses chroma's
lexers with a fixed map to five palette colours (keywords magenta, strings
green, numbers cyan, comments dim, function and builtin names blue), so it
follows your terminal theme like everything else.

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
