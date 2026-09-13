package render

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/AmayaHena/claude-companion/internal/event"
)

// The marks are decided on the same text the user reads: a character that
// Sanitize removes without a trace (NUL, DEL, a zero-width space, a soft
// hyphen) inside `git push` leaves the row reading `git push`, so it must
// carry the warning. ESC is shown as ␛, so `gi␛[0mt` is not `git` and is
// rightly unmarked.
func TestMarksAreDecidedOnSanitisedText(t *testing.T) {
	for _, cmd := range []string{"git\x00 push", "git\x7f push", "gi\u200bt push origin", "cu\u00adrl https://x", "curl\u202e https://x"} {
		lines := plain(Event(event.Command{ID: "t", At: at, Cmd: cmd, ExitKnown: true}, 120, time.UTC))
		if !strings.Contains(lines[0], "⚠️") && !strings.Contains(lines[0], "\U0001f6dc") {
			t.Errorf("%q renders %q without a mark", cmd, lines[0])
		}
	}
	if got := plain(Event(event.Command{ID: "t", At: at, Cmd: "gi\x1b[0mt push", ExitKnown: true}, 120, time.UTC))[0]; strings.Contains(got, "⚠️") {
		t.Errorf("a visible ␛ breaks the word, no mark expected: %q", got)
	}
	if w, _ := Marks("git\x00 push"); !w {
		t.Error("Marks must sanitise before matching")
	}
}

// Bodies are capped: a created file, a hunk or a command output with more
// rows than maxBodyRows shows the first maxBodyRows and one trailer naming
// the rest, so a multi-million-line write cannot freeze the viewer.
func TestHugeBodiesAreCapped(t *testing.T) {
	big := strings.Repeat("line\n", maxBodyRows+123)
	create := Event(event.FileChange{ID: "t", At: at, Path: "/tmp/big.txt", Kind: event.Create, Content: big}, 80, time.UTC)
	if len(create) != 1+maxBodyRows+1+1 { // header + rows + trailer + blank
		t.Fatalf("create rows = %d", len(create))
	}
	if tr := plain(create)[len(create)-2]; tr != "      ▕▏… 123 more lines" {
		t.Fatalf("create trailer = %q", tr)
	}
	out := Event(event.Command{ID: "t", At: at, Cmd: "cat big", Stdout: big, ExitKnown: true}, 80, time.UTC)
	if len(out) != 1+maxBodyRows+1+1 || !strings.Contains(plain(out)[len(out)-2], "… 123 more lines") {
		t.Fatalf("stdout rows = %d last = %q", len(out), plain(out)[len(out)-2])
	}
	hunkLines := make([]string, maxBodyRows+5)
	for i := range hunkLines {
		hunkLines[i] = "+x"
	}
	edit := Event(event.FileChange{ID: "t", At: at, Path: "/tmp/big.go", Kind: event.Update, Hunks: []event.Hunk{{Lines: hunkLines}}}, 80, time.UTC)
	if len(edit) != 1+1+maxBodyRows+1+1 || !strings.Contains(plain(edit)[len(edit)-2], "… 5 more lines") { // header + @@ + rows + trailer + blank
		t.Fatalf("hunk rows = %d last = %q", len(edit), plain(edit)[len(edit)-2])
	}
	small := Event(event.FileChange{ID: "t", At: at, Path: "/tmp/s.txt", Kind: event.Create, Content: "a\nb\n"}, 80, time.UTC)
	if len(small) != 4 || strings.Contains(strings.Join(plain(small), ""), "more lines") {
		t.Fatalf("small file must not be capped: %q", plain(small))
	}
}

// Invisible and direction-changing format characters can reorder or pad a
// displayed command; Sanitize drops them. The zero-width joiner stays so
// emoji sequences in prompts keep their shape.
func TestSanitizeDropsBidiAndZeroWidthControls(t *testing.T) {
	in := "a\u202eb\u200bc\ufeffd\u00ade\u2066f\u200dg\u200eh"
	if got := Sanitize(in); got != "abcdef\u200dgh" {
		t.Fatalf("Sanitize = %q", got)
	}
}

// A hunk line whose first rune is multi-byte must not be split on a byte:
// the sign is only ' ', '-' or '+', anything else is code with no sign.
func TestHunkSignIsARuneNotAByte(t *testing.T) {
	lines := Event(event.FileChange{ID: "t", At: at, Path: "/tmp/x.go", Kind: event.Update, Hunks: []event.Hunk{{Lines: []string{"é", "+é", "-x"}}}}, 80, time.UTC)
	for i, l := range lines {
		if !utf8.ValidString(l) {
			t.Errorf("row %d is not valid UTF-8: %q", i, l)
		}
	}
	if p := plain(lines); p[2] != "      ▕▏ é" || p[3] != "      ▕▏+é" {
		t.Errorf("rows = %q", p)
	}
}
