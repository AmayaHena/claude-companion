// Package render turns events into styled terminal lines. It is pure: same
// event, width and location give the same lines.
package render

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/AmayaHena/claude-companion/internal/event"
)

// Row anatomy: a header row is "HH:MM" (5), a space, the two-cell glyph in
// columns 6-7, a space, then the title from column 9. A body row is 6
// spaces, the bar in column 6, a space, then the text from column 8, so the
// bar sits under the glyph. A continuation row of a wrapped command or
// prompt has no bar: 8 spaces. indentWidth is the body gutter.
const indentWidth = 8

// plainGutter is the bar-free gutter of a continuation row.
const plainGutter = "        "

const ellipsis = "…"

// Event renders one event as lines no wider than width cells, followed by
// one blank line.
func Event(e event.Event, width int, loc *time.Location) []string {
	var lines []string
	switch v := e.(type) {
	case event.Prompt:
		lines = prompt(v, width, loc)
	case event.Command:
		lines = command(v, width, loc)
	case event.FileChange:
		lines = fileChange(v, width, loc)
	case event.Rejected:
		lines = []string{rejected(v, width, loc)}
	case event.Agent:
		lines = []string{header(v.At, loc, glyphAgent, styleAgent.Render(clean(v.Description)), clean(v.Type), width)}
	case event.Skill:
		title := v.Name
		if v.Args != "" {
			title += " " + v.Args
		}
		lines = []string{header(v.At, loc, glyphSkill, styleSkill.Render(clean(title)), "", width)}
	default:
		return nil
	}
	return append(lines, "")
}

func stamp(at time.Time, loc *time.Location) string {
	return styleTime.Render(at.In(loc).Format("15:04"))
}

// lead is the start of a header row: clock and a space; the glyph follows.
// Without a glyph (prompts) the caller pads to indentWidth.
func lead(at time.Time, loc *time.Location) string {
	return stamp(at, loc) + " "
}

// gutter is the 8-cell start of a body row: six blanks under the clock, then
// the two-cell bar under the glyph.
func gutter(accent lipgloss.Style) string {
	return "      " + accent.Render(bar)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// tabWidth matches lipgloss's default TabWidth, so measuring before render
// and rendering agree.
const tabWidth = 4

// clean makes a line measurable and safe to print. Tabs become spaces
// (lipgloss would expand them at render time, after truncation measured them
// as zero width); a bare carriage return keeps only what follows it, which is
// what a terminal would have displayed; then Sanitize removes every terminal
// control.
func clean(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 { // a single row never holds a newline
		s = s[:i]
	}
	if i := strings.LastIndexByte(s, '\r'); i >= 0 {
		s = s[i+1:]
	}
	return Sanitize(strings.ReplaceAll(s, "\t", strings.Repeat(" ", tabWidth)))
}

// Sanitize makes untrusted transcript text inert for a terminal: ESC becomes
// the visible ␛ (U+241B) so the rest of a sequence reads as plain text; the
// other C0 controls, DEL and the C1 controls (U+0080–U+009F, which many
// terminals treat as CSI/OSC introducers) are dropped; so are the invisible
// format characters that can reorder or pad displayed text (bidi controls,
// zero-width spaces, soft hyphen, BOM; the zero-width joiner U+200D stays so
// emoji sequences keep their shape); invalid UTF-8 becomes U+FFFD. Every
// string that comes from a transcript must pass through here before it
// reaches the screen, the title, the footer or the picker.
func Sanitize(s string) string { return sanitize(s, false) }

// SanitizeText is Sanitize for multi-line text that is not drawn as a row:
// newlines and tabs are kept. It is what the clipboard receives.
func SanitizeText(s string) string { return sanitize(s, true) }

func sanitize(s string, keepLayout bool) string {
	s = strings.ToValidUTF8(s, "\ufffd")
	return strings.Map(func(r rune) rune {
		switch {
		case keepLayout && (r == '\n' || r == '\t'):
			return r
		case r == 0x1b:
			return '\u241b'
		case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
			return -1
		case r == 0x200b, r == 0x200c, r == 0x200e, r == 0x200f, // zero-width space, non-joiner, marks
			r >= 0x202a && r <= 0x202e, // bidi embeddings and overrides
			r >= 0x2066 && r <= 0x2069, // bidi isolates
			r == 0x00ad, r == 0xfeff:   // soft hyphen, byte order mark
			return -1
		}
		return r
	}, s)
}

// maxBodyRows caps the rows drawn for one body (a command's output, a
// created file, the hunks of an edit); the rest is one trailer row. Keeps a
// multi-million-line write from freezing the viewer on every resize.
const maxBodyRows = 2000

// trailer is the row that stands for the rows not drawn.
func trailer(g string, more int) string {
	return g + styleMeta.Render(fmt.Sprintf("… %d more lines", more))
}

// fitStyled truncates an already-styled string to width cells without
// cutting an escape sequence, ending with … when cut.
func fitStyled(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(s, width, ellipsis)
}

// fit truncates a plain string to at most width cells, ending with … when cut.
func fit(s string, width int) string {
	s = clean(s)
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width == 1 {
		return ellipsis
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > width {
		r = r[:len(r)-1]
	}
	return string(r) + ellipsis
}

// header composes "HH:MM <glyph> <title>[  <suffix>]" within width. The
// title arrives already styled and is truncated escape-safely.
func header(at time.Time, loc *time.Location, glyph, styledTitle, suffix string, width int) string {
	prefix := lead(at, loc) + glyph + " "
	room := width - lipgloss.Width(prefix)
	if suffix != "" {
		room -= 2 + lipgloss.Width(suffix)
	}
	line := prefix + fitStyled(styledTitle, room)
	if suffix != "" {
		line += "  " + styleMeta.Render(suffix)
	}
	return line
}

func body(accent, style lipgloss.Style, text string, width int) []string {
	if text == "" {
		return nil
	}
	text = strings.TrimRight(text, "\n")
	lines := strings.Split(text, "\n")
	out := make([]string, 0, min(len(lines), maxBodyRows)+1)
	for i, l := range lines {
		if i == maxBodyRows {
			out = append(out, trailer(gutter(accent), len(lines)-i))
			break
		}
		out = append(out, gutter(accent)+style.Render(fit(l, width-indentWidth)))
	}
	return out
}

// wrapRows word-wraps plain text to the given width (a word longer than the
// width is broken) and highlights each row on its own, so every row is
// self-contained.
func wrapRows(lx chroma.Lexer, text string, width int) []string {
	if width <= 0 {
		return []string{""}
	}
	return highlightLines(lx, wrapPlain(text, width))
}

// wrapPlain word-wraps cleaned plain text and trims the trailing space
// ansi.Wrap leaves at a break point. ansi.Wrap can hand back a row one cell
// over the limit when it breaks at a hyphen (seen on `echo "--` in a real
// command), so any row still too wide is hard-wrapped.
func wrapPlain(text string, width int) []string {
	var rows []string
	for _, row := range strings.Split(ansi.Wrap(clean(text), width, ""), "\n") {
		if lipgloss.Width(row) > width {
			rows = append(rows, strings.Split(ansi.Hardwrap(row, width, false), "\n")...)
			continue
		}
		rows = append(rows, row)
	}
	for i := range rows {
		rows[i] = strings.TrimRight(rows[i], " ")
	}
	return rows
}

// gitWriteRE matches a command that creates, rewrites or publishes git or
// GitHub state: a `git` word followed by a writing subcommand (the owner's
// approval list: add, commit, merge, rebase, cherry-pick, revert, reset,
// clean, filter-branch, tag, push, stash drop, branch/checkout/switch/worktree
// creating a ref), or a `gh` word followed by pr create/merge/ready, release
// create, repo sync, or an api call with a writing HTTP method. The rule is
// literal on the command text: `echo 'git push'` is flagged too.
var gitWriteRE = regexp.MustCompile(`(^|[^[:alnum:]_])(` +
	`git\s+(add|commit|merge|rebase|cherry-pick|revert|reset|clean|filter-branch|tag|push|stash\s+drop|branch\s+-[dDm]|checkout\s+(-b|--orphan)|switch\s+-c|worktree\s+add)` +
	`|gh\s+(pr\s+(create|merge|ready)|release\s+create|repo\s+sync|api\s+.*(-X|--method)\s+(POST|PUT|PATCH|DELETE))` +
	`)($|[^[:alnum:]_-])`)

// netRE matches a command that obviously reaches the network: a fetching
// tool, a remote shell or copy, a network probe, a git transfer, a GitHub CLI
// call, a package manager fetch, a container registry call or a cloud CLI.
// Literal on the command text like gitWriteRE.
var netRE = regexp.MustCompile(`(^|[^[:alnum:]_./-])(` +
	`curl|wget|ssh|scp|sftp|rsync|nc|ncat|telnet|ping|dig|nslookup|traceroute` +
	`|git\s+(fetch|pull|clone|ls-remote|push)|gh` +
	`|npm\s+(install|i|ci|add|publish|update)|npx|yarn|pnpm|pip3?\s+install|go\s+(get|mod\s+download)|brew\s+(install|upgrade|update|fetch)` +
	`|docker\s+(pull|push|login)|aws|gcloud|az|kubectl|helm|apt(-get)?\s+(install|update|upgrade)` +
	`)($|[^[:alnum:]_-])`)

// Marks reports the header marks of a command: warn for a git/GitHub write,
// net for a network command that is not already a warning (the warning wins:
// `git push` is ⚠️, never 🛜). The footer counts them.
func Marks(cmd string) (warn, net bool) {
	cmd = SanitizeText(cmd) // decide on the text the user reads, not on bytes Sanitize removes
	warn = gitWriteRE.MatchString(cmd)
	return warn, !warn && netRE.MatchString(cmd)
}

// Glyph is the two-cell glyph that names an event's type in its header and
// in the footer tally. Prompts have none.
func Glyph(e event.Event) string {
	switch v := e.(type) {
	case event.Command:
		if !v.Running && !v.Background && (!v.ExitKnown || v.Exit != 0) {
			return glyphAlert
		}
		return glyphCommand
	case event.FileChange:
		if v.Kind == event.Create {
			return glyphCreate
		}
		return glyphEdit
	case event.Rejected:
		return glyphAlert
	case event.Agent:
		return glyphAgent
	case event.Skill:
		return glyphSkill
	}
	return ""
}

func command(c event.Command, width int, loc *time.Location) []string {
	suffix := ""
	warn, net := Marks(c.Cmd)
	// The header glyph is the type glyph (⚒️, or ❗ when the command failed),
	// preceded by the warning mark for a git write, else the network mark.
	// The body bar takes the hue of that type glyph.
	glyph := Glyph(c)
	accent := accentCommand
	if glyph == glyphAlert {
		accent = accentFail
	}
	switch {
	case warn:
		glyph = glyphWarn + " " + glyph
	case net:
		glyph = glyphNet + " " + glyph
	}
	switch {
	case c.Running:
		suffix = "…"
	case c.Background:
		suffix = "bg"
	case !c.ExitKnown:
		suffix = "exit ?"
	case c.Exit != 0:
		suffix = fmt.Sprintf("exit %d", c.Exit)
	}
	// The command wraps onto continuation rows instead of being cut. Rows are
	// wrapped to the room left by the header prefix (clock, marks, glyph),
	// which is at least as narrow as the 9-cell continuation gutter, so every
	// row fits. The suffix (exit status, running, background) follows the
	// last row, on its own row if it would not fit.
	room := width - lipgloss.Width(lead(c.At, loc)+glyph+" ")
	var rows []string
	if warn {
		for _, r := range wrapPlain(firstLine(c.Cmd), room) {
			rows = append(rows, styleWarn.Render(r))
		}
	} else {
		rows = wrapRows(bashLexer, firstLine(c.Cmd), room)
	}
	lines := []string{header(c.At, loc, glyph, rows[0], "", width)}
	for _, r := range rows[1:] {
		lines = append(lines, plainGutter+" "+r) // under the title, no bar
	}
	if suffix != "" {
		last := lines[len(lines)-1]
		if lipgloss.Width(last)+2+lipgloss.Width(suffix) <= width {
			lines[len(lines)-1] = last + "  " + styleMeta.Render(suffix)
		} else {
			lines = append(lines, plainGutter+" "+styleMeta.Render(suffix))
		}
	}
	if c.Running {
		return lines
	}
	lines = append(lines, body(accent, styleOut, c.Stdout, width)...)
	lines = append(lines, body(accent, styleErr, c.Stderr, width)...)
	return lines
}

func fileChange(fc event.FileChange, width int, loc *time.Location) []string {
	glyph, accent := glyphEdit, accentEdit
	if fc.Kind == event.Create {
		glyph, accent = glyphCreate, accentCreate
	}
	lines := []string{header(fc.At, loc, glyph, stylePath.Render(clean(fc.Path)), "", width)}
	room := width - indentWidth
	g := gutter(accent)
	lx := lexerFor(fc.Path)
	if fc.Kind == event.Create {
		codes := strings.Split(strings.TrimRight(fc.Content, "\n"), "\n")
		more := 0
		if len(codes) > maxBodyRows {
			more, codes = len(codes)-maxBodyRows, codes[:maxBodyRows]
		}
		for i := range codes {
			codes[i] = clean(codes[i])
		}
		for _, code := range highlightLines(lx, codes) {
			lines = append(lines, g+styleSignAdd.Render("+")+fitStyled(code, room-1))
		}
		if more > 0 {
			lines = append(lines, trailer(g, more))
		}
		return lines
	}
	drawn := 0 // hunk rows drawn so far; the cap spans all hunks of the change
	for hi, h := range fc.Hunks {
		if drawn >= maxBodyRows {
			more := 0
			for _, rest := range fc.Hunks[hi:] {
				more += len(rest.Lines)
			}
			lines = append(lines, trailer(g, more))
			break
		}
		lines = append(lines, g+styleHunk.Render(fit(fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldLines, h.NewStart, h.NewLines), room)))
		hl := h.Lines
		more := 0
		if drawn+len(hl) > maxBodyRows {
			keep := maxBodyRows - drawn
			more, hl = len(hl)-keep, hl[:keep]
			for _, rest := range fc.Hunks[hi+1:] {
				more += len(rest.Lines)
			}
		}
		// Split each hunk line into its sign and its code; highlight the code
		// of the whole hunk at once so multi-line constructs are recognised.
		// The sign is one of ' ', '-', '+'; any other first rune means the
		// line is code with no sign (never split a multi-byte rune).
		signs := make([]string, len(hl))
		codes := make([]string, len(hl))
		for i, l := range hl {
			l = clean(l)
			r, size := utf8.DecodeRuneInString(l)
			if r == ' ' || r == '-' || r == '+' {
				signs[i], codes[i] = l[:size], l[size:]
			} else {
				signs[i], codes[i] = " ", l
			}
		}
		for i, code := range highlightLines(lx, codes) {
			sign := " "
			switch signs[i] {
			case "-":
				sign = styleSignDel.Render("-")
			case "+":
				sign = styleSignAdd.Render("+")
			}
			lines = append(lines, g+sign+fitStyled(code, room-1))
		}
		drawn += len(hl)
		if more > 0 {
			lines = append(lines, trailer(g, more))
			break
		}
	}
	return lines
}

func rejected(r event.Rejected, width int, loc *time.Location) string {
	return header(r.At, loc, glyphAlert, styleCmd.Render(clean(r.Tool+" "+firstLine(r.Subject))), "rejected", width)
}

// prompt renders the user's message in full: clock, then every line of the
// prompt in bold, each wrapped onto continuation rows; a blank line of the
// prompt stays a blank row.
func prompt(p event.Prompt, width int, loc *time.Location) []string {
	var lines []string
	for _, text := range strings.Split(strings.TrimRight(p.Text, "\n"), "\n") {
		if strings.TrimSpace(text) == "" && len(lines) > 0 {
			lines = append(lines, "")
			continue
		}
		for _, r := range wrapPlain(text, width-indentWidth) {
			gutter := plainGutter // no bar on continuation rows
			if len(lines) == 0 {
				gutter = lead(p.At, loc) + "  " // no glyph: pad to the body column
			}
			lines = append(lines, gutter+stylePrompt.Render(r))
		}
	}
	return lines
}
