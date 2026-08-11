package dashboard

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

// seedReviewedPR creates ~/.revu/{owner}/{repo}/pr-{n}/{sha}/review.yml,
// which is what store counts as "this PR has been reviewed".
func seedReviewedPR(t *testing.T, home, slug string, pr int, sha string) string {
	t.Helper()
	owner, repo, ok := strings.Cut(slug, "/")
	if !ok {
		t.Fatalf("slug %q is not owner/repo", slug)
	}
	dir := filepath.Join(home, owner, repo, "pr-"+strconv.Itoa(pr), sha)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "review.yml"), []byte("schema_version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// writeConfig points $REVU_CONFIG at a throwaway config.toml.
func writeConfig(t *testing.T, body string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REVU_CONFIG", path)
}

func TestReviewedCount(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REVU_HOME", home)

	if got := reviewedCount("o/r"); got != 0 {
		t.Errorf("no reviews should count 0, got %d", got)
	}

	seedReviewedPR(t, home, "o/r", 1, "abc1234")
	seedReviewedPR(t, home, "o/r", 2, "def5678")
	if got := reviewedCount("o/r"); got != 2 {
		t.Errorf("reviewedCount = %d, want 2", got)
	}

	// Several SHAs under one PR still count as a single reviewed PR.
	seedReviewedPR(t, home, "o/r", 1, "999aaaa")
	if got := reviewedCount("o/r"); got != 2 {
		t.Errorf("a second SHA on the same PR should not add a row, got %d", got)
	}

	// Best-effort: a malformed slug reports zero rather than blowing up
	// the sidebar.
	if got := reviewedCount("not-a-slug"); got != 0 {
		t.Errorf("malformed slug should count 0, got %d", got)
	}
}

func TestLoadReposFromConfigListsRegisteredRepos(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REVU_HOME", home)
	clone := t.TempDir()
	gone := filepath.Join(t.TempDir(), "removed")
	writeConfig(t, ""+
		"[[repo]]\nslug = \"o/present\"\npath = \""+clone+"\"\nsearch = \"is:open author:@me\"\n"+
		"[[repo]]\nslug = \"o/gone\"\npath = \""+gone+"\"\n")
	seedReviewedPR(t, home, "o/present", 3, "abc1234")

	data, err := loadReposFromConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Items) != 2 {
		t.Fatalf("items = %d, want 2: %+v", len(data.Items), data.Items)
	}
	if len(data.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", data.Warnings)
	}
	if data.Profile != "" {
		t.Errorf("profile = %q, want empty for the default profile", data.Profile)
	}

	present := data.Items[0]
	if !present.Registered || present.PathMissing {
		t.Errorf("present repo = %+v, want registered and on disk", present)
	}
	if present.Search != "is:open author:@me" {
		t.Errorf("per-repo search = %q, want the configured override", present.Search)
	}
	if present.ReviewedCount != 1 {
		t.Errorf("ReviewedCount = %d, want 1", present.ReviewedCount)
	}

	// A registered clone that is no longer on disk must be flagged rather
	// than dropped — the user needs to see why it stopped working.
	if missing := data.Items[1]; !missing.PathMissing || !missing.Registered {
		t.Errorf("removed clone = %+v, want registered with PathMissing", missing)
	}
}

// With no [[repo]] entries the sidebar falls back to whatever has reviews
// under ~/.revu, so a fresh install still shows something.
func TestLoadReposFromConfigFallsBackToReviewedRepos(t *testing.T) {
	home := t.TempDir()
	t.Setenv("REVU_HOME", home)
	writeConfig(t, "")
	seedReviewedPR(t, home, "o/stored", 1, "abc1234")
	seedReviewedPR(t, home, "o/stored", 2, "def5678")

	data, err := loadReposFromConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Items) != 1 {
		t.Fatalf("items = %d, want 1: %+v", len(data.Items), data.Items)
	}
	it := data.Items[0]
	if it.Slug != "o/stored" {
		t.Errorf("slug = %q, want o/stored", it.Slug)
	}
	if it.Registered {
		t.Error("a store-only row must not claim to be registered")
	}
	if it.ReviewedCount != 2 {
		t.Errorf("ReviewedCount = %d, want 2", it.ReviewedCount)
	}
}

func TestLoadReposFromConfigProfileWarnings(t *testing.T) {
	t.Run("active profile is not declared", func(t *testing.T) {
		t.Setenv("REVU_HOME", t.TempDir())
		clone := t.TempDir()
		writeConfig(t, ""+
			"active_profile = \"ghost\"\n"+
			"[[repo]]\nslug = \"o/r\"\npath = \""+clone+"\"\n")

		data, err := loadReposFromConfig()
		if err != nil {
			t.Fatal(err)
		}
		if len(data.Warnings) != 1 || !strings.Contains(data.Warnings[0], "ghost") {
			t.Fatalf("warnings = %v, want one naming the undeclared profile", data.Warnings)
		}
		if data.Profile != "" {
			t.Errorf("profile = %q, want empty when it is undeclared", data.Profile)
		}
		if len(data.Items) != 1 {
			t.Errorf("an undeclared profile should still show every repo, got %d", len(data.Items))
		}
	})

	t.Run("profile references an unregistered repo", func(t *testing.T) {
		t.Setenv("REVU_HOME", t.TempDir())
		clone := t.TempDir()
		writeConfig(t, ""+
			"active_profile = \"work\"\n"+
			"[[repo]]\nslug = \"o/known\"\npath = \""+clone+"\"\n"+
			"[[profile]]\nname = \"work\"\nrepos = [\"o/known\", \"o/unknown\"]\n")

		data, err := loadReposFromConfig()
		if err != nil {
			t.Fatal(err)
		}
		if data.Profile != "work" {
			t.Errorf("profile = %q, want work", data.Profile)
		}
		if len(data.Warnings) != 1 || !strings.Contains(data.Warnings[0], "o/unknown") {
			t.Fatalf("warnings = %v, want one naming the unregistered slug", data.Warnings)
		}
		if len(data.Items) != 1 || data.Items[0].Slug != "o/known" {
			t.Errorf("items = %+v, want just o/known", data.Items)
		}
	})

	t.Run("the default profile is not surfaced", func(t *testing.T) {
		t.Setenv("REVU_HOME", t.TempDir())
		clone := t.TempDir()
		writeConfig(t, ""+
			"active_profile = \"default\"\n"+
			"[[repo]]\nslug = \"o/r\"\npath = \""+clone+"\"\n")

		data, err := loadReposFromConfig()
		if err != nil {
			t.Fatal(err)
		}
		if data.Profile != "" || len(data.Warnings) != 0 {
			t.Errorf("default profile should be invisible, got profile=%q warnings=%v",
				data.Profile, data.Warnings)
		}
	})
}

func TestTruncate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{name: "no size yet returns the text whole", s: "abcdefghijklm", n: 0, want: "abcdefghijklm"},
		{name: "negative size returns the text whole", s: "abcdefghijklm", n: -5, want: "abcdefghijklm"},
		{name: "shorter than the limit is untouched", s: "abc", n: 10, want: "abc"},
		{name: "exactly the limit is untouched", s: "abcdefghij", n: 10, want: "abcdefghij"},
		{name: "over the limit gets an ellipsis", s: "abcdefghijk", n: 10, want: "abcdefghi…"},
		// A limit under 10 is raised to 10 rather than producing a stub.
		{name: "tiny limits are floored at 10", s: "abcdefghijklm", n: 3, want: "abcdefghi…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := truncate(tt.s, tt.n); got != tt.want {
				t.Fatalf("truncate(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
			}
		})
	}
}

// truncateCells counts a fullwidth rune as two cells, so a shortened title
// never wraps inside a fixed-width card.
func TestTruncateCells(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		s         string
		n         int
		wantWidth int  // max display width the result may occupy
		wantWhole bool // true when the input must come back untouched
	}{
		{name: "no size yet returns the text whole", s: "日本語のタイトル", n: 0, wantWhole: true},
		{name: "ascii under the limit", s: "short", n: 20, wantWhole: true},
		{name: "ascii over the limit", s: strings.Repeat("a", 40), n: 20, wantWidth: 20},
		{name: "fullwidth under the limit", s: "日本語", n: 20, wantWhole: true},
		{name: "fullwidth over the limit", s: strings.Repeat("日", 20), n: 20, wantWidth: 20},
		{name: "mixed width over the limit", s: "abc" + strings.Repeat("日", 20), n: 15, wantWidth: 15},
		{name: "tiny limits are floored at 10", s: strings.Repeat("日", 20), n: 2, wantWidth: 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := truncateCells(tt.s, tt.n)
			if tt.wantWhole {
				if got != tt.s {
					t.Fatalf("truncateCells(%q, %d) = %q, want it unchanged", tt.s, tt.n, got)
				}
				return
			}
			if got == tt.s {
				t.Fatalf("truncateCells(%q, %d) did not shorten anything", tt.s, tt.n)
			}
			if w := runewidth.StringWidth(got); w > tt.wantWidth {
				t.Fatalf("truncateCells(%q, %d) = %q (width %d), want at most %d cells",
					tt.s, tt.n, got, w, tt.wantWidth)
			}
			if !strings.HasSuffix(got, "…") {
				t.Fatalf("truncateCells(%q, %d) = %q, want a trailing ellipsis", tt.s, tt.n, got)
			}
		})
	}
}
