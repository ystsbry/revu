package diff

import (
	"strings"
	"testing"
)

// sampleDiff intentionally uses string concatenation (not a raw literal) so
// the blank context lines carry a leading space character — git always
// prefixes context lines with " ", and editors silently strip trailing
// whitespace from raw strings.
const sampleDiff = "diff --git a/foo.go b/foo.go\n" +
	"index 0000001..0000002 100644\n" +
	"--- a/foo.go\n" +
	"+++ b/foo.go\n" +
	"@@ -1,5 +1,6 @@ func Bar()\n" +
	" package foo\n" +
	" \n" +
	"-func Old() {}\n" +
	"+func New() {}\n" +
	"+var Added int\n" +
	" \n" +
	" // tail\n" +
	"@@ -10,3 +11,3 @@ trailer\n" +
	" a\n" +
	"-b\n" +
	"+B\n" +
	" c\n"

func TestParseExtractsHunks(t *testing.T) {
	t.Parallel()
	hunks, err := Parse([]byte(sampleDiff))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(hunks) != 2 {
		t.Fatalf("want 2 hunks, got %d", len(hunks))
	}

	h0 := hunks[0]
	if h0.OldStart != 1 || h0.OldCount != 5 || h0.NewStart != 1 || h0.NewCount != 6 {
		t.Errorf("h0 spec wrong: %+v", h0)
	}
	if h0.Header != "func Bar()" {
		t.Errorf("h0 header = %q", h0.Header)
	}

	// Verify a deletion + an addition show up with the right side numbers.
	var sawDel, sawAdd bool
	for _, l := range h0.Lines {
		if l.Kind == LineDelete && strings.Contains(l.Text, "Old()") {
			if l.OldLine != 3 || l.NewLine != 0 {
				t.Errorf("Old() line numbers = (old=%d new=%d), want (3,0)", l.OldLine, l.NewLine)
			}
			sawDel = true
		}
		if l.Kind == LineAdd && strings.Contains(l.Text, "var Added") {
			// After "func New()" was added on new line 3, Added comes on new line 4.
			if l.NewLine != 4 || l.OldLine != 0 {
				t.Errorf("Added line numbers = (old=%d new=%d), want (0,4)", l.OldLine, l.NewLine)
			}
			sawAdd = true
		}
	}
	if !sawDel || !sawAdd {
		t.Errorf("expected both delete and add lines: del=%v add=%v", sawDel, sawAdd)
	}
}

func TestParseEmpty(t *testing.T) {
	t.Parallel()
	hunks, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if hunks == nil || len(hunks) != 0 {
		t.Errorf("want empty non-nil slice, got %#v", hunks)
	}
}

func TestParseDefaultCount(t *testing.T) {
	t.Parallel()
	// "@@ -5 +6 @@" (no count) should default to count=1.
	hunks, err := Parse([]byte("@@ -5 +6 @@\n+x\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(hunks) != 1 || hunks[0].OldCount != 1 || hunks[0].NewCount != 1 {
		t.Errorf("count defaults wrong: %+v", hunks)
	}
}

func TestHunkContains(t *testing.T) {
	t.Parallel()
	h := Hunk{OldStart: 5, OldCount: 3, NewStart: 10, NewCount: 4}
	if !h.Contains(5, 'L') || !h.Contains(7, 'L') || h.Contains(8, 'L') {
		t.Errorf("LEFT containment wrong")
	}
	if !h.Contains(10, 'R') || !h.Contains(13, 'R') || h.Contains(14, 'R') {
		t.Errorf("RIGHT containment wrong")
	}
}

func TestFindHunkForRange(t *testing.T) {
	t.Parallel()
	hunks, err := Parse([]byte(sampleDiff))
	if err != nil {
		t.Fatal(err)
	}
	// Cross-side: LEFT 3 -> RIGHT 4 should land in hunk 0.
	h := FindHunkForRange(hunks, 3, 'L', 4, 'R')
	if h == nil || h.OldStart != 1 {
		t.Errorf("cross-side LEFT 3 -> RIGHT 4 missed hunk 0: %+v", h)
	}
	// Same-side RIGHT 11..12 should land in hunk 1.
	h = FindHunkForRange(hunks, 11, 'R', 12, 'R')
	if h == nil || h.OldStart != 10 {
		t.Errorf("same-side RIGHT in hunk 1 missed: %+v", h)
	}
	// Out of range.
	h = FindHunkForRange(hunks, 100, 'L', 100, 'R')
	if h != nil {
		t.Errorf("expected nil for out-of-range")
	}
}
