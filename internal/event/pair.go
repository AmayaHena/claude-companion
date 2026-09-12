package event

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// entry is the subset of a transcript line the pairer reads. Every field is
// optional; a missing field decodes to its zero value.
type entry struct {
	Type             string          `json:"type"`
	UUID             string          `json:"uuid"`
	Timestamp        time.Time       `json:"timestamp"`
	IsMeta           bool            `json:"isMeta"`
	IsCompactSummary bool            `json:"isCompactSummary"`
	Message          message         `json:"message"`
	ToolUseResult    json.RawMessage `json:"toolUseResult"`
}

type message struct {
	Content json.RawMessage `json:"content"`
}

type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
	Content   json.RawMessage `json:"content"`
}

type pending struct {
	name  string
	input json.RawMessage
	at    time.Time
}

// Pairer matches tool_use blocks with their tool_result and remembers how
// many lines it could not parse.
type Pairer struct {
	pending map[string]pending
	skipped int
}

func NewPairer() *Pairer { return &Pairer{pending: map[string]pending{}} }

// Skipped is the number of non-blank lines that were not valid JSON objects.
func (p *Pairer) Skipped() int { return p.skipped }

// rejectedMarker is the exact toolUseResult Claude Code writes when the user
// refuses a tool use.
const rejectedMarker = "User rejected tool use"

var exitCodeRE = regexp.MustCompile(`^Exit code (\d+)\n?`)

// Feed consumes one transcript line and returns the events it produces, in
// order. Blank lines produce nothing. Malformed lines return an error and
// are counted in Skipped.
func (p *Pairer) Feed(line string) ([]Event, error) {
	if strings.TrimSpace(line) == "" {
		return nil, nil
	}
	var e entry
	if err := json.Unmarshal([]byte(line), &e); err != nil {
		p.skipped++
		return nil, err
	}
	switch e.Type {
	case "assistant":
		return p.assistant(e), nil
	case "user":
		return p.user(e), nil
	}
	return nil, nil
}

func (p *Pairer) assistant(e entry) []Event {
	var out []Event
	for _, b := range blocks(e.Message.Content) {
		if b.Type != "tool_use" {
			continue
		}
		switch b.Name {
		case "Bash", "Edit", "Write":
			p.pending[b.ID] = pending{name: b.Name, input: b.Input, at: e.Timestamp}
			if b.Name == "Bash" {
				out = append(out, Command{ID: b.ID, At: e.Timestamp, Cmd: inputString(b.Input, "command"), Running: true})
			}
		}
	}
	return out
}

func (p *Pairer) user(e entry) []Event {
	var out []Event
	bl := blocks(e.Message.Content)
	sawResult := false
	for _, b := range bl {
		if b.Type != "tool_result" {
			continue
		}
		sawResult = true
		pd, ok := p.pending[b.ToolUseID]
		if !ok {
			continue
		}
		delete(p.pending, b.ToolUseID)
		out = append(out, decode(b.ToolUseID, pd, b, e.ToolUseResult))
	}
	if sawResult || e.IsMeta || e.IsCompactSummary {
		return out
	}
	if text, ok := promptText(e.Message.Content, bl); ok {
		out = append(out, Prompt{ID: e.UUID, At: e.Timestamp, Text: text})
	}
	return out
}

// decode builds the final event for a pending tool use from its result block
// b and the entry-level toolUseResult r.
func decode(id string, pd pending, b block, r json.RawMessage) Event {
	var rejected string
	if b.IsError && json.Unmarshal(r, &rejected) == nil && rejected == rejectedMarker {
		subject := inputString(pd.input, "file_path")
		if pd.name == "Bash" {
			subject = inputString(pd.input, "command")
		}
		return Rejected{ID: id, At: pd.at, Tool: pd.name, Subject: subject}
	}
	switch pd.name {
	case "Bash":
		return decodeBash(id, pd, b, r)
	case "Edit":
		return decodeEdit(id, pd, r)
	default:
		return decodeWrite(id, pd, r)
	}
}

func decodeBash(id string, pd pending, b block, r json.RawMessage) Command {
	c := Command{ID: id, At: pd.at, Cmd: inputString(pd.input, "command")}
	text := contentText(b.Content)
	if !b.IsError {
		c.Exit, c.ExitKnown = 0, true
	} else if m := exitCodeRE.FindStringSubmatch(text); m != nil {
		c.Exit, _ = strconv.Atoi(m[1])
		c.ExitKnown = true
		text = text[len(m[0]):]
	}
	var res struct {
		Stdout           string `json:"stdout"`
		Stderr           string `json:"stderr"`
		BackgroundTaskID string `json:"backgroundTaskId"`
	}
	if json.Unmarshal(r, &res) == nil && (res.Stdout != "" || res.Stderr != "" || res.BackgroundTaskID != "") {
		c.Stdout, c.Stderr = res.Stdout, res.Stderr
		c.Background = res.BackgroundTaskID != ""
	}
	if c.Stdout == "" && c.Stderr == "" {
		c.Stdout = text
	}
	return c
}

type patchResult struct {
	Type            string `json:"type"`
	FilePath        string `json:"filePath"`
	Content         string `json:"content"`
	StructuredPatch []struct {
		OldStart int      `json:"oldStart"`
		OldLines int      `json:"oldLines"`
		NewStart int      `json:"newStart"`
		NewLines int      `json:"newLines"`
		Lines    []string `json:"lines"`
	} `json:"structuredPatch"`
}

func (pr patchResult) hunks() []Hunk {
	hs := make([]Hunk, 0, len(pr.StructuredPatch))
	for _, h := range pr.StructuredPatch {
		hs = append(hs, Hunk{OldStart: h.OldStart, OldLines: h.OldLines, NewStart: h.NewStart, NewLines: h.NewLines, Lines: h.Lines})
	}
	return hs
}

func decodeEdit(id string, pd pending, r json.RawMessage) FileChange {
	var pr patchResult
	_ = json.Unmarshal(r, &pr)
	path := pr.FilePath
	if path == "" {
		path = inputString(pd.input, "file_path")
	}
	return FileChange{ID: id, At: pd.at, Path: path, Kind: Update, Hunks: pr.hunks()}
}

func decodeWrite(id string, pd pending, r json.RawMessage) FileChange {
	var pr patchResult
	_ = json.Unmarshal(r, &pr)
	path := pr.FilePath
	if path == "" {
		path = inputString(pd.input, "file_path")
	}
	fc := FileChange{ID: id, At: pd.at, Path: path, Kind: Update, Hunks: pr.hunks()}
	if pr.Type == "create" {
		fc.Kind = Create
		fc.Hunks = nil
		fc.Content = pr.Content
		if fc.Content == "" {
			fc.Content = inputString(pd.input, "content")
		}
	}
	return fc
}

// blocks decodes a content list; a string content yields no blocks.
func blocks(raw json.RawMessage) []block {
	var bl []block
	if len(raw) == 0 || raw[0] != '[' {
		return nil
	}
	_ = json.Unmarshal(raw, &bl)
	return bl
}

// contentText flattens a tool_result content, which is a string or a list of
// text blocks.
func contentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if raw[0] == '"' {
		var s string
		_ = json.Unmarshal(raw, &s)
		return s
	}
	var sb strings.Builder
	for _, b := range blocks(raw) {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return sb.String()
}

// promptText applies the prompt rule: content is a string, or text blocks
// possibly accompanied by image blocks (a prompt with screenshots), and the
// trimmed text does not start with '<' (system-injected). Any other block
// type means the entry is not a prompt.
func promptText(raw json.RawMessage, bl []block) (string, bool) {
	var text string
	switch {
	case len(raw) > 0 && raw[0] == '"':
		_ = json.Unmarshal(raw, &text)
	case len(bl) > 0:
		var sb strings.Builder
		for _, b := range bl {
			switch b.Type {
			case "text":
				sb.WriteString(b.Text)
			case "image":
			default:
				return "", false
			}
		}
		text = sb.String()
	default:
		return "", false
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || strings.HasPrefix(trimmed, "<") {
		return "", false
	}
	return trimmed, true
}

func inputString(raw json.RawMessage, key string) string {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	var s string
	_ = json.Unmarshal(m[key], &s)
	return s
}
