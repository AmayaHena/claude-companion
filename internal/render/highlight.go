package render

import (
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// Syntax colouring uses chroma's lexers for tokenisation and a fixed map
// from token category to the terminal palette. chroma's own terminal
// formatters are not used: with common styles they emit black (ESC[30m) for
// punctuation, which is invisible on a dark theme.
var (
	tokKeyword = lipgloss.NewStyle().Foreground(lipgloss.Magenta)
	tokString  = lipgloss.NewStyle().Foreground(lipgloss.Green)
	tokNumber  = lipgloss.NewStyle().Foreground(lipgloss.Cyan)
	tokComment = lipgloss.NewStyle().Foreground(lipgloss.BrightBlack).Faint(true)
	tokName    = lipgloss.NewStyle().Foreground(lipgloss.Blue) // functions, builtins, tags
)

var bashLexer = chroma.Coalesce(lexers.Get("bash"))

// lexerFor picks a lexer from the file name; nil when chroma knows none,
// in which case the caller renders plain text.
func lexerFor(path string) chroma.Lexer {
	lx := lexers.Match(filepath.Base(path))
	if lx == nil {
		return nil
	}
	return chroma.Coalesce(lx)
}

// tokenStyle maps a chroma token type to a palette style; nil means default.
func tokenStyle(t chroma.TokenType) *lipgloss.Style {
	switch {
	case t.InCategory(chroma.Keyword):
		return &tokKeyword
	case t.InSubCategory(chroma.LiteralString):
		return &tokString
	case t.InSubCategory(chroma.LiteralNumber):
		return &tokNumber
	case t.InCategory(chroma.Comment):
		return &tokComment
	case t == chroma.NameFunction, t == chroma.NameBuiltin, t == chroma.NameTag, t == chroma.NameClass, t == chroma.NameAttribute:
		return &tokName
	}
	return nil
}

// highlightLines tokenises the lines as one text (so multi-line constructs
// are recognised) and returns one styled string per input line. Every
// styled run is closed on its own line, so nothing bleeds across rows. A nil
// lexer or a tokeniser error returns the input unchanged.
func highlightLines(lx chroma.Lexer, lines []string) []string {
	if lx == nil || len(lines) == 0 {
		return lines
	}
	it, err := lx.Tokenise(nil, strings.Join(lines, "\n"))
	if err != nil {
		return lines
	}
	out := make([]string, 0, len(lines))
	var cur strings.Builder
	for tok := it(); tok != chroma.EOF; tok = it() {
		st := tokenStyle(tok.Type)
		parts := strings.Split(tok.Value, "\n")
		for i, part := range parts {
			if i > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
			if part == "" {
				continue
			}
			if st == nil {
				cur.WriteString(part)
			} else {
				cur.WriteString(st.Render(part))
			}
		}
	}
	out = append(out, cur.String())
	// chroma always appends a trailing newline token; drop the empty last
	// line it produces and pad if a lexer swallowed one.
	if len(out) > len(lines) {
		out = out[:len(lines)]
	}
	for len(out) < len(lines) {
		out = append(out, "")
	}
	return out
}
