// Package event turns raw transcript lines into the things the viewer shows:
// prompts, commands, file changes, rejected tool uses, subagent launches and
// skill invocations. Assistant text and thinking never become events.
package event

import "time"

// Event is one displayable item. EventID is stable across the running and
// finished states of a command so the UI can replace it in place.
type Event interface {
	EventID() string
	When() time.Time
}

// Prompt is a message typed by the user (not a system-injected one).
type Prompt struct {
	ID   string
	At   time.Time
	Text string
}

// Command is a Bash tool call. While the result has not arrived Running is
// true and the output fields are empty.
type Command struct {
	ID         string
	At         time.Time
	Cmd        string
	Stdout     string
	Stderr     string
	Running    bool
	Background bool
	Exit       int
	ExitKnown  bool
}

// Kind distinguishes a created file from a modified one.
type Kind int

const (
	Update Kind = iota
	Create
)

// Hunk mirrors one element of Claude Code's structuredPatch: unified-diff
// coordinates plus lines prefixed by ' ', '-' or '+'.
type Hunk struct {
	OldStart, OldLines int
	NewStart, NewLines int
	Lines              []string
}

// FileChange is an Edit or Write that succeeded. A Create carries the whole
// file in Content and no hunks; an Update carries hunks and no Content.
type FileChange struct {
	ID      string
	At      time.Time
	Path    string
	Kind    Kind
	Hunks   []Hunk
	Content string
}

// Rejected is a Bash, Edit or Write the user refused. Subject is the command
// text or the file path.
type Rejected struct {
	ID      string
	At      time.Time
	Tool    string
	Subject string
}

// Agent is a subagent launch (the Agent tool). It is shown when the tool_use
// appears; the subagent's own work is out of scope.
type Agent struct {
	ID          string
	At          time.Time
	Description string
	Type        string // subagent_type
}

// Skill is a skill invocation (the Skill tool).
type Skill struct {
	ID   string
	At   time.Time
	Name string
	Args string
}

func (p Prompt) EventID() string     { return p.ID }
func (p Prompt) When() time.Time     { return p.At }
func (c Command) EventID() string    { return c.ID }
func (c Command) When() time.Time    { return c.At }
func (f FileChange) EventID() string { return f.ID }
func (f FileChange) When() time.Time { return f.At }
func (r Rejected) EventID() string   { return r.ID }
func (r Rejected) When() time.Time   { return r.At }
func (a Agent) EventID() string      { return a.ID }
func (a Agent) When() time.Time      { return a.At }
func (s Skill) EventID() string      { return s.ID }
func (s Skill) When() time.Time      { return s.At }
