package views

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/tui/keys"
)

func detailFixture(t *testing.T) (*model.Review, string) {
	t.Helper()
	root := t.TempDir()
	src := `package x

func A() {
	a := 1
	b := 2
	_ = a + b
}
`
	target := filepath.Join(root, "x.go")
	if err := os.WriteFile(target, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &model.Review{
		BaseDir: root,
		PR:      model.PRMeta{Repo: "o/r", Number: 1},
		Comments: []model.Comment{
			{ID: "c1", Status: model.StatusPending, Severity: model.SeverityMajor, Category: model.CategoryDesign, Path: "x.go", Line: 4, Side: model.SideRight, BodyFile: "c1.md", Body: "## fix this\n"},
			{ID: "c2", Status: model.StatusPending, Severity: model.SeverityNit, Category: model.CategoryStyle, Path: "x.go", Line: 5, Side: model.SideRight, BodyFile: "c2.md", Body: "ok"},
		},
	}
	return r, root
}

func TestDetailNavigateNext(t *testing.T) {
	t.Parallel()
	r, root := detailFixture(t)
	d := NewDetail(r, root, keys.DefaultKeyMap(), 0, DetailSettings{})

	if d.Index() != 0 {
		t.Fatalf("initial index = %d", d.Index())
	}
	d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if d.Index() != 1 {
		t.Errorf("after n: %d", d.Index())
	}
	// Past the end clamps.
	d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if d.Index() != 1 {
		t.Errorf("past end clamps: %d", d.Index())
	}
	d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if d.Index() != 0 {
		t.Errorf("after p: %d", d.Index())
	}
}

func TestDetailArrowKeysScrollMarkdown(t *testing.T) {
	t.Parallel()
	r, root := detailFixture(t)
	d := NewDetail(r, root, keys.DefaultKeyMap(), 0, DetailSettings{})
	// Pretend the rendered markdown overflows the pane so the scroll
	// guard against past-the-bottom drift doesn't pin mdScroll at 0.
	d.mdMaxScroll = 10

	// ↓ scrolls the markdown pane without advancing the comment index.
	d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.Index() != 0 {
		t.Errorf("arrow down moved comment index to %d, want 0", d.Index())
	}
	if d.mdScroll != 1 {
		t.Errorf("after ↓: mdScroll = %d, want 1", d.mdScroll)
	}
	d.Update(tea.KeyMsg{Type: tea.KeyDown})
	if d.mdScroll != 2 {
		t.Errorf("after second ↓: mdScroll = %d, want 2", d.mdScroll)
	}

	// ↑ scrolls back, clamped at 0.
	d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.mdScroll != 1 {
		t.Errorf("after ↑: mdScroll = %d, want 1", d.mdScroll)
	}
	d.Update(tea.KeyMsg{Type: tea.KeyUp})
	d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.mdScroll != 0 {
		t.Errorf("clamped: mdScroll = %d, want 0", d.mdScroll)
	}

	// Home jumps to the top, End to the bottom.
	d.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if d.mdScroll != d.mdMaxScroll {
		t.Errorf("after End: mdScroll = %d, want %d", d.mdScroll, d.mdMaxScroll)
	}
	d.Update(tea.KeyMsg{Type: tea.KeyHome})
	if d.mdScroll != 0 {
		t.Errorf("after Home: mdScroll = %d, want 0", d.mdScroll)
	}

	// ↓ doesn't grow past mdMaxScroll.
	for i := 0; i < d.mdMaxScroll+5; i++ {
		d.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if d.mdScroll != d.mdMaxScroll {
		t.Errorf("after overshoot: mdScroll = %d, want clamp at %d", d.mdScroll, d.mdMaxScroll)
	}

	// Switching comments resets scroll.
	d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if d.mdScroll != 0 {
		t.Errorf("after n: mdScroll = %d, want 0 (reset)", d.mdScroll)
	}
}

func TestClipPaneHeight(t *testing.T) {
	t.Parallel()
	in := "a\nb\nc\nd\ne"
	cases := []struct {
		name   string
		height int
		want   string
	}{
		{"truncate", 3, "a\nb\nc"},
		{"unchanged when under limit", 10, in},
		{"exact fit", 5, in},
		{"zero", 0, ""},
		{"negative", -1, ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := clipPaneHeight(in, tc.height); got != tc.want {
				t.Errorf("clipPaneHeight(%q, %d) = %q, want %q", in, tc.height, got, tc.want)
			}
		})
	}
}

func TestDetailAcceptMutates(t *testing.T) {
	t.Parallel()
	r, root := detailFixture(t)
	d := NewDetail(r, root, keys.DefaultKeyMap(), 0, DetailSettings{})

	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if r.Comments[0].Status != model.StatusAccepted {
		t.Errorf("c1 status = %s", r.Comments[0].Status)
	}
	if cmd == nil {
		t.Fatal("expected DirtyMsg cmd")
	}
	if _, ok := cmd().(DirtyMsg); !ok {
		t.Errorf("got %T, want DirtyMsg", cmd())
	}
}

func TestDetailGoToList(t *testing.T) {
	t.Parallel()
	r, root := detailFixture(t)
	d := NewDetail(r, root, keys.DefaultKeyMap(), 0, DetailSettings{})

	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if cmd == nil {
		t.Fatal("expected GoToListMsg cmd")
	}
	if _, ok := cmd().(GoToListMsg); !ok {
		t.Errorf("got %T, want GoToListMsg", cmd())
	}
}

func TestDetailEditEmitsAbsPath(t *testing.T) {
	t.Parallel()
	r, root := detailFixture(t)
	d := NewDetail(r, root, keys.DefaultKeyMap(), 0, DetailSettings{})

	_, cmd := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if cmd == nil {
		t.Fatal("expected EditMsg cmd")
	}
	em, ok := cmd().(EditMsg)
	if !ok {
		t.Fatalf("got %T, want EditMsg", cmd())
	}
	if !filepath.IsAbs(em.Path) {
		t.Errorf("EditMsg.Path should be absolute, got %q", em.Path)
	}
	if !strings.HasSuffix(em.Path, "c1.md") {
		t.Errorf("expected path ending in c1.md, got %q", em.Path)
	}
}

func TestDetailViewLayout(t *testing.T) {
	t.Parallel()
	r, root := detailFixture(t)
	d := NewDetail(r, root, keys.DefaultKeyMap(), 0, DetailSettings{})

	d.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	wide := d.View()
	if !strings.Contains(wide, "c1") {
		t.Errorf("wide view missing comment id:\n%s", wide)
	}

	d.Update(tea.WindowSizeMsg{Width: 60, Height: 30})
	narrow := d.View()
	if !strings.Contains(narrow, "c1") {
		t.Errorf("narrow view missing comment id:\n%s", narrow)
	}
}

// stubPreImage implements PreImageSource for testing without spawning git.
type stubPreImage struct {
	files     map[string][]byte // pre-image
	postFiles map[string][]byte // post-image
	diffs     map[string][]byte
}

func (s *stubPreImage) Content(path string) ([]byte, error) {
	if b, ok := s.files[path]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("stubPreImage: no entry for %q", path)
}

func (s *stubPreImage) PostImage(path string) ([]byte, error) {
	if b, ok := s.postFiles[path]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("stubPreImage: no post-image for %q", path)
}

func (s *stubPreImage) Diff(path string) ([]byte, error) {
	if b, ok := s.diffs[path]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("stubPreImage: no diff for %q", path)
}

func TestDetailCodeContentLeftSide(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Working tree contains only the post-image.
	postSrc := "package x\n\nfunc New() {}\n"
	if err := os.WriteFile(filepath.Join(root, "x.go"), []byte(postSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	// Pre-image is supplied via the stub — different content, so we can
	// distinguish which source the renderer pulled from.
	preSrc := "package x\n\nfunc Old() {}\n"
	pi := &stubPreImage{files: map[string][]byte{"x.go": []byte(preSrc)}}

	r := &model.Review{
		BaseDir: root,
		PR:      model.PRMeta{Repo: "o/r", Number: 1, HeadSHA: "h", BaseBranch: "main"},
		Comments: []model.Comment{
			{ID: "c1", Status: model.StatusPending, Severity: model.SeverityMinor,
				Category: model.CategoryDesign, Path: "x.go", Line: 3,
				Side: model.SideLeft, BodyFile: "c1.md", Body: "x"},
		},
	}
	d := NewDetail(r, root, keys.DefaultKeyMap(), 0, DetailSettings{PreImage: pi})

	out, err := d.codeContent(&r.Comments[0])
	if err != nil {
		t.Fatalf("codeContent: %v", err)
	}
	if !strings.Contains(out, "Old") {
		t.Errorf("LEFT comment should render pre-image content; got:\n%s", out)
	}
	if strings.Contains(out, "New") {
		t.Errorf("LEFT comment must not pull from working tree; got:\n%s", out)
	}
}

func TestDetailCodeContentCrossSide(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// codeContent for cross-side does not touch the working tree (the diff
	// hunk supplied via PreImageSource is the sole source of code), but
	// repoRoot must be set so the early-return guard passes.
	if err := os.WriteFile(filepath.Join(root, "x.go"), []byte("dummy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stubDiff := "diff --git a/x.go b/x.go\n" +
		"--- a/x.go\n" +
		"+++ b/x.go\n" +
		"@@ -1,3 +1,3 @@\n" +
		" package x\n" +
		" \n" +
		"-func Old() {}\n" +
		"+func New() {}\n"
	pi := &stubPreImage{
		diffs: map[string][]byte{"x.go": []byte(stubDiff)},
	}

	startLine := 3
	startSide := model.SideLeft
	r := &model.Review{
		BaseDir: root,
		PR:      model.PRMeta{Repo: "o/r", Number: 1, HeadSHA: "h", BaseBranch: "main"},
		Comments: []model.Comment{
			{ID: "c1", Status: model.StatusPending, Severity: model.SeverityMinor,
				Category: model.CategoryDesign, Path: "x.go", Line: 3,
				Side:      model.SideRight,
				StartLine: &startLine,
				StartSide: &startSide,
				BodyFile:  "c1.md", Body: "x"},
		},
	}
	d := NewDetail(r, root, keys.DefaultKeyMap(), 0, DetailSettings{PreImage: pi})

	out, err := d.codeContent(&r.Comments[0])
	if err != nil {
		t.Fatalf("codeContent: %v", err)
	}
	// The unified diff hunk must show both deletion and addition, plus the
	// hunk header. Anchor markers should appear on each side.
	for _, want := range []string{"@@ -1,3 +1,3 @@", "- func Old()", "+ func New()"} {
		if !strings.Contains(out, want) {
			t.Errorf("cross-side hunk missing %q in:\n%s", want, out)
		}
	}
	if got := strings.Count(out, "▶"); got != 2 {
		t.Errorf("expected anchor markers on both endpoints, got %d:\n%s", got, out)
	}
}

func TestDetailCodeContentRightSideUsesPostImage(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Working tree carries DIFFERENT content than what the review was
	// generated against, simulating the user having uncommitted changes
	// after the review was created.
	workingTree := "package x\n\nfunc Drifted() {}\n"
	if err := os.WriteFile(filepath.Join(root, "x.go"), []byte(workingTree), 0o644); err != nil {
		t.Fatal(err)
	}
	// The review's head_sha state — what should actually be displayed.
	postSrc := "package x\n\nfunc Reviewed() {}\n"
	pi := &stubPreImage{
		postFiles: map[string][]byte{"x.go": []byte(postSrc)},
	}

	r := &model.Review{
		BaseDir: root,
		PR:      model.PRMeta{Repo: "o/r", Number: 1, HeadSHA: "h", BaseBranch: "main"},
		Comments: []model.Comment{
			{ID: "c1", Status: model.StatusPending, Severity: model.SeverityMinor,
				Category: model.CategoryDesign, Path: "x.go", Line: 3,
				Side: model.SideRight, BodyFile: "c1.md", Body: "x"},
		},
	}
	d := NewDetail(r, root, keys.DefaultKeyMap(), 0, DetailSettings{PreImage: pi})

	out, err := d.codeContent(&r.Comments[0])
	if err != nil {
		t.Fatalf("codeContent: %v", err)
	}
	if !strings.Contains(out, "Reviewed") {
		t.Errorf("RIGHT comment should render post-image; got:\n%s", out)
	}
	if strings.Contains(out, "Drifted") {
		t.Errorf("RIGHT comment must not pull from working tree when post-image is available; got:\n%s", out)
	}
}

func TestDetailCodeContentRightSideFallsBackToWorkingTree(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	workingTree := "package x\n\nfunc Local() {}\n"
	if err := os.WriteFile(filepath.Join(root, "x.go"), []byte(workingTree), 0o644); err != nil {
		t.Fatal(err)
	}
	// Stub returns no post-image — simulates head_sha not resolving.
	pi := &stubPreImage{}

	r := &model.Review{
		BaseDir: root,
		PR:      model.PRMeta{Repo: "o/r", Number: 1, HeadSHA: "abc1234", BaseBranch: "main"},
		Comments: []model.Comment{
			{ID: "c1", Status: model.StatusPending, Severity: model.SeverityMinor,
				Category: model.CategoryDesign, Path: "x.go", Line: 3,
				Side: model.SideRight, BodyFile: "c1.md", Body: "x"},
		},
	}
	d := NewDetail(r, root, keys.DefaultKeyMap(), 0, DetailSettings{PreImage: pi})

	out, err := d.codeContent(&r.Comments[0])
	if err != nil {
		t.Fatalf("codeContent: %v", err)
	}
	if !strings.Contains(out, "Local") {
		t.Errorf("expected working-tree fallback content; got:\n%s", out)
	}
	if !strings.Contains(out, "working tree") {
		t.Errorf("expected degraded notice mentioning working tree; got:\n%s", out)
	}
}
