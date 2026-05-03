package render

import (
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// Code returns a syntax-highlighted excerpt of the file at path, centered on
// targetLine with `ctx` lines of surrounding context. The target line is
// marked with a ▶ in the gutter. If the file cannot be read, an error is
// returned; the caller is expected to render a placeholder.
func Code(path string, targetLine, ctx int) (string, error) {
	if targetLine < 1 {
		return "", fmt.Errorf("targetLine must be >= 1, got %d", targetLine)
	}
	if ctx < 0 {
		ctx = 0
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	lexer := lexers.Match(path)
	if lexer == nil {
		lexer = lexers.Analyse(string(raw))
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	style := styles.Get("monokai")
	if style == nil {
		style = styles.Fallback
	}
	formatter := formatters.Get("terminal256")
	if formatter == nil {
		formatter = formatters.Fallback
	}

	it, err := lexer.Tokenise(nil, string(raw))
	if err != nil {
		return "", err
	}
	tokens := it.Tokens()
	lines := chroma.SplitTokensIntoLines(tokens)

	start := targetLine - ctx
	if start < 1 {
		start = 1
	}
	end := targetLine + ctx
	if end > len(lines) {
		end = len(lines)
	}
	if start > len(lines) {
		return "", fmt.Errorf("targetLine %d beyond file length %d", targetLine, len(lines))
	}

	var out strings.Builder
	for i := start; i <= end; i++ {
		marker := "  "
		if i == targetLine {
			marker = "▶ "
		}
		fmt.Fprintf(&out, "%s%4d  ", marker, i)
		// Strip trailing newline from line tokens to avoid stray blank rows
		// before re-emitting our own \n.
		stripped := stripTrailingNewline(lines[i-1])
		if err := formatter.Format(&out, style, chroma.Literator(stripped...)); err != nil {
			return "", err
		}
		out.WriteByte('\n')
	}
	return out.String(), nil
}

func stripTrailingNewline(line []chroma.Token) []chroma.Token {
	if len(line) == 0 {
		return line
	}
	last := line[len(line)-1]
	if !strings.HasSuffix(last.Value, "\n") {
		return line
	}
	out := make([]chroma.Token, len(line))
	copy(out, line)
	out[len(out)-1].Value = strings.TrimRight(last.Value, "\n")
	if out[len(out)-1].Value == "" {
		out = out[:len(out)-1]
	}
	return out
}
