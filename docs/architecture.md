# Architecture

claude-companion is four small packages in a straight line. Data flows one
way, from the transcript file to the screen, and every layer below the UI is
a pure function that can be tested with a string.

```
transcript file ──▶ transcript.Tail ──▶ event.Pairer ──▶ render.Event ──▶ ui.Model ──▶ terminal
   (jsonl)            complete lines        typed events     styled lines      viewport
```

## transcript

`Resolve(projectsDir, arg)` maps a session id or a unique prefix to the file
`<projectsDir>/<slug>/<id>.jsonl`. Subagent transcripts live one directory
deeper and never match.

`Tail(ctx, path, poll)` reads the file to the end, emits each complete line,
then polls the size. A line is emitted only once its newline has arrived, so a
half-written entry is never parsed. If the file shrinks, reading restarts from
the beginning. Cancelling the context closes the channel.

## event

`Pairer.Feed(line)` decodes one entry and returns zero or more events.

- An `assistant` entry with a `tool_use` block for Bash, Edit or Write is
  remembered under its id. Bash is emitted at once as a running `Command`.
- A `user` entry with the matching `tool_result` finishes the pair. The
  outcome comes from the entry-level `toolUseResult`: stdout and stderr for
  Bash, `structuredPatch` hunks for Edit and Write updates, the whole content
  for a Write that created the file. The exact string `User rejected tool
  use` marks a `Rejected`.
- A `user` entry that is neither a tool result, nor `isMeta`, nor a compact
  summary, and whose trimmed text does not start with `<`, is a `Prompt`.
  The `<` rule drops the entries Claude Code injects itself, such as task
  notifications and slash-command echoes.

Everything else in the file, including assistant text and thinking, is
ignored by construction: there is no event type for it.

## render

`Event(e, width, loc)` returns one styled line per screen row, never wider
than `width`. Colours are palette indices, so the terminal theme decides the
shades. `Greeting(...)` renders the opening block with a reveal count for the
typed effect.

## ui

`Model` owns a viewport, the event list, and a parallel list of rendered
lines. A new event appends; a result for a running command replaces the
entry with the same id in place. The view follows the tail while the user is
at the bottom and stops when they scroll up. Startup: the greeting types out
one character per tick, holds for a beat, then the log appears.

## Testing

Fixtures under `internal/event/testdata/` are two-line cuts of real tool
calls, one per event kind. `replay_test.go` runs the whole pipeline over the
concatenated fixtures through a real `Tail`, then appends lines while it
runs. An opt-in test replays any real transcript and checks event counts.
