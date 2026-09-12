# claude-companion — design

Date: 2026-09-10. Status: approved in chat, implementation follows.

## Purpose

A read-only terminal viewer that attaches to one running (or finished) Claude Code
session and shows, in real time, what Claude *did*: the commands it ran with their
output, and the files it changed with their diffs. Assistant prose and thinking are
never shown. It is a display tool, not an agent: deterministic, no network, no
writes outside its own process.

Usage: `claude-companion <session-id>` (full id, or a prefix that matches exactly one session).
Keys: `q`, `esc`, `ctrl+c` quit; `↑ ↓`, `j k`, `pgup pgdn`, `home end`, mouse wheel scroll.
Nothing else.

## Data source (verified 2026-09-10 on this machine, 56 sessions, 60,015 entries)

One session is one append-only file `~/.claude/projects/<slug>/<session-id>.jsonl`,
one JSON object per line, terminated by `\n`. `<session-id>` equals the `sessionId`
field (56/56) and basenames are unique across all `<slug>` folders. Subagents write
to `<slug>/<session-id>/subagents/*.jsonl`, never to the main file: every entry of a
main file has `isSidechain: false`. Main-session-only scope therefore needs no filter.

Entries the tool reads (all other `type` values are ignored):

| entry | condition | becomes |
|---|---|---|
| `assistant` with `message.content[]` block `type=="tool_use"` | `name` ∈ {Bash, Edit, Write} | a *pending* call keyed by block `id` |
| `user` with block `type=="tool_result"` whose `tool_use_id` is a pending call | | the call's outcome; the pair becomes one event |
| `user` prompt | `isMeta` ≠ true, `isCompactSummary` ≠ true, content is a string or a list of `text` blocks without `tool_result`, trimmed text does not start with `<` | `Prompt` |

The `<` rule excludes system-injected user entries seen in the corpus:
`<task-notification`, `<local-command-caveat`, `<command-name`, `<command-message`,
`<local-command-stdout`, `<bash-input`, `<bash-stdout`, `<system-reminder`.

Outcome decoding, from the `user` entry carrying the `tool_result` block `b` and the
top-level `toolUseResult` field `r`:

- Rejected: `b.is_error == true` and `r` is the string `"User rejected tool use"`.
- Bash: `r` is an object → `stdout`, `stderr`, `backgroundTaskId` (present ⇒ background).
  Exit status: `b.is_error != true` ⇒ 0; `b.is_error == true` and `b.content` text
  starts with `Exit code N` ⇒ N; otherwise failed with unknown code. When `r` is a
  string (error path) the output shown is `b.content` text.
- Edit: `r.filePath` (fallback `input.file_path`), `r.structuredPatch` hunks, kind = update.
- Write: `r.type` is `"create"` (344 seen) or `"update"` (67 seen); `r.filePath`. For `update`, `r.structuredPatch` holds the hunks. For `create`, `r.structuredPatch` is empty (verified on the fixture) and the file content is `r.content` (fallback `input.content`), rendered as added lines.
- `b.content` is a string or a list of `{type:"text", text}` blocks; both are joined to text.
- `structuredPatch` hunk: `{oldStart, oldLines, newStart, newLines, lines[]}` where each
  line is prefixed by one of ` `, `-`, `+`.

Timestamps: entry `timestamp` (RFC 3339), shown as local `HH:MM:SS`.

## Architecture

```
cmd/claude-companion/main.go     argument parsing, session file resolution, program start
internal/transcript              Tail (file → complete lines) and Parse (line → raw entry)
internal/event                   Pair (raw entries → typed events): Prompt, Command, FileChange, Rejected
internal/ui                      Bubble Tea model: greeting, event log, viewport, keys
internal/ui/render               pure functions: event → styled lines for a given width
testdata/                        fixtures cut from real transcripts, one per event kind
```

Libraries (versions verified against GitHub releases and `go doc`, 2026-09-10):
`charm.land/bubbletea/v2 v2.0.9`, `charm.land/lipgloss/v2 v2.0.6`, `charm.land/bubbles/v2 v2.2.1`
(viewport). Standard library for everything else. No diff library: hunks are read, not computed.

### transcript

`Resolve(projectsDir, arg) (path, error)`, with `projectsDir` = `$CLAUDE_CONFIG_DIR/projects` when the variable is set (the override Claude Code documents) else `~/.claude/projects`: glob `<projectsDir>/*/<arg>*.jsonl` on basenames;
exactly one match → its path; zero → error listing nothing; many → error listing the candidates.
`Tail(ctx, path) <-chan Line`: reads existing content, emits each complete line, then polls
`os.Stat` size every 250 ms and emits only newly completed lines (a trailing partial line is
held until its `\n` arrives). Truncation (size shrinks) restarts from offset 0.

### event

`Pairer` keeps pending `tool_use` calls by id. `Feed(entry) []Event` returns zero or more
events in file order. A Command whose result has not arrived is emitted immediately as
`Command{Running: true}` and replaced (same `ID`) when the result lands, so a long command
shows as running rather than absent.

Types:
```go
type Prompt     struct{ At time.Time; Text string }
type Command    struct{ ID, Cmd, Stdout, Stderr string; At time.Time; Running, Background bool; Exit int; ExitKnown bool }
type FileChange struct{ ID, Path string; At time.Time; Kind Kind /* Create | Update */; Hunks []Hunk }
type Rejected   struct{ ID, Tool, Subject string; At time.Time }   // Subject: command or path
```

### ui

State: greeting phase (typed reveal on a 40 ms tick, fixed text, no randomness), then the
log. Lines are ingested during the reveal but the log is not shown until the greeting is
complete; then it appears and the view jumps to the tail (without this the tail of a long
session hides the greeting within milliseconds). The viewport follows the tail while `AtBottom()`; any upward scroll stops following;
scrolling back to the bottom resumes it. Window resize re-renders all events at the new width.

Colours are terminal palette indices only (`lipgloss.Color("1")` … `"15"`), never hex, so the
active terminal theme decides the look. Glyphs, one per category: `⚒` command, `📁` edit,
`🆕` new file, `❗` rejected or failed (non-zero exit). No other emoji.

Layout per event (width-aware):
```
HH:MM:SS ⚒  <command, first line, truncated>          exit 0 · 0.4s (duration omitted: not in data)
          <stdout lines, dim>                          stderr lines in red
HH:MM:SS 📁 path/relative/to/cwd
          @@ -39,7 +39,7 @@                            faint
           context                                     default
          -removed                                     red
          +added                                       green
HH:MM:SS ❗ Edit path                                  "rejected"
── HH:MM:SS  first line of the prompt ──────────────   dim separator
```
Duration is not present in the transcript for tool calls, so it is not shown.

Greeting (first screen, then the log appends below it):
```
hello, amaya                       typed out, bold, accent colour 4 (blue)
session  <slug or id>
cwd      <cwd of first entry>
started  <timestamp of first entry, local>
```

## Error handling

- Missing or ambiguous session id: message on stderr, exit 1.
- Unparseable line: skipped, counted; the count is shown in the footer as `n skipped`.
- Missing fields: the event is still produced with empty fields; never a panic.
- File disappears mid-run: footer shows `file gone`; the tool keeps the log and waits for `q`.

## Testing

- `internal/transcript`: Tail against a temp file appended by the test, including a partial
  last line completed later, and a truncation. Resolve with 0, 1, n matches.
- `internal/event`: table tests over `testdata/*.jsonl` fixtures cut from real sessions:
  bash ok, bash exit≠0, bash rejected, bash background, edit, write create, write update,
  edit rejected, prompt (plain, text block, each excluded `<` tag, isMeta).
- `internal/ui/render`: golden tests per event kind at widths 40 and 120, ANSI stripped.
- `internal/ui`: Update-driven tests for follow/stop-follow and quit keys.
- Every test runs offline; `go test ./...` must pass with `GOFLAGS=-mod=mod` and no network
  after the first module download.

## Out of scope for this POC

Subagents and workflows, clicking a path to open a file, sessions relocated mid-run,
MultiEdit and NotebookEdit (absent from the corpus), deletions (no tool deletes; an `rm`
inside a command is not classified), any write outside the process, any network call.
