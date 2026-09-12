# claude-companion — notes for the next Claude Code session

Audience: an AI agent working in this repository. Every statement below was
verified against the tree or by an executed experiment on 2026-09-13; when
something was not verified it says so. Re-verify before relying on a number.

## What this is

A read-only Bubble Tea terminal viewer. `claude-companion <session-id | unique
prefix>` tails one Claude Code session transcript and shows what Claude did:
commands with output, file edits and creations as diffs, refused tool uses,
and the user's prompts. Assistant prose and thinking are never shown. No
network, no writes, deterministic (no randomness anywhere; the greeting
animation is a fixed timer).

Owner: Amaya (`amaya` on this Mac). Repo: `github.com/AmayaHena/claude-companion`,
remote alias `github-personal` (the plain `github.com` SSH host maps to a work
key; see "Git rules").

## Layout (Go 1.27, module `claude-companion`)

```
cmd/claude-companion/main.go     args, CLAUDE_CONFIG_DIR, Resolve, Tail(100ms), run program
internal/transcript              Resolve(projectsDir, arg) · Tail(ctx, path, poll) <-chan Line
internal/event                   Pairer.Feed(line) []Event · types Prompt/Command/FileChange/Rejected/Hunk
internal/render                  Event(e, width, loc, shade) []string · Greeting(...) · highlight.go · styles.go
internal/ui                      Model (Bubble Tea v2) · batching · greeting · clear-on-prompt · shades
internal/event/testdata/*.jsonl  14 two-line fixtures cut from real transcripts, payloads neutralised
docs/superpowers/specs, plans    original design and plan (the layout section there predates the
                                 current row anatomy; this file is authoritative)
docs/architecture.md             prose architecture, kept current
```

Dependencies, exact: `charm.land/bubbletea/v2 v2.0.9`, `charm.land/lipgloss/v2
v2.0.6`, `charm.land/bubbles/v2 v2.2.1` (viewport), `github.com/alecthomas/chroma/v2
v2.27.0` (lexers only), `github.com/charmbracelet/x/ansi v0.11.8`. Note the
`charm.land/...` import paths for the v2 charm modules.

## Commands

```
go build ./... && go vet ./... && gofmt -l . && go test ./...      # the gate; all must be clean
go install ./cmd/claude-companion                                   # puts the binary in ~/go/bin
go test ./internal/ui/ -run '^$' -bench . -benchtime 3x             # ingestion benchmarks
CLAUDE_COMPANION_E2E_FILE=<transcript> CLAUDE_COMPANION_E2E_EXPECT=<cmd>,<files>,<rejected>,<prompts> \
  go test ./internal/ui/ -run TestReplayRealSession -v               # opt-in replay of a real session
```

Module downloads need `GOFLAGS= GOPROXY=https://proxy.golang.org,direct
GOSUMDB=sum.golang.org` on this machine: the owner's shell exports
`GOPROXY=direct GOSUMDB=off`.

## Data source facts (from a census of 56 sessions, 60,015 entries)

- One session = `~/.claude/projects/<slug>/<session-id>.jsonl`, append-only,
  one JSON object per line, always newline-terminated. Basename == `sessionId`
  field and is unique across slugs. `Resolve` globs `<projectsDir>/*/<arg>*.jsonl`
  and requires exactly one basename match.
- Subagents write to `<slug>/<session-id>/subagents/*.jsonl`; every entry in a
  main file has `isSidechain:false`. Main-session-only scope is free.
- A session moved with `/cd` writes a `relocated` entry and its file moves to
  the new slug directory. `Resolve` handles this only at startup (it searches
  all slugs); a relocation mid-run is out of scope.
- Tool call = `assistant` entry, `message.content[]` block `type:"tool_use"`
  `{id,name,input}`. Outcome = later `user` entry with `tool_result` block
  `{tool_use_id,is_error,content}` plus top-level `toolUseResult`.
- `toolUseResult` shapes: Bash `{stdout,stderr,interrupted,backgroundTaskId?}`
  or, on error, a string; Edit `{filePath,structuredPatch[],...}`; Write
  `{type:"create"|"update",filePath,content,structuredPatch[]}` where `create`
  has an EMPTY `structuredPatch` and the file body is in `content`; a refused
  tool has `is_error:true` and `toolUseResult == "User rejected tool use"`
  (exact string); a failed command has `is_error:true` and content starting
  `Exit code N\n`.
- `structuredPatch` hunk: `{oldStart,oldLines,newStart,newLines,lines[]}`,
  each line prefixed by one of ` `, `-`, `+`.
- Prompt rule (`event.promptText`): `user` entry, not `isMeta`, not
  `isCompactSummary`, no `tool_result` block; content is a string, or blocks
  that are only `text` and `image` (prompts with screenshots are
  `text,image,image`); trimmed text must not start with `<`. The `<` rule
  excludes the system-injected user entries seen in the corpus:
  `<task-notification`, `<local-command-caveat`, `<command-name`,
  `<command-message`, `<local-command-stdout`, `<bash-input`,
  `<bash-stdout`, `<system-reminder`.
- The transcript format is internal to Claude Code and has changed between
  versions in this corpus. When something looks off, re-census before
  changing a rule: `python3` over `~/.claude/projects/*/*.jsonl` counting
  entry types, block types and `toolUseResult` keys per tool name.

## Rendering facts and traps (each one cost a debugging round)

- Row anatomy: header `HH:MM` (5) + space + two-cell glyph (cols 6-7) + space
  + title from col 9. Body rows: 6 spaces + bar `▎` (col 6, under the glyph)
  + space + text from col 8. `indentWidth = 8`. Continuation rows of a
  wrapped command or prompt: 8 spaces, no bar. Every event ends with one
  blank line.
- Glyphs: `⚒️` (U+2692 + U+FE0F), `📁`, `🆕`, `❗`, `⚠️` (U+26A0 + U+FE0F). The
  variation selector is required: `ansi.StringWidth("⚒") == 1` but Ghostty
  draws it as two cells, which swallowed the following space; with VS16 the
  measure is 2 and matches. Do not add glyphs without checking
  `ansi.StringWidth` against the terminal.
- Colours: palette indices only (`lipgloss.Yellow`, `BrightYellow`, ...),
  never hex, so the terminal theme decides. Command bar yellow/bright yellow,
  file bar magenta/bright magenta; the `shade` argument (0/1) picks the
  variant and `ui.Model` alternates it between consecutive events of the
  same Go type (`sameType` compares `%T`). Prompts have no bar.
- Clock style: `Bold(true).Foreground(BrightBlack)` → `ESC[1;90m`.
- Width discipline: the viewport (`bubbles/v2/viewport`, `SoftWrap=false`)
  enables horizontal scrolling if ANY line measures wider than the width by
  `ansi.StringWidth`. Two real causes found in this session's transcript: a
  tab (`StringWidth` counts it 0, lipgloss expands it to 4 spaces at render
  time) and a bare `\r`. `render.clean` expands tabs to `tabWidth = 4`
  (lipgloss's default), keeps only what follows the last `\r`, and cuts at
  the first `\n`. Keep the "overflow hunt" habit: render every event of a
  real transcript and assert `ansi.StringWidth(line) <= width` (recipe in
  the session that built this; a test file for it lived at
  `internal/render/overflow_hunt_test.go` and was removed; recreate on demand).
- Truncating styled strings: use `ansi.Truncate(s, width, "…")`; rune
  slicing cuts escape sequences. Plain strings go through `fit`.
- Wrapping: `ansi.Wrap(text, width, "")` is word-aware and breaks over-long
  words; it leaves a trailing space at a break, trimmed by `wrapPlain`.
  Wrap the PLAIN text, then highlight each row separately: `ansi.Hardwrap`/
  `Wrap` on styled text does not re-open colour on the next row.
- lipgloss resets with `ESC[m`, not `ESC[0m`. Underline renders per run as
  `ESC[4;31;4m`. Bold/Faint wrappers around a string that contains inner
  resets lose the attribute after the first reset, so never wrap
  highlighted text in another style.
- Syntax colouring (`highlight.go`): chroma lexers + a fixed token map
  (keyword magenta, string green, number cyan, comment bright-black faint,
  Name.Function/Builtin/Tag/Class/Attribute blue). chroma's own `terminal16`
  formatter is NOT used: with common styles it emits `ESC[30m` (black) for
  punctuation, invisible on a dark theme. Use `InSubCategory` for
  `LiteralString`/`LiteralNumber` (`InCategory` compares the top-level
  `Literal` category, which made numbers green). A whole hunk is tokenised at
  once (multi-line constructs survive) and chroma re-opens colour per line,
  verified. `lexers.Match(basename)` returns nil for unknown extensions →
  plain text; `Makefile`, `*.yaml`, `*.json`, `*.md`, `*.go` match.
- Diff lines: sign `-`/`+` rendered bold red / bold green (`ESC[1;31m`,
  `ESC[1;32m`), code highlighted from the file's lexer; context lines
  highlighted too.
- Git-write warning: `gitWriteRE` =
  `(^|[^[:alnum:]_])git\s+(add|commit|push)($|[^[:alnum:]_])` on the whole
  command text → glyph `⚠️`, every row `styleWarn` (underline + red), no
  syntax colouring. Literal by owner's request: `echo 'git push'` is flagged.

## UI model facts

- `Init` batches `waitLines` (one blocking read, then drain up to
  `maxBatch = 256` queued lines into one `linesMsg`) and the greeting tick.
  Per-line refresh was O(n) inside the viewport (`SetContentLines` measures
  every line); batching took 2,000 lines from 11.9 s to 0.10 s
  (`BenchmarkIngest` vs `BenchmarkIngestBatched`).
- Greeting: `hello, <user>` typed at `tickEvery = 40ms` per rune, then the
  block (session, cwd, started) held `holdAfter = 700ms`, then the log. The
  log is ingested during the reveal but hidden (`held == false`); the
  greeting shows during reveal/hold even if a prompt already set `cleared`
  (`greetingLines(force)`).
- A `Prompt` event clears `events`, `rendered`, `shades`, `flat`, `index`,
  sets `cleared` (greeting gone for good) and starts a new block. Design
  intent: the screen shows the work since the user's last message.
- A running `Command` (tool_use seen, result pending) is emitted immediately
  and replaced in place by id when the result lands; its shade is kept.
- `follow` is true while the viewport is at the bottom; any upward scroll
  pauses following; `end` resumes; `home` goes top and pauses.
- `View()`: `AltScreen`, `MouseMode = MouseModeNone` (mouse reporting off so
  the terminal keeps text selection), window title `claude-companion <id>`.
  UNVERIFIED: whether Ghostty converts wheel motion into arrow keys on the
  alternate screen when reporting is off (the "alternate scroll" behaviour of
  xterm). Ghostty's config docs do not state it; if the wheel stops scrolling
  the log, that is the cause, and the trade-off against text selection is the
  owner's call.
- Footer: `<id> · <n> events · ⇣ following|⇡ paused [· k skipped] [error]`.

## Tests (60 functions, all offline)

- Fixtures are real two-line cuts; employer content was replaced by neutral
  text with identical JSON shape and identical asserted values. Never commit
  raw transcript lines without the same pass: scan for tokens, emails, the
  Air's serial, and work paths.
- Render tests compare `plain(...)` (ANSI stripped with the `sgr` regexp)
  goldens and check specific SGR substrings for colour. When a golden fails,
  print `got` with `%q` before touching code: two "failures" in this
  project's history were wrong expectations (a failed command's glyph is
  `❗`; `ansi.Wrap` trailing space).
- Prove tests bite: the pairing rules were mutation-checked (rejection
  marker, `<` prompt rule, exit-code parse); repeat that after touching
  `pair.go`.
- `TestReplayFixturesThroughTail` runs Tail → Pairer → render → Model over
  the concatenated fixtures and appends lines live; prompts are ordered
  first because a prompt clears the log.

## Owner's rules that bind you here

- Never commit, push, branch, tag, or change GitHub state without explicit
  approval in that moment; "finish the task" is not approval. Show the exact
  command and wait.
- Commit messages: pass the body on git's stdin (`git commit -F - <<'EOF'`),
  never through a `cat` heredoc in `$(...)`: `cat` is aliased to a
  colourising pager on this machine and corrupts messages.
- Commit trailers the owner uses: `Co-Authored-By: Claude <model> <noreply@anthropic.com>`
  and `Claude-Session: <url>`. Local git identity is set per-repo
  (`AmayaHena <AmayaHena@protonmail.com>`, SSH-signed); do not touch global
  git config.
- Edit files, do not rewrite them; when a small change solves the issue,
  stop there. Verify each line you change; if you cannot say where a value
  comes from, you do not have it.
- No guesses presented as facts. Mark unverified things as unverified.
- Palette only for colours. No emoji beyond the five glyphs.

## Out of scope so far (do not build silently)

Subagents and workflows, click-to-open files, sessions relocated mid-run,
`MultiEdit`/`NotebookEdit` (absent from the corpus), deletions (no tool
deletes; an `rm` inside a command is not classified), highlighting of
command OUTPUT (only the command line and file contents are coloured).

## Known rough edges

- `ui.sameType` uses `fmt.Sprintf("%T")`; fine at this scale.
- Highlighting a hunk tokenises `codes` joined by `\n`; a lexer that swallows
  a newline would misalign rows — guarded by padding/truncating to the input
  length in `highlightLines`, not by a test.
- The greeting shows `cwd` from the first transcript entry; a `/cd` later in
  the session is not reflected.
