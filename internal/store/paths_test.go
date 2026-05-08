package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseRemoteURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"git@github.com:ystsbry/revu.git", "ystsbry/revu", false},
		{"git@github.com:ystsbry/revu", "ystsbry/revu", false},
		{"https://github.com/ystsbry/revu.git", "ystsbry/revu", false},
		{"https://github.com/ystsbry/revu", "ystsbry/revu", false},
		{"https://github.com/ystsbry/revu/", "ystsbry/revu", false},
		{"ssh://git@github.com/ystsbry/revu.git", "ystsbry/revu", false},
		{"https://gitlab.com/group/project.git", "group/project", false},
		{"https://github.com/owner-with-dash/repo_with_underscore.git", "owner-with-dash/repo_with_underscore", false},
		{"  https://github.com/ystsbry/revu.git\n", "ystsbry/revu", false},
		{"", "", true},
		{"not a url", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseRemoteURL(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseRemoteURL(%q) err=%v wantErr=%v", tc.in, err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Fatalf("ParseRemoteURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestHomeRespectsEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REVU_HOME", dir)
	got, err := Home()
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("Home()=%q want %q", got, dir)
	}
}

func TestRepoDirAndPRDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("REVU_HOME", dir)

	got, err := RepoDir("ystsbry/revu")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "ystsbry", "revu")
	if got != want {
		t.Fatalf("RepoDir = %q want %q", got, want)
	}

	gotPR, err := PRDir("ystsbry/revu", 42)
	if err != nil {
		t.Fatal(err)
	}
	wantPR := filepath.Join(want, "pr-42")
	if gotPR != wantPR {
		t.Fatalf("PRDir = %q want %q", gotPR, wantPR)
	}

	if _, err := PRDir("ystsbry/revu", 0); err == nil {
		t.Fatalf("PRDir(0) should error")
	}
	if _, err := RepoDir("invalid-slug"); err == nil {
		t.Fatalf("RepoDir without slash should error")
	}
}

func TestShortSHA(t *testing.T) {
	t.Parallel()
	got, err := ShortSHA("abcdef0123456789")
	if err != nil {
		t.Fatal(err)
	}
	if got != "abcdef0" {
		t.Fatalf("ShortSHA = %q, want %q", got, "abcdef0")
	}
	if _, err := ShortSHA("abc"); err == nil {
		t.Fatalf("ShortSHA on short input should error")
	}
	if _, err := ShortSHA(""); err == nil {
		t.Fatalf("ShortSHA on empty input should error")
	}
}

func TestReviewDir(t *testing.T) {
	root := t.TempDir()
	t.Setenv("REVU_HOME", root)
	got, err := ReviewDir("ystsbry/revu", 42, "abcdef0123456789")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "ystsbry", "revu", "pr-42", "abcdef0")
	if got != want {
		t.Fatalf("ReviewDir = %q, want %q", got, want)
	}

	if _, err := ReviewDir("ystsbry/revu", 0, "abcdef0123456789"); err == nil {
		t.Fatalf("ReviewDir with pr=0 should error")
	}
	if _, err := ReviewDir("ystsbry/revu", 42, "abc"); err == nil {
		t.Fatalf("ReviewDir with short sha should error")
	}
}

// mkReviewed writes review.yml at repoDir/pr-N/sha/review.yml. modTime, when
// non-zero, is applied to the review.yml so tests can assert on which SHA
// dir wins under multiple-SHA scenarios.
func mkReviewed(t *testing.T, repoDir string, pr int, sha string, modTime time.Time) string {
	t.Helper()
	dir := filepath.Join(repoDir, fmt.Sprintf("pr-%d", pr), sha)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "review.yml")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if !modTime.IsZero() {
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLatestPRDir(t *testing.T) {
	dir := t.TempDir()
	mkReviewed(t, dir, 1, "aaaaaaa", time.Time{})
	mkReviewed(t, dir, 7, "bbbbbbb", time.Time{})
	mkReviewed(t, dir, 42, "ccccccc", time.Time{})

	// pr-99 exists but has no SHA-with-review subdir; must be skipped.
	if err := os.MkdirAll(filepath.Join(dir, "pr-99", "ddddddd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "pr-not-a-number"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Legacy pr-N/review.yml (no SHA dir) must NOT be discovered.
	if err := os.MkdirAll(filepath.Join(dir, "pr-200"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pr-200", "review.yml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := LatestPRDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "pr-42", "ccccccc")
	if got != want {
		t.Fatalf("LatestPRDir = %q want %q", got, want)
	}
}

func TestLatestPRDirEmpty(t *testing.T) {
	dir := t.TempDir()
	if _, err := LatestPRDir(dir); err == nil {
		t.Fatalf("expected error for empty dir")
	}
}

func TestLatestPRDirSkipsUnreviewed(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pr-8"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := LatestPRDir(dir); err == nil {
		t.Fatalf("expected error when only unreviewed pr-* dirs exist")
	}
}

func TestListReviewedPRDirs(t *testing.T) {
	dir := t.TempDir()
	mkReviewed(t, dir, 3, "aaaaaaa", time.Time{})
	mkReviewed(t, dir, 12, "bbbbbbb", time.Time{})
	mkReviewed(t, dir, 7, "ccccccc", time.Time{})

	// pr-99 with no review.yml under any subdir → skipped.
	if err := os.MkdirAll(filepath.Join(dir, "pr-99", "ddddddd"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := ListReviewedPRDirs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("len=%d want 3 (got %v)", len(got), got)
	}
	wantNumbers := []int{12, 7, 3}
	wantSHAs := []string{"bbbbbbb", "ccccccc", "aaaaaaa"}
	for i, n := range wantNumbers {
		if got[i].Number != n {
			t.Fatalf("got[%d].Number=%d want %d", i, got[i].Number, n)
		}
		if got[i].ShortSHA != wantSHAs[i] {
			t.Fatalf("got[%d].ShortSHA=%q want %q", i, got[i].ShortSHA, wantSHAs[i])
		}
		want := filepath.Join(dir, fmt.Sprintf("pr-%d", n), wantSHAs[i])
		if got[i].Path != want {
			t.Fatalf("got[%d].Path=%q want %q", i, got[i].Path, want)
		}
	}
}

func TestListReviewedPRDirsPicksLatestSHA(t *testing.T) {
	dir := t.TempDir()
	older := time.Now().Add(-2 * time.Hour)
	newer := time.Now().Add(-5 * time.Minute)
	mkReviewed(t, dir, 10, "0000111", older)
	mkReviewed(t, dir, 10, "2222333", newer)

	got, err := ListReviewedPRDirs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d want 1 (multiple SHAs should collapse to the newest)", len(got))
	}
	if got[0].ShortSHA != "2222333" {
		t.Fatalf("ShortSHA=%q want %q (newer ModTime)", got[0].ShortSHA, "2222333")
	}
	if !strings.HasSuffix(got[0].Path, filepath.Join("pr-10", "2222333")) {
		t.Fatalf("Path=%q does not end with pr-10/2222333", got[0].Path)
	}
}

func TestListReviewedPRDirsIgnoresLegacyLayout(t *testing.T) {
	dir := t.TempDir()
	// Pre-SHA-layout: pr-5/review.yml directly. Must be ignored.
	if err := os.MkdirAll(filepath.Join(dir, "pr-5"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pr-5", "review.yml"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ListReviewedPRDirs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("legacy pr-N/review.yml should be ignored, got %v", got)
	}
}

func TestResolveReviewDirExplicitPath(t *testing.T) {
	dir := t.TempDir()
	got, err := ResolveReviewDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs(dir)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	if _, err := ResolveReviewDir(filepath.Join(dir, "does-not-exist")); err == nil {
		t.Fatalf("non-existent path should error")
	}

	// File (not dir) should error.
	f := filepath.Join(dir, "file")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveReviewDir(f); err == nil {
		t.Fatalf("file path should error")
	}
}
