package render

import (
	"strings"
	"testing"
)

func TestHighlightGoLineKeepsTextAndColoursTokens(t *testing.T) {
	lx := lexerFor("/tmp/main.go")
	if lx == nil {
		t.Fatal("Go must have a lexer")
	}
	got := highlightLines(lx, []string{`func main() { s := "a" // hi`, `}`})
	if len(got) != 2 {
		t.Fatalf("line count changed: %d", len(got))
	}
	if plainStr(got[0]) != `func main() { s := "a" // hi` || plainStr(got[1]) != `}` {
		t.Fatalf("text altered: %q %q", plainStr(got[0]), plainStr(got[1]))
	}
	for _, want := range []string{"\x1b[35mfunc", "\x1b[32m\"a\"", "\x1b[2;90m// hi", "\x1b[34mmain"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("missing %q in %q", want, got[0])
		}
	}
	if strings.Contains(got[0], "\x1b[30m") || strings.Contains(got[0], "38;5") || strings.Contains(got[0], "38;2") {
		t.Fatalf("only the 16-colour palette, never black or extended colours: %q", got[0])
	}
}

func TestHighlightMultiLineTokenDoesNotBleed(t *testing.T) {
	got := highlightLines(lexerFor("/tmp/a.go"), []string{"/* one", "two */", "x := 1"})
	for i, l := range got {
		if strings.Count(l, "\x1b[") > 0 && !strings.HasSuffix(l, "\x1b[m") { // lipgloss resets with ESC[m
			t.Errorf("line %d does not end with a reset: %q", i, l)
		}
	}
	if !strings.Contains(got[2], "\x1b[36m1") {
		t.Fatalf("number must be cyan, not the string colour: %q", got[2])
	}
	if !strings.Contains(got[1], "\x1b[2;90mtwo */") {
		t.Fatalf("comment continuation must be re-opened on its own line: %q", got[1])
	}
}

func TestLexerForUnknownAndBash(t *testing.T) {
	if lexerFor("/tmp/config") != nil || lexerFor("/tmp/x.unknownext") != nil {
		t.Fatal("unknown files must have no lexer, so they render plain")
	}
	if lexerFor("/tmp/Makefile") == nil || lexerFor("/tmp/x.yaml") == nil || lexerFor("/tmp/t.json") == nil {
		t.Fatal("Makefile, yaml and json must match")
	}
	out := highlightLines(bashLexer, []string{`go test ./... 2>&1 | grep -E "^ok" || echo none`})
	if plainStr(out[0]) != `go test ./... 2>&1 | grep -E "^ok" || echo none` {
		t.Fatalf("text altered: %q", plainStr(out[0]))
	}
	if !strings.Contains(out[0], "\x1b[32m\"^ok\"") {
		t.Fatalf("the string literal must be green: %q", out[0])
	}
}

func TestHighlightNilLexerIsIdentity(t *testing.T) {
	in := []string{"plain\ttext", "more"}
	out := highlightLines(nil, in)
	if strings.Join(out, "|") != strings.Join(in, "|") {
		t.Fatalf("got %q", out)
	}
}

func plainStr(s string) string { return sgr.ReplaceAllString(s, "") }
