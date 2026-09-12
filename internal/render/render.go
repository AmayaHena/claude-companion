// Package render turns events into styled terminal lines. It is pure: same
// event, width and location give the same lines.
package render

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"claude-companion/internal/event"
)

// indent is the column where output and diff lines start: "HH:MM:SS " (9)
// plus a one-cell glyph and one space.
const indent = "          "

const ellipsis = "…"

// Event renders one event as lines no wider than width cells.
func Event(e event.Event, width int, loc *time.Location) []string {
	switch v := e.(type) {
	case event.Prompt:
		return []string{prompt(v, width, loc)}
	case event.Command:
		return command(v, width, loc)
	case event.FileChange:
		return fileChange(v, width, loc)
	case event.Rejected:
		return []string{rejected(v, width, loc)}
	}
	return nil
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
	return styleTime.Render(at.In(loc).Format("15:04:05"))
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// fit truncates a plain string to at most width cells, ending with … when cut.
func fit(s string, width int) string {
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

// header composes "HH:MM:SS <glyph> <title>[  <suffix>]" within width.
func header(at time.Time, loc *time.Location, glyph string, titleStyle lipgloss.Style, title, suffix string, width int) string {
	prefix := stamp(at, loc) + " " + glyph + strings.Repeat(" ", 3-lipgloss.Width(glyph))
	room := width - lipgloss.Width(prefix)
	if suffix != "" {
		room -= 2 + lipgloss.Width(suffix)
	}
	line := prefix + titleStyle.Render(fit(title, room))
	if suffix != "" {
		line += "  " + styleMeta.Render(suffix)
	}
	return line
}

func body(style lipgloss.Style, text string, width int) []string {
	if text == "" {
		return nil
	}
	text = strings.TrimRight(text, "\n")
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, indent+style.Render(fit(l, width-len(indent))))
	}
	return out
}

func command(c event.Command, width int, loc *time.Location) []string {
	glyph, suffix := glyphCommand, ""
	switch {
	case c.Running:
		suffix = "…"
	case c.Background:
		suffix = "bg"
	case !c.ExitKnown:
		glyph, suffix = glyphAlert, "exit ?"
	case c.Exit != 0:
		glyph, suffix = glyphAlert, fmt.Sprintf("exit %d", c.Exit)
	}
	lines := []string{header(c.At, loc, glyph, styleCmd, firstLine(c.Cmd), suffix, width)}
	if c.Running {
		return lines
	}
	lines = append(lines, body(styleOut, c.Stdout, width)...)
	lines = append(lines, body(styleErr, c.Stderr, width)...)
	return lines
}

func fileChange(fc event.FileChange, width int, loc *time.Location) []string {
	glyph := glyphEdit
	if fc.Kind == event.Create {
		glyph = glyphCreate
	}
	lines := []string{header(fc.At, loc, glyph, stylePath, fc.Path, "", width)}
	room := width - len(indent)
	if fc.Kind == event.Create {
		for _, l := range strings.Split(strings.TrimRight(fc.Content, "\n"), "\n") {
			lines = append(lines, indent+styleAdd.Render(fit("+"+l, room)))
		}
		return lines
	}
	for _, h := range fc.Hunks {
		lines = append(lines, indent+styleHunk.Render(fit(fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldLines, h.NewStart, h.NewLines), room)))
		for _, l := range h.Lines {
			st := styleCtx
			switch {
			case strings.HasPrefix(l, "-"):
				st = styleDel
			case strings.HasPrefix(l, "+"):
				st = styleAdd
			}
			lines = append(lines, indent+st.Render(fit(l, room)))
		}
	}
	return lines
}

func rejected(r event.Rejected, width int, loc *time.Location) string {
	return header(r.At, loc, glyphAlert, styleCmd, r.Tool+" "+firstLine(r.Subject), "rejected", width)
}

// prompt renders "── HH:MM:SS  text ──────" filling exactly width cells.
func prompt(p event.Prompt, width int, loc *time.Location) string {
	lead := "── " + p.At.In(loc).Format("15:04:05") + "  "
	text := fit(firstLine(p.Text), width-lipgloss.Width(lead)-3)
	line := lead + text + " "
	if pad := width - lipgloss.Width(line); pad > 0 {
		line += strings.Repeat("─", pad)
	}
	return stylePrompt.Render(line)
}
