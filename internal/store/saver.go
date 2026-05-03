package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/ystsbry/revu/internal/model"
)

// SaveStatuses persists the current Comment.Status values back to review.yml.
// It rewrites the entire file (after re-marshaling the in-memory Review), then
// renames atomically into place. Existing comments and ordering in the YAML
// are not preserved beyond what yaml.v3 produces by default.
//
// In Phase 1 we accept this limitation; if comment-preservation becomes
// important, switch to a yaml.Node-based partial editor.
func SaveStatuses(r *model.Review) error {
	if r == nil {
		return errors.New("nil review")
	}
	if r.BaseDir == "" {
		return errors.New("review has no BaseDir; load it via store.Load first")
	}

	out, err := yaml.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshal review: %w", err)
	}

	dst := filepath.Join(r.BaseDir, "review.yml")
	return atomicWrite(dst, out, 0o644)
}

func atomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".review-*.yml.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("rename temp -> %s: %w", path, err)
	}
	return nil
}
