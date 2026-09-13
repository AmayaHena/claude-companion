# claude-companion — notes for the next Claude Code session

Audience: an AI agent working in this repository. Every statement below was
verified against the tree or by an executed experiment on 2026-09-13; when
something was not verified it says so. Re-verify before relying on a number.

## What this is

A read-only Bubble Tea terminal viewer. `claude-companion [session-id | unique
prefix]` tails one Claude Code session transcript and shows what Claude did:
commands with output, file edits and creations as diffs, refused tool uses,
and the user's prompts. Assistant prose and thinking are never shown. No
network, no writes, deterministic (no randomness anywhere, no timers).

Owner: Amaya (`amaya` on this Mac). Repo: `github.com/AmayaHena/claude-companion`
(module path renamed to match on 2026-09-13 so `go install …@version` works),
remote alias `github-personal` (the plain `github.com` SSH host maps to a work
key; see "Git rules").

## Layout (Go 1.27, module `github.com/AmayaHena/claude-companion`)

```
cmd/claude-companion/main.go     args, CLAUDE_CONFIG_DIR, Resolve or picker (no arg), Tail(100ms), run program
internal/transcript              Resolve(projectsDir, arg) · Recent(projectsDir, n) []Session · Tail(ctx, path, poll) <-chan Line
internal/event                   Pairer.Feed(line) []Event · types Prompt/Command/FileChange/Rejected/Agent/Skill/Hunk
internal/render                  Event(e, width, loc) []string · highlight.go · styles.go
internal/ui                      Model (Bubble Tea v2) · batching · clear-on-prompt · footer tally · copy keys
internal/ui/picker.go            Picker: the no-argument session list (own tea.Program, run before the viewer)
internal/event/testdata/*.jsonl  16 fixtures cut from real transcripts (14 two-line, agent/skill one-line), payloads neutralised
docs/superpowers/specs, plans    original design and plan (the layout section there predates the
                                 current row anatomy; this file is authoritative)
docs/architecture.md             prose architecture, kept current
demo/                            synthetic one-of-each transcript + play.sh replaying it line by line
                                 (sh demo/play.sh first, it creates the replayed file; then
                                 CLAUDE_CONFIG_DIR="$PWD/demo" claude-companion demo1111); replayed file gitignored
```

Dependencies, exact: `charm.land/bubbletea/v2 v2.0.9`, `charm.land/lipgloss/v2
v2.0.6`, `charm.land/bubbles/v2 v2.2.1` (viewport), `github.com/alecthomas/chroma/v2
v2.27.0` (lexers only), `github.com/charmbracelet/x/ansi v0.11.8`. Note the
`charm.land/...` import paths for the v2 charm modules.

## Commands

```
go build ./... && go vet ./... && gofmt -l . && go test ./...      # the gate; all must be clean
go install -trimpath ./cmd/claude-companion                         # puts the binary in ~/go/bin; -trimpath keeps home paths out of it
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
- Current files (checked on the 12 most recent, 2026-09-13) open with 3 to 8
  header entries that carry neither `timestamp` nor `cwd`: types seen are
  `last-prompt`, `mode`, `permission-mode`, `bridge-session`, `atis-latch`,
  `ai-title`, `agent-name`, `file-history-snapshot`, `attachment`. The first
  `cwd` is on line 3 to 8. Anything reading "the first entry" must instead
  keep the first non-empty value (`ui.Model.ingest`, `transcript.head`).
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
  + title from col 9. Body rows: 6 spaces + bar `▕▏` (U+2595 U+258F, cols
  6-7: the ink of the two eighth blocks meets on their shared edge, so the
  line sits in the middle of the glyph above; owner's request) + text from
  col 8. `indentWidth = 8`. Continuation rows of a
  wrapped command or prompt: 8 spaces, no bar. Every event ends with one
  blank line. A command header may carry one mark before its glyph, with a
  space: `⚠️ ⚒️` (git/GitHub write) or `🛜 ⚒️` (network command); the title
  then starts at col 12 and the body bar stays at col 6. A failed command
  keeps its mark: `⚠️ ❗`, `🛜 ❗`. Agents (`🤖 <description>  <type>`) and
  skills (`ℹ️ <skill> [args]`) are one header row, no body, no bar. Prompts
  render every line of the text (a blank line stays a blank row), each line
  wrapped.
- Glyphs: `⚒️` (U+2692 + U+FE0F), `📁`, `🆕`, `❗`, `⚠️` (U+26A0 + U+FE0F),
  `🛜` (U+1F6DC), `🤖` (U+1F916), `ℹ️` (U+2139 + U+FE0F; bare U+2139 measures 1). The
  variation selector is required: `ansi.StringWidth("⚒") == 1` but Ghostty
  draws it as two cells, which swallowed the following space; with VS16 the
  measure is 2 and matches. Do not add glyphs without checking
  `ansi.StringWidth` against the terminal.
- Colours: palette indices only (`lipgloss.Yellow`, `Red`, ...), never hex,
  so the terminal theme decides. One bar hue per action type, matching the
  header glyph: `⚒️` yellow, `❗` red, `📁` magenta, `🆕` cyan (blue and
  green belong to agent and skill titles). No dark/light alternation any
  more (removed 2026-09-13 with the `shade` argument, `Model.shades` and
  `sameType`). Prompts have no bar.
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
- Untrusted text: everything from a transcript (commands, output, file
  lines, paths, prompts, agent descriptions and types, skill names, picker
  rows, the footer's copy target) passes through `render.Sanitize`: ESC →
  `␛` (U+241B, one cell, keeps the payload readable), other C0, DEL and C1
  (U+0080–U+009F) dropped, invalid UTF-8 → U+FFFD. `clean` calls it, so
  the render path is covered by construction; the picker and the footer
  call it explicitly. Found on 2026-09-13 by feeding `ESC ] 0 ; … BEL` through
  the renderer: it reached the terminal raw (title, clear-screen and OSC 52
  injection from a transcript were all possible). Test:
  `TestTerminalControlsNeverReachTheScreen` covers every event type; the
  Agent `Type` suffix was the last leak (it bypassed `clean`). Any new
  string that comes from a transcript must be added to that test. Also
  sanitised: the window title (session id is a filename), the footer error
  (embeds a path), the picker id. `SanitizeText` keeps `\n`/`\t` and is what
  the clipboard gets; `Marks` decides on `SanitizeText(cmd)` so a NUL or
  zero-width character inside `git push` cannot hide the warning. Sanitize
  also drops bidi controls, zero-width space/non-joiner/marks, soft hyphen
  and BOM (U+200D joiner kept).
- Caps: `maxBodyRows = 2000` rows per body (command output, created file,
  all hunks of one change) then one `… N more lines` trailer;
  `transcript.maxLine = 64 MiB` for an unterminated line (dropped with an
  error Line, reading resumes at the next newline). Hunk signs are read as
  a rune, never a byte.
- `main` unsets `TEA_TRACE` and `TEA_DEBUG` before starting Bubble Tea: both
  are read with `os.Getenv` inside the library (not through
  `WithEnvironment`) and would create files. Verified in a pty: no file.
- Truncating styled strings: use `ansi.Truncate(s, width, "…")`; rune
  slicing cuts escape sequences. Plain strings go through `fit`.
- Wrapping: `ansi.Wrap(text, width, "")` is word-aware and breaks over-long
  words; it leaves a trailing space at a break, trimmed by `wrapPlain`. It
  can also return a row ONE cell over the limit when it breaks at a hyphen
  (found by the overflow hunt on `…; echo "--` at width 40 and 80);
  `wrapPlain` hard-wraps any such row (`TestWrappedRowsNeverExceedTheWidth`).
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
- Git-write warning: `gitWriteRE` (render.go) on the whole command text →
  mark `⚠️` before the glyph, every row `styleWarn` (underline + red), no
  syntax colouring. Matches `git` + add, commit, merge, rebase, cherry-pick,
  revert, reset, clean, filter-branch, tag, push, `stash drop`,
  `branch -d/-D/-m`, `checkout -b/--orphan`, `switch -c`, `worktree add`;
  and `gh` + `pr create|merge|ready`, `release create`, `repo sync`,
  `api … -X|--method POST|PUT|PATCH|DELETE`. Literal by owner's request:
  `echo 'git push'` is flagged. The full list is the table in
  `TestGitWriteCommandsAreFlagged`.
- Network mark: `netRE` (render.go) → mark `🛜` before the glyph, no other
  styling. Matches curl, wget, ssh, scp, sftp, rsync, nc, ncat, telnet,
  ping, dig, nslookup, traceroute, `git fetch|pull|clone|ls-remote|push`,
  any `gh`, `npm install|i|ci|add|publish|update`, npx, yarn, pnpm,
  `pip[3] install`, `go get`, `go mod download`, `brew
  install|upgrade|update|fetch`, `docker pull|push|login`, aws, gcloud, az,
  kubectl, helm, `apt[-get] install|update|upgrade`. Literal like the git
  rule. `render.Marks(cmd)` returns `(warn, net)` with the warning winning:
  `git push` is `⚠️` only and counts once. `render.Glyph(e)` is the type
  glyph (`❗` for a failed or rejected command) used by both the header and
  the footer, so the two cannot drift.

## UI model facts

- `Init` is `waitLines` (one blocking read, then drain up to
  `maxBatch = 256` queued lines into one `linesMsg`).
  Per-line refresh was O(n) inside the viewport (`SetContentLines` measures
  every line); batching took 2,000 lines from 11.9 s to 0.10 s
  (`BenchmarkIngest` vs `BenchmarkIngestBatched`).
- No greeting: the log shows from the first line (the typed `hello, <user>`
  screen, its hold timer, and the cwd/started block were removed on
  2026-09-13 at the owner's request; the window title carries the session
  id, the picker shows the project folder).
- A `Prompt` event clears `events`, `rendered`, `flat`, `index`, `tally` and
  starts a new block. Design intent: the screen shows the work since the
  user's last message.
- A running `Command` (tool_use seen, result pending) is emitted immediately
  and replaced in place by id when the result lands.
- `follow` is true while the viewport is at the bottom; any upward scroll
  pauses following; `end` resumes; `home` goes top and pauses.
- Copy keys: `c` sends the targeted command's full text to the clipboard via
  `tea.SetClipboard` (OSC 52; Bubble Tea writes `ansi.SetSystemClipboard`).
  The target is the latest command of the current block; `tab` moves it one
  command older, `shift+tab` back; the footer shows `copy → <first line…>`
  while the target is not the latest, `copied` after a copy, `nothing to
  copy` when the block has no command. Any new event resets the target.
  Ghostty: the owner's config sets `clipboard-write = ask` (default `allow`),
  so each copy raises a Ghostty confirmation until that line changes.
- Picker (`claude-companion` with no argument): `transcript.Recent` lists
  the `*/*.jsonl` files directly under the project folders (subagent files
  are one level deeper and never match), newest mtime first, `recentLimit =
  10`; for each it reads at most `headLines = 200` lines for the first
  entry's cwd and the first prompt's first line. `ui.Picker` is its own
  program (`↑↓ j k` move, `enter` opens, `q` quits); `main.pick` then starts
  the viewer on the chosen path. A missing projects dir is an error, an
  empty one prints `no sessions under …`.
- `View()`: `AltScreen`, `MouseMode = MouseModeNone` (mouse reporting off so
  the terminal keeps text selection), window title `claude-companion <id>`.
  UNVERIFIED: whether Ghostty converts wheel motion into arrow keys on the
  alternate screen when reporting is off (the "alternate scroll" behaviour of
  xterm). Ghostty's config docs do not state it; if the wheel stops scrolling
  the log, that is the cause, and the trade-off against text selection is the
  owner's call.
- Footer: a tally of the work since the last prompt (kept in `Model.tally`
  by event id, so a running command replaced by its result counts once, at
  its final state; reset together with the log when a prompt arrives):
  `⚒️ 62%  ·  📁 25%  ·  🆕 6%  ·  ❗ 6%` (share of all actions, rounded half
  up, zero entries omitted, `0 actions` when empty), then plain counts
  `⚠️ n  ·  🛜 n  ·  🤖 n  ·  ℹ️ n`, then `k skipped` and the last error.
  Numbers are `White` (`ESC[37m`), separators and labels `BrightBlack`; the
  `⚠️` count is red, `🤖` blue, `ℹ️` green, matching the event colours
  (`styleWarn`, `styleAgent` bold blue, `styleSkill` bold green). Agents and
  skills are counted but are not in the percentage base. No session id, no
  follow state, by owner's request.

## Tests (77 functions, all offline)

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
- Palette only for colours. No emoji beyond the eight glyphs.

## Trust audit (2026-09-13)

A five-lens workflow (security, determinism, read-only, no network, privacy;
15 agents, every finding re-checked by a skeptic told to refute it) ran
against the tree before v1. Claims that held as stated: determinism (one
wall-clock read, the picker's "ago"; footer order fixed by construction),
read-only (four read-only fs calls in product code, `lsof` shows the
transcript as `3r`), no network (171-package graph, no `net`, no TLS; binary
symbols confirm). Fixed after the audit: raw escapes from transcripts reaching
the terminal, title/footer/picker id unsanitised, marks decided on raw text,
whole raw command copied while one line shown, unbounded tailer line, uncapped
bodies, byte-split hunk sign, bidi/zero-width passthrough, `TEA_TRACE`/`TEA_DEBUG`
file writes, home paths in the binary (`-trimpath`). Known and accepted: a
`tmux info` subprocess under tmux (colorprofile); the tailer follows the
inode, not the path (a transcript replaced in place is not re-opened);
fixtures carry `/Users/amaya` paths and the public username; the picker reads
at most 200 lines of the 10 most recent sessions.

## Website (`site/`, published by `.github/workflows/pages.yml`)

Static, no build: `index.html`, `style.css`, `main.js`, `fonts/` (Archivo and
Geist Mono variable woff2, latin subset, OFL texts alongside). The terminal
panel in the hero and the five samples are REAL renders of `demo/` through
`render.Event` converted to spans (class per SGR: `b`, `dim`, `u`,
`c-<colour>`); regenerate them with a throwaway `cmd/zz-htmldump` (see the
2026-09-13 session) rather than hand-editing rows. Design decisions:
Marathon-style graphic realism after the owner's reference screenshots
(second iteration, 2026-09-13; the first, a generic dark landing with a
cyan accent, was rejected): HUD chrome (fixed crop marks at the viewport
corners, a rotated edge label, captioned progress bars with real numbers,
mono caps labels in acid blocks), pixel-block marks and checkerboards as
inline SVG rects, a barcode strip, a hazard strip, caps display type in
Archivo at 116-118% width with the key word on a flat acid block plus a
cursor block, one acid accent `#c8ff2e` replacing the reference's yellow,
one full-bleed electric-blue field (`#1f2ee3`, the audit section), one
light document panel (`#e6e9e4` with mint and violet tags, keys and demo),
scanlines and grain fixed over the page, a blurred rust/teal field with
soft-light noise behind the hero. Browser surfaces are themed too
(`::selection`, thin acid scrollbars on the code panels). Motion is one
authored moment: the rows arriving plus the progress fill; the earlier
reveal-on-scroll on every section was removed on 2026-09-14 (craft-floor:
not one identical entrance per section). Running text is Geist Mono, display
type Archivo; labels are 12px minimum. Radius 0 everywhere, no em dashes, no
icon library, no emoji outside the real terminal renders. Motion: rows arrive with a 55 ms
stagger on load (explanatory), reveal-once on scroll armed by JS with a 2.5 s
release timer so nothing can stay hidden, all gated by
`prefers-reduced-motion`. Checks used: an iframe probe page in headless
Chrome for overflow/media-query/font facts (a plain `--screenshot` at 400 px
reports a false overflow on macOS), `grep -P '[\x{2014}\x{2013}]'` for
dashes, and the Chrome DevTools MCP server (`.mcp.json`, project scope)
for real-Chrome captures: when the MCP is not loaded in the session, a
30-line stdio client (`scratchpad/mcp/cdm.py` in the 2026-09-14 session)
drives `npx chrome-devtools-mcp --headless --isolated` directly
(`new_page` → parse the numeric page id → `resize_page`, `take_screenshot`;
large screenshots come back as a file path, not inline data). impeccable's
detector (`.claude/skills/impeccable/scripts/impeccable detect --json
site/index.html`, run from the repo root) reports 14 findings that are all
either false positives on decorative absolute layers or the brief's own
choices (uppercase headings, Geist, the blurred field, the hazard strip). GitHub Pages: the API answered "Your current plan does not support
GitHub Pages for this repository" while the repo is private; the workflow
will fail until the repo is public and Pages is enabled with source
"GitHub Actions". Repo-level skills installed for site work (gitignored under
`.claude/skills/`): taste-skill (design-taste-frontend), ui-ux-pro-max,
animate.

## Out of scope so far (do not build silently)

The work done inside subagents (only the launch line `🤖` is shown), the
`Workflow` tool (38 uses in the corpus, no event), click-to-open files,
sessions relocated mid-run,
`MultiEdit`/`NotebookEdit` (absent from the corpus), deletions (no tool
deletes; an `rm` inside a command is not classified), highlighting of
command OUTPUT (only the command line and file contents are coloured).

## Known rough edges

- Highlighting a hunk tokenises `codes` joined by `\n`; a lexer that swallows
  a newline would misalign rows — guarded by padding/truncating to the input
  length in `highlightLines`, not by a test.
