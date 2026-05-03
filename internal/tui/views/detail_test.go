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
type stubPreImage struct{ files map[string][]byte }

func (s *stubPreImage) Content(path string) ([]byte, error) {
	if b, ok := s.files[path]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("stubPreImage: no entry for %q", path)
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
	// Working tree (post-image): line 3 contains New().
	postSrc := "package x\n\nfunc New() {}\n"
	if err := os.WriteFile(filepath.Join(root, "x.go"), []byte(postSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	// Pre-image: line 3 contains Old(); range starts here.
	preSrc := "package x\n\nfunc Old() {}\n"
	pi := &stubPreImage{files: map[string][]byte{"x.go": []byte(preSrc)}}

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
	// Both pre-image and post-image content must appear in the cross-side view.
	if !strings.Contains(out, "Old") {
		t.Errorf("cross-side view missing pre-image excerpt:\n%s", out)
	}
	if !strings.Contains(out, "New") {
		t.Errorf("cross-side view missing post-image excerpt:\n%s", out)
	}
	// Separator labels should be present so the user can tell which is which.
	if !strings.Contains(out, "変更前") || !strings.Contains(out, "変更後") {
		t.Errorf("expected 変更前/変更後 separators in:\n%s", out)
	}
}
