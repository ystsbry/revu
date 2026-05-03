package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodeBasic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := `package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := Code(path, 6, 2)
	if err != nil {
		t.Fatalf("Code: %v", err)
	}
	if !strings.Contains(out, "▶") {
		t.Errorf("expected target marker ▶ in output:\n%s", out)
	}
	// Lines 4-7 should be visible (target=6, ctx=2 → [4..7] but file has 7 lines incl. blank).
	for _, want := range []string{"   4  ", "   5  ", "▶    6  ", "   7  "} {
		if !strings.Contains(out, want) {
			t.Errorf("expected gutter %q in output:\n%s", want, out)
		}
	}
	// Lines outside the window must NOT appear.
	if strings.Contains(out, "   1  ") {
		t.Errorf("line 1 should not appear in window:\n%s", out)
	}
}

func TestCodeMissingFile(t *testing.T) {
	t.Parallel()
	_, err := Code("/does/not/exist", 1, 2)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCodeBadLine(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(path, []byte("a\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Code(path, 0, 1); err == nil {
		t.Errorf("expected error for line 0")
	}
	if _, err := Code(path, 999, 1); err == nil {
		t.Errorf("expected error for line beyond EOF")
	}
}

func TestCodeContextClamped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(path, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := Code(path, 1, 5) // ctx of 5 but only 3 lines exist.
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"   1  ", "   2  ", "   3  "} {
		if !strings.Contains(out, want) {
			t.Errorf("expected line %q:\n%s", want, out)
		}
	}
}

func TestCodeRangeMarkers(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(path, []byte("a\nb\nc\nd\ne\nf\ng\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Range 3..6 with no context: lines 3,4,5,6 should carry ┌ │ │ └.
	out, err := CodeRange(path, 3, 6, 0)
	if err != nil {
		t.Fatalf("CodeRange: %v", err)
	}
	for _, want := range []string{"┌    3  ", "│    4  ", "│    5  ", "└    6  "} {
		if !strings.Contains(out, want) {
			t.Errorf("missing gutter %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "▶") {
		t.Errorf("range output must not use single-line marker ▶:\n%s", out)
	}
}

func TestCodeRangeSwapped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(path, []byte("a\nb\nc\nd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Caller passed end < start; should still render range 2..3.
	out, err := CodeRange(path, 3, 2, 0)
	if err != nil {
		t.Fatalf("CodeRange: %v", err)
	}
	if !strings.Contains(out, "┌    2  ") || !strings.Contains(out, "└    3  ") {
		t.Errorf("expected normalized 2..3 range:\n%s", out)
	}
}

func TestCodeRangeSingleEqualsCode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(path, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// CodeRange with start==end should behave like Code (single ▶).
	a, err := CodeRange(path, 2, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Code(path, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("CodeRange(2,2) != Code(2):\n%q\nvs\n%q", a, b)
	}
	if !strings.Contains(a, "▶") {
		t.Errorf("single-line should use ▶: %s", a)
	}
}

func TestCodeBytesNoFilesystem(t *testing.T) {
	t.Parallel()
	// Content not backed by any file on disk; chroma should still highlight
	// based on the .go hintPath.
	content := []byte("package foo\n\nfunc Bar() {}\n")
	out, err := CodeBytes(content, "synthetic.go", 3, 3, 0)
	if err != nil {
		t.Fatalf("CodeBytes: %v", err)
	}
	if !strings.Contains(out, "▶    3  ") {
		t.Errorf("expected line 3 marker:\n%s", out)
	}
	if !strings.Contains(out, "Bar") {
		t.Errorf("expected source content in output:\n%s", out)
	}
}

func TestMarkdownBasic(t *testing.T) {
	t.Parallel()
	out, err := Markdown("# Hello\n\nworld\n", 40)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Hello") {
		t.Errorf("expected 'Hello' in rendered markdown:\n%s", out)
	}
	if !strings.Contains(out, "world") {
		t.Errorf("expected 'world' in rendered markdown:\n%s", out)
	}
}

func TestMarkdownDefaultWidth(t *testing.T) {
	t.Parallel()
	out, err := Markdown("hello", 0)
	if err != nil {
		t.Fatalf("Markdown: %v", err)
	}
	if out == "" {
		t.Errorf("empty output")
	}
}
