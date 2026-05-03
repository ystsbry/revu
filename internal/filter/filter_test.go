package filter

import (
	"strings"
	"testing"

	"github.com/ystsbry/revu/internal/model"
)

func sample() []model.Comment {
	return []model.Comment{
		{ID: "c1", Status: model.StatusPending, Severity: model.SeverityMajor, Category: model.CategoryDesign, Path: "src/order/application.py"},
		{ID: "c2", Status: model.StatusPending, Severity: model.SeverityNit, Category: model.CategoryStyle, Path: "src/order/domain.py"},
		{ID: "c3", Status: model.StatusAccepted, Severity: model.SeverityMinor, Category: model.CategoryPerf, Path: "src/order/repository.py"},
		{ID: "c4", Status: model.StatusRejected, Severity: model.SeverityMajor, Category: model.CategoryBug, Path: "src/api/handler.py"},
		{ID: "c5", Status: model.StatusPending, Severity: model.SeverityCritical, Category: model.CategorySecurity, Path: "src/auth/login.py"},
	}
}

func TestParseEmpty(t *testing.T) {
	t.Parallel()
	f, err := Parse("")
	if err != nil {
		t.Fatal(err)
	}
	if !f.IsEmpty() {
		t.Errorf("empty expr should be empty filter")
	}
	for i := range sample() {
		c := sample()[i]
		if !f.Match(&c) {
			t.Errorf("empty filter should match all; %s did not", c.ID)
		}
	}
}

func TestSeverityFilter(t *testing.T) {
	t.Parallel()
	f, err := Parse("severity:major,critical")
	if err != nil {
		t.Fatal(err)
	}
	got := visibleIDs(f, sample())
	want := []string{"c1", "c4", "c5"}
	if !equalSlice(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCategoryFilter(t *testing.T) {
	t.Parallel()
	f, _ := Parse("category:bug,security")
	got := visibleIDs(f, sample())
	want := []string{"c4", "c5"}
	if !equalSlice(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestStatusFilter(t *testing.T) {
	t.Parallel()
	f, _ := Parse("status:pending")
	got := visibleIDs(f, sample())
	want := []string{"c1", "c2", "c5"}
	if !equalSlice(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestPathFilter(t *testing.T) {
	t.Parallel()
	f, _ := Parse("path:order")
	got := visibleIDs(f, sample())
	want := []string{"c1", "c2", "c3"}
	if !equalSlice(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// Multi-value: OR within path
	f, _ = Parse("path:order,api")
	got = visibleIDs(f, sample())
	want = []string{"c1", "c2", "c3", "c4"}
	if !equalSlice(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// Case insensitive
	f, _ = Parse("path:APPLICATION")
	got = visibleIDs(f, sample())
	if len(got) != 1 || got[0] != "c1" {
		t.Errorf("case insensitive failed: %v", got)
	}
}

func TestANDOfDimensions(t *testing.T) {
	t.Parallel()
	f, _ := Parse("severity:major status:pending")
	got := visibleIDs(f, sample())
	want := []string{"c1"} // c4 is major but rejected
	if !equalSlice(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in       string
		errSubst string
	}{
		{"severity", "expected key:value"},
		{"severity:", "expected key:value"},
		{"severity:zzz", "invalid severity"},
		{"category:ux", "invalid category"},
		{"status:done", "invalid status"},
		{"unknown:foo", "unknown filter key"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			_, err := Parse(tc.in)
			if err == nil {
				t.Fatalf("expected error for %q", tc.in)
			}
			if !strings.Contains(err.Error(), tc.errSubst) {
				t.Errorf("err = %v, want substring %q", err, tc.errSubst)
			}
		})
	}
}

func TestVisibleIndices(t *testing.T) {
	t.Parallel()
	cs := sample()
	f, _ := Parse("severity:major")
	got := f.VisibleIndices(cs)
	want := []int{0, 3} // c1, c4
	if !equalIntSlice(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// Empty filter returns all indices in order.
	f, _ = Parse("")
	got = f.VisibleIndices(cs)
	if len(got) != len(cs) {
		t.Errorf("empty filter should return all indices, got %v", got)
	}
}

// helpers

func visibleIDs(f Filter, cs []model.Comment) []string {
	out := []string{}
	for i := range cs {
		if f.Match(&cs[i]) {
			out = append(out, cs[i].ID)
		}
	}
	return out
}

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalIntSlice(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
