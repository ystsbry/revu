// Package diff parses unified diff output into a hunk model that the TUI can
// render. It handles only what revu needs (single-file diffs as produced by
// `git diff`) — no support for binary, rename, or mode-change pseudo-hunks
// beyond skipping their headers.
package diff

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// LineKind describes the role of a line within a hunk.
type LineKind byte

const (
	LineContext LineKind = ' ' // unchanged
	LineDelete  LineKind = '-' // present in pre-image only
	LineAdd     LineKind = '+' // present in post-image only
)

// Line is a single row inside a hunk. OldLine and NewLine carry the
// pre-image / post-image line numbers; either is 0 when the line does not
// exist on that side (e.g. an additions's OldLine).
type Line struct {
	Kind    LineKind
	OldLine int
	NewLine int
	Text    string // line content without the leading +/-/<space>
}

// Hunk corresponds to one `@@ -OldStart,OldCount +NewStart,NewCount @@` block.
type Hunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Header   string // text after the second "@@" (function name etc.)
	Lines    []Line
}

// Contains reports whether the hunk covers the given line on the given side.
//
// Side conventions match GitHub's review API: 'L' means the pre-image
// (deletion side), 'R' means the post-image (addition side).
func (h Hunk) Contains(line int, side byte) bool {
	switch side {
	case 'L':
		return line >= h.OldStart && line < h.OldStart+h.OldCount
	case 'R':
		return line >= h.NewStart && line < h.NewStart+h.NewCount
	default:
		return false
	}
}

// Parse extracts hunks from a unified diff. File-level headers
// (`diff --git`, `index`, `---`, `+++`) are ignored; only `@@` headers
// and their line bodies are kept. Returns an empty slice (not nil) when
// the input contains no hunks.
func Parse(raw []byte) ([]Hunk, error) {
	var hunks []Hunk
	var cur *Hunk
	var oldCursor, newCursor int

	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "@@") {
			h, err := parseHunkHeader(line)
			if err != nil {
				return nil, err
			}
			hunks = append(hunks, h)
			cur = &hunks[len(hunks)-1]
			oldCursor = h.OldStart
			newCursor = h.NewStart
			continue
		}
		if cur == nil {
			// Still in file headers; ignore.
			continue
		}
		if line == "" {
			// Blank lines between hunks (rare). Skip.
			continue
		}
		switch line[0] {
		case ' ':
			cur.Lines = append(cur.Lines, Line{
				Kind: LineContext, OldLine: oldCursor, NewLine: newCursor,
				Text: line[1:],
			})
			oldCursor++
			newCursor++
		case '-':
			cur.Lines = append(cur.Lines, Line{
				Kind: LineDelete, OldLine: oldCursor, NewLine: 0,
				Text: line[1:],
			})
			oldCursor++
		case '+':
			cur.Lines = append(cur.Lines, Line{
				Kind: LineAdd, OldLine: 0, NewLine: newCursor,
				Text: line[1:],
			})
			newCursor++
		case '\\':
			// "\ No newline at end of file" — informational, drop.
			continue
		default:
			// Unknown prefix means we left the hunk body (e.g. a new
			// "diff --git" header for the next file). Stop appending.
			cur = nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if hunks == nil {
		hunks = []Hunk{}
	}
	return hunks, nil
}

// parseHunkHeader handles "@@ -a,b +c,d @@ optional context".
// `b` and `d` default to 1 when omitted (single-line ranges).
func parseHunkHeader(header string) (Hunk, error) {
	rest := strings.TrimPrefix(header, "@@")
	end := strings.Index(rest, "@@")
	if end < 0 {
		return Hunk{}, fmt.Errorf("malformed hunk header: %q", header)
	}
	spec := strings.TrimSpace(rest[:end])
	tail := strings.TrimSpace(rest[end+2:])

	parts := strings.Fields(spec)
	if len(parts) != 2 {
		return Hunk{}, fmt.Errorf("malformed hunk spec: %q", spec)
	}
	oldStart, oldCount, err := parseRange(parts[0], '-')
	if err != nil {
		return Hunk{}, err
	}
	newStart, newCount, err := parseRange(parts[1], '+')
	if err != nil {
		return Hunk{}, err
	}
	return Hunk{
		OldStart: oldStart, OldCount: oldCount,
		NewStart: newStart, NewCount: newCount,
		Header: tail,
	}, nil
}

// parseRange parses "-12,5" or "+12" into (start, count).
func parseRange(s string, sign byte) (start, count int, err error) {
	if len(s) == 0 || s[0] != sign {
		return 0, 0, fmt.Errorf("range %q missing %c prefix", s, sign)
	}
	body := s[1:]
	count = 1
	if i := strings.IndexByte(body, ','); i >= 0 {
		count, err = strconv.Atoi(body[i+1:])
		if err != nil {
			return 0, 0, fmt.Errorf("bad count in %q: %w", s, err)
		}
		body = body[:i]
	}
	start, err = strconv.Atoi(body)
	if err != nil {
		return 0, 0, fmt.Errorf("bad start in %q: %w", s, err)
	}
	return start, count, nil
}

// FindHunkForRange picks the hunk that best covers a comment range. For
// same-side comments (startSide == endSide) it returns the hunk containing
// the start line on that side. For cross-side ranges it prefers a hunk
// that contains both endpoints; when no single hunk does, it falls back to
// the hunk holding the LEFT (pre-image) endpoint.
func FindHunkForRange(hunks []Hunk, startLine int, startSide byte, endLine int, endSide byte) *Hunk {
	for i := range hunks {
		h := &hunks[i]
		if h.Contains(startLine, startSide) && h.Contains(endLine, endSide) {
			return h
		}
	}
	leftLine, rightLine := endLine, endLine
	if startSide == 'L' {
		leftLine = startLine
	} else if endSide != 'L' {
		// neither endpoint is on the LEFT side
		leftLine = -1
	}
	if endSide == 'R' {
		rightLine = endLine
	} else if startSide == 'R' {
		rightLine = startLine
	}
	for i := range hunks {
		h := &hunks[i]
		if leftLine > 0 && h.Contains(leftLine, 'L') {
			return h
		}
		if rightLine > 0 && h.Contains(rightLine, 'R') {
			return h
		}
	}
	return nil
}
