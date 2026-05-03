package store

import (
	"os"
	"path/filepath"
	"testing"
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

func TestLatestPRDir(t *testing.T) {
	dir := t.TempDir()
	mkdir := func(name string) {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mkdir("pr-1")
	mkdir("pr-7")
	mkdir("pr-42")
	mkdir("pr-not-a-number")
	mkdir("notes")

	got, err := LatestPRDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "pr-42")
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
