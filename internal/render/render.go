// Package render turns events into styled terminal lines. It is pure: same
// event, width and location give the same lines.
package render

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/x/ansi"

	"claude-companion/internal/event"
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
// one blank line. shade selects the dark (0) or light (1) variant of the
// type's accent; the caller alternates it between consecutive events of the
// same type.
func Event(e event.Event, width int, loc *time.Location, shade int) []string {
	shade &= 1
	var lines []string
	switch v := e.(type) {
	case event.Prompt:
		lines = prompt(v, width, loc)
	case event.Command:
		lines = command(v, width, loc, accentCommand[shade])
	case event.FileChange:
		lines = fileChange(v, width, loc, accentFile[shade])
	case event.Rejected:
		lines = []string{rejected(v, width, loc)}
	default:
		return nil
	}
	return append(lines, "")
}

// Greeting renders the opening block. revealed is how many runes of the
// "hello, <user>" line to show; the rest of the block appears only once the
// line is complete.
func Greeting(user, session, cwd string, started time.Time, loc *time.Location, revealed int) []string {
	hello := "hello, " + user
	r := []rune(hello)
	if revealed < len(r) {
		if revealed < 0 {
			revealed = 0
		}
		return []string{styleHello.Render(string(r[:revealed]))}
	}
	return []string{
		styleHello.Render(hello),
		"",
		styleLabel.Render("session  ") + styleValue.Render(session),
		styleLabel.Render("cwd      ") + styleValue.Render(cwd),
		styleLabel.Render("started  ") + styleValue.Render(started.In(loc).Format("2006-01-02 15:04:05")),
	}
}

func stamp(at time.Time, loc *time.Location) string {
	return styleTime.Render(at.In(loc).Format("15:04"))
}

// lead is the start of a header row: clock and a space; the glyph follows.
// Without a glyph (prompts) the caller pads to indentWidth.
func lead(at time.Time, loc *time.Location) string {
	return stamp(at, loc) + " "
}

// gutter is the 8-cell start of a body row: blanks under the clock, then the bar.
func gutter(accent lipgloss.Style) string {
	return "      " + accent.Render(bar) + " "
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

// clean makes a line measurable: tabs become spaces (lipgloss would expand
// them at render time, after truncation measured them as zero width), and a
// bare carriage return keeps only what follows it, which is what a terminal
// would have displayed.
func clean(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 { // a single row never holds a newline
		s = s[:i]
	}
	if i := strings.LastIndexByte(s, '\r'); i >= 0 {
		s = s[i+1:]
	}
	return strings.ReplaceAll(s, "\t", strings.Repeat(" ", tabWidth))
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
	out := make([]string, 0, len(lines))
	for _, l := range lines {
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
// ansi.Wrap leaves at a break point.
func wrapPlain(text string, width int) []string {
	rows := strings.Split(ansi.Wrap(clean(text), width, ""), "\n")
	for i := range rows {
		rows[i] = strings.TrimRight(rows[i], " ")
	}
	return rows
}

// gitWriteRE matches a command that creates, rewrites or publishes git
// state: a `git` word followed by add, commit or push as its subcommand.
var gitWriteRE = regexp.MustCompile(`(^|[^[:alnum:]_])git\s+(add|commit|push)($|[^[:alnum:]_])`)

func command(c event.Command, width int, loc *time.Location, accent lipgloss.Style) []string {
	glyph, suffix := glyphCommand, ""
	warn := gitWriteRE.MatchString(c.Cmd)
	if warn {
		glyph = glyphWarn
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
	if suffix != "" && suffix != "…" && suffix != "bg" && !warn {
		glyph = glyphAlert
	}
	// The command wraps onto continuation rows instead of being cut. The
	// suffix (exit status, running, background) follows the last row, on its
	// own row if it would not fit.
	var rows []string
	if warn {
		for _, r := range wrapPlain(firstLine(c.Cmd), width-len(plainGutter)-1) {
			rows = append(rows, styleWarn.Render(r))
		}
	} else {
		rows = wrapRows(bashLexer, firstLine(c.Cmd), width-len(plainGutter)-1)
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

func fileChange(fc event.FileChange, width int, loc *time.Location, accent lipgloss.Style) []string {
	glyph := glyphEdit
	if fc.Kind == event.Create {
		glyph = glyphCreate
	}
	lines := []string{header(fc.At, loc, glyph, stylePath.Render(clean(fc.Path)), "", width)}
	room := width - indentWidth
	g := gutter(accent)
	lx := lexerFor(fc.Path)
	if fc.Kind == event.Create {
		codes := strings.Split(strings.TrimRight(fc.Content, "\n"), "\n")
		for i := range codes {
			codes[i] = clean(codes[i])
		}
		for _, code := range highlightLines(lx, codes) {
			lines = append(lines, g+styleSignAdd.Render("+")+fitStyled(code, room-1))
		}
		return lines
	}
	for _, h := range fc.Hunks {
		lines = append(lines, g+styleHunk.Render(fit(fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldLines, h.NewStart, h.NewLines), room)))
		// Split each hunk line into its sign and its code; highlight the code
		// of the whole hunk at once so multi-line constructs are recognised.
		signs := make([]string, len(h.Lines))
		codes := make([]string, len(h.Lines))
		for i, l := range h.Lines {
			l = clean(l)
			if l == "" {
				l = " "
			}
			signs[i], codes[i] = l[:1], l[1:]
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
	}
	return lines
}

func rejected(r event.Rejected, width int, loc *time.Location) string {
	return header(r.At, loc, glyphAlert, styleCmd.Render(clean(r.Tool+" "+firstLine(r.Subject))), "rejected", width)
}

// prompt renders the user's message: clock, then the bold first line of the
// prompt, wrapped onto continuation rows under the bar.
func prompt(p event.Prompt, width int, loc *time.Location) []string {
	l := lead(p.At, loc) + "  " // no glyph: pad to the body column
	rows := wrapPlain(firstLine(p.Text), width-indentWidth)
	lines := []string{l + stylePrompt.Render(rows[0])}
	for _, r := range rows[1:] {
		lines = append(lines, plainGutter+stylePrompt.Render(r)) // no bar on continuation rows
	}
	return lines
}
