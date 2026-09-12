# claude-companion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A read-only Bubble Tea viewer, `claude-companion <session-id>`, that tails one Claude Code session transcript and shows commands with output and file changes with diffs, live, without assistant prose.

**Architecture:** Three pure layers under `internal/` (transcript tailing, event pairing, rendering) feed one Bubble Tea model in `internal/ui`. Every layer below the UI is tested offline against fixtures cut from real transcripts in `internal/event/testdata/`.

**Tech Stack:** Go 1.27.1; `charm.land/bubbletea/v2 v2.0.9`, `charm.land/lipgloss/v2 v2.0.6`, `charm.land/bubbles/v2 v2.2.1`; standard library only otherwise.

**Spec:** `docs/superpowers/specs/2026-09-10-claude-companion-design.md`

## Global Constraints

- Module path `claude-companion`; binary `cmd/claude-companion`.
- No network access at runtime. Module download happens once at build time only.
- No file writes anywhere at runtime; the transcript is opened read-only.
- Colours: terminal palette constants only (`lipgloss.Red`, `lipgloss.Green`, `lipgloss.Blue`, `lipgloss.BrightBlack` …), never hex.
- Glyphs: `⚒` command, `📁` edit, `🆕` new file, `❗` rejected or failed. Nothing else.
- Keys: `q`, `esc`, `ctrl+c` quit; `up`/`k`, `down`/`j`, `pgup`, `pgdown`, `home`, `end`, mouse wheel scroll.
- Git: the owner's rules require explicit approval for any commit; this plan has no commit steps. Leave the tree uncommitted.
- Verified key names for bubbletea v2 (`KeyPressMsg.String()`): `up`, `down`, `pgup`, `pgdown`, `home`, `end`, `esc`, `ctrl+c`, `q`, `j`, `k`.
- Verified viewport v2 semantics: `New(WithWidth, WithHeight)`, `SetContent`, `GotoBottom`, `AtBottom` (false until GotoBottom when content exceeds height), `ScrollUp/Down(n)`, `PageUp/PageDown`, `GotoTop`, `SetWidth/SetHeight`, `Update(msg) (Model, tea.Cmd)`, `View() string`; default keymap already binds up/k, down/j, pgup/b, pgdown/space/f.

---

### Task 1: Module and transcript tailing

**Files:**
- Create: `go.mod`, `internal/transcript/resolve.go`, `internal/transcript/tail.go`
- Test: `internal/transcript/resolve_test.go`, `internal/transcript/tail_test.go`

**Interfaces:**
- Produces: `func Resolve(projectsDir, arg string) (string, error)`; `type Line struct{ Text string; Err error }`; `func Tail(ctx context.Context, path string, poll time.Duration) <-chan Line`.

- [ ] Step 1: `go mod init claude-companion` and `go get` the three charm modules at the pinned versions.
- [ ] Step 2: Failing tests for Resolve: 0 matches → error containing "no session"; 1 match by full id and by unique prefix → path; 2 matches → error listing both basenames.
- [ ] Step 3: Implement Resolve with `filepath.Glob(filepath.Join(projectsDir, "*", arg+"*.jsonl"))`, filtering basenames with `strings.HasPrefix`.
- [ ] Step 4: Failing tests for Tail: (a) existing 2 lines are emitted; (b) a partial third line without `\n` is not emitted until the `\n` is appended; (c) truncation to 0 bytes then a new line restarts from offset 0; (d) cancelling ctx closes the channel.
- [ ] Step 5: Implement Tail: open read-only, read to EOF splitting on `\n`, keep the remainder; loop: sleep `poll`, `Stat`; if size < offset → seek 0 and clear remainder; if size > offset → read the delta. Errors are sent as `Line{Err}` and the loop continues except on open failure.
- [ ] Step 6: `go test ./internal/transcript/` passes.

### Task 2: Event model and pairing

**Files:**
- Create: `internal/event/event.go`, `internal/event/pair.go`
- Test: `internal/event/pair_test.go` against `internal/event/testdata/*.jsonl`

**Interfaces:**
- Produces: types `Prompt`, `Command`, `FileChange`, `Rejected`, `Hunk`, `Kind`; `type Event interface{ EventID() string; When() time.Time }`; `type Pairer struct`; `func NewPairer() *Pairer`; `func (p *Pairer) Feed(line string) ([]Event, error)`; `func (p *Pairer) Skipped() int`.

Decoding rules are the spec's "Outcome decoding" table, implemented in `pair.go` exactly:
- `assistant` entry: for each `tool_use` block with name Bash/Edit/Write, store `{id, name, input, at}`; for Bash emit `Command{Running:true}` immediately.
- `user` entry: if a `tool_result` block references a stored id, decode by name and emit the final event; delete the pending entry. Otherwise apply the prompt rule (not `isMeta`, not `isCompactSummary`, string or text-only content, trimmed text not starting with `<`) and emit `Prompt`.
- Content text: string as is; list → concatenation of `text` fields of `text` blocks.
- Exit code: `is_error != true` → 0, known; else regex `^Exit code (\d+)` on the content text → N, known; else unknown.
- Bash output: `toolUseResult` object → `stdout`, `stderr`; string → content text goes to `Stdout`.
- Write create: hunks empty; `Content` field carries `toolUseResult.content` (fallback `input.content`).
- Errors: an unparseable line increments `Skipped()` and returns `(nil, err)`; missing fields yield empty strings, never a panic.

- [ ] Step 1: Failing table test: for each fixture, feed both lines and assert the concrete values printed by the fixture extraction (ids, exit codes 0 / 2 / unknown, `Background` true for `b1zhxped1`, edit path `/Users/amaya/example/repo/docs/investigation.md` with one hunk `oldStart 161`, rejected subjects, write kinds create/update, prompt inclusion/exclusion for the six prompt fixtures plus a synthetic `isCompactSummary` line).
- [ ] Step 2: Implement `event.go` types and `pair.go`.
- [ ] Step 3: `go test ./internal/event/` passes; add a test that feeding garbage returns an error and increments Skipped.

### Task 3: Rendering

**Files:**
- Create: `internal/render/render.go`, `internal/render/styles.go`
- Test: `internal/render/render_test.go` (golden strings with ANSI stripped via `charm.land/x/ansi`? no: use a local `strip` that removes `\x1b[...m`), widths 40 and 120.

**Interfaces:**
- Consumes: `event.*` types.
- Produces: `func Event(e event.Event, width int, loc *time.Location) []string` (one styled line per slice element, each ≤ width cells), `func Greeting(user, session, cwd string, started time.Time, revealed int) []string`.

Layout (spec "Layout per event"): `HH:MM:SS ⚒  cmd-first-line` then output lines indented 10 columns; stderr red; a failed command shows `❗` instead of `⚒` and `exit N` (or `exit ?`) on the header line, dim; `Running` shows `…` dim; `Background` shows `bg` dim. Edits: `HH:MM:SS 📁 path` then `@@ -a,b +c,d @@` faint, ` ` default, `-` red, `+` green. Creates: `🆕 path` then every content line as `+` green. Rejected: `HH:MM:SS ❗ Edit path` + `rejected` dim. Prompts: `── HH:MM:SS  text ────` in BrightBlack. Long lines are truncated to width with `…`.

- [ ] Step 1: Failing golden tests for one event of each kind at width 120 and a prompt at width 40.
- [ ] Step 2: Implement; `go test ./internal/render/` passes.

### Task 4: UI model

**Files:**
- Create: `internal/ui/model.go`
- Test: `internal/ui/model_test.go`

**Interfaces:**
- Consumes: `transcript.Line`, `event.Pairer`, `render.Event`, `render.Greeting`.
- Produces: `func New(cfg Config) Model`; `type Config struct{ User, SessionID, Path string; Lines <-chan transcript.Line; Loc *time.Location }`; messages `lineMsg`, `tickMsg`.

Behaviour: `Init` returns `tea.Batch(waitLine, tea.Tick(40ms, tick))`. Greeting reveals one rune per tick until complete. `lineMsg` → `Pairer.Feed` → events appended, or a `Running` command replaced in place by id → re-render → `SetContent`; if `follow` then `GotoBottom`. `WindowSizeMsg` → `SetWidth/SetHeight(height-1)` (footer takes one line) → re-render. Keys: quit set → `tea.Quit`; `home` → `GotoTop`, follow=false; `end` → `GotoBottom`, follow=true; other keys and wheel → `viewport.Update`, then `follow = AtBottom()`. Footer: `<session-id>  <n events>  <skipped>  ⇣ following | ⇡ paused`. View: `tea.NewView(vp.View()+"\n"+footer)`, `AltScreen: true`, `MouseMode: MouseModeCellMotion`.

- [ ] Step 1: Failing tests: `q`/`esc`/`ctrl+c` return `tea.Quit` cmd; after 30 lines and a window of 10 rows, `up` sets follow=false and a new line does not move the viewport; `end` restores follow.
- [ ] Step 2: Implement; `go test ./internal/ui/` passes.

### Task 5: main and end-to-end

**Files:**
- Create: `cmd/claude-companion/main.go`, `README.md`

- [ ] Step 1: main: usage on wrong arg count (exit 2); `Resolve(filepath.Join(home, ".claude", "projects"), arg)`; user from `os/user`; `Tail(ctx, path, 250ms)`; `tea.NewProgram(ui.New(cfg)).Run()`.
- [ ] Step 2: `go build ./...`, `go vet ./...`, `go test ./...` all clean.
- [ ] Step 3: Replay test: run the binary against a copy of a finished session in a pseudo-terminal (`script -q`) for 3 s, capture, assert the greeting and at least one `⚒` and one `📁` line appear; then append a fixture pair to the copy while running and assert the new event appears.
