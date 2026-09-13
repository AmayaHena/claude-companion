# claude-companion

A read-only terminal viewer for one Claude Code session. It shows what Claude
*does*: the commands it runs with their output, and the files it changes with
their diffs, live, as the session goes. It never shows the assistant's answers.

```
claude-companion [session-id | unique prefix]
```

Site: https://amayahena.github.io/claude-companion/ (served from `site/` by
GitHub Pages; goes live once the repository is public).

## Deterministic, offline, read-only

These three properties are the design, not features, and every change is held
to them.

- **Deterministic.** The screen is a pure function of the transcript file,
  the terminal width and your time zone. Same file, same width: same rows,
  every time. There is no randomness, no timer, no animation, no state kept
  between runs. The only place the wall clock is read is the "3 min ago"
  column of the session picker.
- **No network.** The binary links no socket-capable package: no `net`,
  `net/http` or TLS in its dependency graph (only the `net/url` parser, via
  the terminal library). No telemetry, no update check, no API call, no
  model. Everything on screen comes from bytes already on your disk. What
  leaves the process is terminal output to your own terminal, which includes
  the OSC 52 escape when you press `c` to copy a command; and, only when
  running inside tmux, the terminal library runs `tmux info` once at startup
  to read the colour capabilities.
- **Read-only.** Transcripts are opened for reading and polled by size. The
  tool writes no file, no cache, no config, and never touches the transcript
  or Claude Code's state; the terminal library's own debug files
  (`TEA_TRACE`, `TEA_DEBUG`) are disabled at startup. You can run it against
  a live session with nothing at risk.
- **Untrusted input.** A transcript is text an AI session wrote, including
  whatever a command printed. Every string from it is sanitised before it
  reaches the screen, the window title, the footer or the clipboard: escape
  characters are shown as `␛`, other control and invisible format
  characters are dropped. Bodies are capped at 2,000 rows per event and an
  unterminated line at 64 MB, so a hostile or huge transcript cannot freeze
  the viewer or drive your terminal.

If a change needs a clock, a socket or a write to work, the change is wrong
for this tool.

### Audited, with checks you can rerun

Before v1 the code went through a five-lens audit (security, determinism and
no model calls, read-only, no network, privacy), fifteen independent agents,
every finding handed to a skeptic instructed to refute it. Ten findings were
verified, none refuted, all fixed or documented below. The claims above are
backed by tests in the repository and by commands anyone can run:

| claim | proof in the repo | rerun it yourself |
|---|---|---|
| no network | no `net`, `net/http` or TLS package in the binary's dependency graph | `go list -deps ./cmd/claude-companion \| grep -E '^(net\|net/http\|crypto/tls)$'` prints nothing |
| read-only | four filesystem calls in product code, all reads; `TEA_TRACE`/`TEA_DEBUG` unset at startup | `grep -rn --exclude='*_test.go' 'os\.\(Create\|WriteFile\|OpenFile\|Mkdir\|Remove\)' cmd internal` prints nothing (tests write only in their temp dirs); run it and `lsof -p <pid>` shows the transcript opened `r` |
| deterministic | one wall-clock read (`time.Now` in `main`, for the picker's "ago"); footer order is a fixed list | `grep -rn 'time.Now\|math/rand' cmd internal` shows that single line |
| untrusted input is inert | `TestTerminalControlsNeverReachTheScreen`, `TestTitleAndErrorAreSanitised`, `TestPickerSanitisesTranscriptText`, `TestCopySanitisesAndCountsLines`, `TestMarksAreDecidedOnSanitisedText` | `go test ./... -run 'Sanitis\|Controls\|Marks'` |
| bounded work | `TestHugeBodiesAreCapped`, `TestTailDropsAnEndlessLine` | `go test ./... -run 'Capped\|Endless'` |
| no build-machine paths | `-trimpath` in the documented build | `go build -trimpath ./cmd/claude-companion && strings claude-companion \| grep -c "$HOME"` prints 0 |

Every guard was mutation-checked: removing it makes its test fail.

What the audit found and what was done:

- Escape sequences inside a transcript reached the terminal raw, enough to
  rename the tab, clear the screen or write the clipboard from a command's
  output. Fixed: every transcript string is sanitised before the screen, the
  title, the footer, the picker and the clipboard.
- The git warning was decided on raw text, so an invisible byte inside
  `git push` removed it. Fixed: marks are decided on the sanitised text.
- `c` copied a multi-line command while the screen showed one line. Fixed:
  the copy is sanitised and the footer says how many lines went out.
- An unterminated line grew memory without bound; a multi-million-line
  write froze the viewer. Fixed: 64 MB line cap, 2,000 rows per event.
- Bubble Tea writes trace and panic files when `TEA_TRACE` or `TEA_DEBUG` is
  set. Fixed: both unset at startup.
- A plain `go build` embedded the builder's home directory. Fixed: `-trimpath`.

Known and accepted:

- Inside tmux, the terminal library runs `tmux info` once at startup to read
  colour capabilities. No other subprocess exists.
- The tailer follows the opened file, not the path: a transcript replaced in
  place by a different file is not re-opened. Claude Code appends, it does
  not replace.
- The session picker reads at most the first 200 lines of the ten most recent
  transcripts to show a folder and a first prompt; nothing is kept.
- Test fixtures are cut from the author's own sessions and carry the
  author's home path and username, nothing else identifying.
- Everything in a transcript is shown as it is: a secret printed by a command
  is visible in the viewer exactly as it was in the session. The tool does
  not redact; it does not send anything anywhere.

## Use

Run it without an argument to pick one of the ten most recent sessions across
all projects: each row shows the id, the time since the last write, the
project folder and the first prompt. `↑ ↓` move, `enter` opens, `q` quits.
With an id or a unique prefix it opens that session directly. There is no
opening screen: the log shows from the first line.

Keys: `q`, `esc`, `ctrl+c` quit. `↑ ↓`, `j k`, `pgup pgdn`, `home end` scroll.
`c` copies the latest command to the clipboard; `tab` and `shift+tab` move the
copy target to older commands, the footer names it. The whole command is
copied, sanitised, even when the screen shows only its first line; the footer
says how many lines went out. The copy goes through the terminal's OSC 52
support; in Ghostty, `clipboard-write = allow` avoids a confirmation on each
copy.

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
go build -trimpath ./cmd/claude-companion
go test ./...
```

`-trimpath` keeps your build machine's paths out of the binary; a release
binary is built with it.

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

A self-contained demo, no real session needed: in one terminal
`sh demo/play.sh`, which replays a synthetic transcript with one event of each
kind, one line every 0.8 s; in another, right after,
`CLAUDE_CONFIG_DIR="$PWD/demo" claude-companion demo1111`.

Design: `docs/superpowers/specs/2026-09-10-claude-companion-design.md`.
Architecture: `docs/architecture.md`.
