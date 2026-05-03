package render

import (
	"strings"
	"testing"

	"github.com/ystsbry/revu/internal/diff"
)

func makeHunk() diff.Hunk {
	return diff.Hunk{
		OldStart: 1, OldCount: 5,
		NewStart: 1, NewCount: 6,
		Header: "func Bar()",
		Lines: []diff.Line{
			{Kind: diff.LineContext, OldLine: 1, NewLine: 1, Text: "package foo"},
			{Kind: diff.LineContext, OldLine: 2, NewLine: 2, Text: ""},
			{Kind: diff.LineDelete, OldLine: 3, Text: "func Old() {}"},
			{Kind: diff.LineAdd, NewLine: 3, Text: "func New() {}"},
			{Kind: diff.LineAdd, NewLine: 4, Text: "var Added int"},
			{Kind: diff.LineContext, OldLine: 4, NewLine: 5, Text: ""},
			{Kind: diff.LineContext, OldLine: 5, NewLine: 6, Text: "// tail"},
		},
	}
}

func TestDiffHunkContainsLines(t *testing.T) {
	t.Parallel()
	out := DiffHunk(makeHunk(), DiffHunkOptions{})
	for _, want := range []string{
		"@@ -1,5 +1,6 @@",
		"func Bar()",
		"package foo",
		"- func Old()",
		"+ func New()",
		"+ var Added int",
		"// tail",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestDiffHunkAnchorMarker(t *testing.T) {
	t.Parallel()
	// Anchor on the LEFT-side line 3 (the deletion). The marker glyph must
	// appear and only on that line.
	out := DiffHunk(makeHunk(), DiffHunkOptions{AnchorOldLine: 3})
	if !strings.Contains(out, "▶") {
		t.Fatalf("expected anchor marker:\n%s", out)
	}
	// Count ▶ occurrences — should be exactly one.
	if got := strings.Count(out, "▶"); got != 1 {
		t.Errorf("anchor marker count = %d, want 1:\n%s", got, out)
	}
}

func TestDiffHunkBothAnchors(t *testing.T) {
	t.Parallel()
	// Cross-side anchor: LEFT 3 + RIGHT 4 should mark two distinct lines.
	out := DiffHunk(makeHunk(), DiffHunkOptions{AnchorOldLine: 3, AnchorNewLine: 4})
	if got := strings.Count(out, "▶"); got != 2 {
		t.Errorf("expected 2 anchors, got %d:\n%s", got, out)
	}
}
