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

// SaveSessionID patches review.yml under reviewDir so generated_by.session_id
// is set to the given value. Implemented as a targeted yaml.Node edit rather
// than a full re-marshal of the Review struct, so the rest of the file
// (comment ordering, scalar styles, etc.) is left untouched. A no-op when
// sessionID is empty.
func SaveSessionID(reviewDir, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	if reviewDir == "" {
		return errors.New("reviewDir is required")
	}
	path := filepath.Join(reviewDir, "review.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	doc := mappingNode(&root)
	if doc == nil {
		return fmt.Errorf("%s: top-level is not a mapping", path)
	}
	gen := childNode(doc, "generated_by")
	if gen == nil {
		// generated_by missing entirely: append a fresh mapping.
		gen = &yaml.Node{Kind: yaml.MappingNode}
		doc.Content = append(doc.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "generated_by"},
			gen,
		)
	}
	if gen.Kind != yaml.MappingNode {
		return fmt.Errorf("%s: generated_by is not a mapping", path)
	}
	if existing := childNode(gen, "session_id"); existing != nil {
		existing.Kind = yaml.ScalarNode
		existing.Tag = ""
		existing.Style = 0
		existing.Value = sessionID
	} else {
		gen.Content = append(gen.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "session_id"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: sessionID},
		)
	}

	out, err := yaml.Marshal(&root)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	return atomicWrite(path, out, 0o644)
}

// mappingNode returns the document's mapping node, unwrapping a top-level
// DocumentNode if present.
func mappingNode(n *yaml.Node) *yaml.Node {
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		return n.Content[0]
	}
	return n
}

// childNode returns the value node for the given key in a mapping, or nil
// if absent.
func childNode(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
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
