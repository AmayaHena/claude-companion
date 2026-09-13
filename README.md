# claude-companion

A read-only terminal viewer for one Claude Code session. It shows what Claude
*does*: the commands it runs with their output, and the files it changes with
their diffs, live, as the session goes. It never shows the assistant's answers.

```
claude-companion [session-id | unique prefix]
```

## Deterministic, offline, read-only

These three properties are the design, not features, and every change is held
to them.

- **Deterministic.** The screen is a pure function of the transcript file,
  the terminal width and your time zone. Same file, same width: same rows,
  every time. There is no randomness, no timer, no animation, no state kept
  between runs. The only place the wall clock is read is the "3 min ago"
  column of the session picker.
- **No network.** The binary imports no network package and opens no socket.
  No telemetry, no update check, no API call, no model. Everything on screen
  comes from bytes already on your disk. The one thing that leaves the
  process is terminal output to your own terminal, which includes the OSC 52
  escape used when you press `c` to copy a command.
- **Read-only.** Transcripts are opened for reading and polled by size. The
  tool writes no file, no cache, no config, and never touches the transcript
  or Claude Code's state. You can run it against a live session with nothing
  at risk.

If a change needs a clock, a socket or a write to work, the change is wrong
for this tool.

## Use

Run it without an argument to pick one of the ten most recent sessions across
all projects: each row shows the id, the time since the last write, the
project folder and the first prompt. `↑ ↓` move, `enter` opens, `q` quits.
With an id or a unique prefix it opens that session directly. There is no
opening screen: the log shows from the first line.

Keys: `q`, `esc`, `ctrl+c` quit. `↑ ↓`, `j k`, `pgup pgdn`, `home end` scroll.
`c` copies the latest command to the clipboard; `tab` and `shift+tab` move the
copy target to older commands, the footer names it. The copy goes through the
terminal's OSC 52 support; in Ghostty, `clipboard-write = allow` avoids a
confirmation on each copy.

The tool does not capture the mouse, so the terminal keeps its own text
selection. Newest at the bottom; the view follows the tail until you scroll
up, and resumes when you scroll back down or press `end`. The footer is a
tally of the work since your last prompt: the share of each action glyph in
percent, then plain counts of git warnings `⚠️` (red), network commands `🛜`,
subagent launches `🤖` (blue) and skills `ℹ️` (green), and any lines that
could not be parsed. A new prompt clears the screen and the tally.

## What it reads

The session transcript Claude Code writes at
`~/.claude/projects/<slug>/<session-id>.jsonl` (or under `$CLAUDE_CONFIG_DIR`
when set, the same override Claude Code honours). The file is opened for
reading only and its size polled every 100 ms; a line is parsed only once its
newline has arrived, and a shrunken file is read again from the start.
Subagents write to separate files; only their launch line is shown.

Each action starts with a header row: a faded clock (`HH:MM`), the glyph and
the title. Output and diff rows below it carry a coloured bar under the
glyph, one hue per action: yellow for a command, red for a failed one,
magenta for an edit, cyan for a created file. Long commands and prompts wrap onto
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

Go 1.27. Direct dependencies: `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`,
`charm.land/bubbles/v2`, `github.com/alecthomas/chroma/v2` (lexers only) and
`github.com/charmbracelet/x/ansi`. None of them is used for anything that
reaches the network.

The tests are offline too: fixtures are two-line cuts of real transcripts with
the payloads neutralised, and the suite runs with no network and no home
directory access.

An opt-in replay against a real transcript:

```
CLAUDE_COMPANION_E2E_FILE=~/.claude/projects/<slug>/<id>.jsonl \
CLAUDE_COMPANION_E2E_EXPECT=<commands>,<files>,<rejected>,<prompts> \
go test ./internal/ui/ -run TestReplayRealSession -v
```

Design: `docs/superpowers/specs/2026-09-10-claude-companion-design.md`.
Architecture: `docs/architecture.md`.
