package model

import (
	"fmt"
	"sort"
	"sync"
)

// SeverityInfo is one entry in the severity registry. Mirrors the TOML
// shape in [[review.severity]] but lives in the model package so the
// runtime can reference it without depending on config.
type SeverityInfo struct {
	Name        string
	Level       int
	Description string
	ReviewEvent ReviewEvent
	Color       string
}

// SeverityRegistry is the set of severities revu treats as valid for a
// given run. Callers obtain the active one via ActiveSeverityRegistry().
type SeverityRegistry struct {
	defs   []SeverityInfo
	byName map[string]SeverityInfo
}

// NewSeverityRegistry validates and constructs a registry from defs.
// Returns an error when any entry is malformed (empty/duplicate name,
// unknown review_event).
func NewSeverityRegistry(defs []SeverityInfo) (*SeverityRegistry, error) {
	if len(defs) == 0 {
		return nil, fmt.Errorf("severity registry must contain at least one entry")
	}
	out := &SeverityRegistry{
		defs:   make([]SeverityInfo, len(defs)),
		byName: make(map[string]SeverityInfo, len(defs)),
	}
	for i, d := range defs {
		if d.Name == "" {
			return nil, fmt.Errorf("severities[%d].name is required", i)
		}
		if _, dup := out.byName[d.Name]; dup {
			return nil, fmt.Errorf("severities[%d].name %q is duplicated", i, d.Name)
		}
		if d.ReviewEvent == "" {
			d.ReviewEvent = EventComment
		}
		if !d.ReviewEvent.Valid() {
			return nil, fmt.Errorf("severities[%d].review_event %q is invalid (expected APPROVE | COMMENT | REQUEST_CHANGES)", i, d.ReviewEvent)
		}
		out.defs[i] = d
		out.byName[d.Name] = d
	}
	return out, nil
}

// Has reports whether name is a registered severity.
func (r *SeverityRegistry) Has(name string) bool {
	if r == nil {
		return false
	}
	_, ok := r.byName[name]
	return ok
}

// Info returns the SeverityInfo for name. The bool is false for unknown names.
func (r *SeverityRegistry) Info(name string) (SeverityInfo, bool) {
	if r == nil {
		return SeverityInfo{}, false
	}
	d, ok := r.byName[name]
	return d, ok
}

// All returns a copy of the registered severities in their declaration order.
func (r *SeverityRegistry) All() []SeverityInfo {
	if r == nil {
		return nil
	}
	out := make([]SeverityInfo, len(r.defs))
	copy(out, r.defs)
	return out
}

// SortedByLevel returns severities sorted by Level descending (most severe first).
// Useful for skill-side review_event aggregation where we pick the strongest event.
func (r *SeverityRegistry) SortedByLevel() []SeverityInfo {
	out := r.All()
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Level > out[j].Level
	})
	return out
}

// DefaultSeverityRegistry returns the built-in 4-level severity set used
// when the user has not configured [[review.severity]] in config.toml.
func DefaultSeverityRegistry() *SeverityRegistry {
	r, err := NewSeverityRegistry([]SeverityInfo{
		{
			Name:        "critical",
			Level:       100,
			Description: "本番障害・データ破損・重大セキュリティに直結する",
			ReviewEvent: EventRequestChanges,
			Color:       "red",
		},
		{
			Name:        "major",
			Level:       80,
			Description: "設計の根本問題、リファクタが必要、将来のバグ温床",
			ReviewEvent: EventRequestChanges,
			Color:       "yellow",
		},
		{
			Name:        "minor",
			Level:       30,
			Description: "改善はするが優先度低、現状でも動く",
			ReviewEvent: EventComment,
			Color:       "cyan",
		},
		{
			Name:        "nit",
			Level:       10,
			Description: "趣味・スタイルの提案、無視されても困らない",
			ReviewEvent: EventComment,
			Color:       "gray",
		},
	})
	if err != nil {
		panic(fmt.Sprintf("default severity registry must be valid: %v", err))
	}
	return r
}

var (
	registryMu      sync.RWMutex
	activeRegistry  = DefaultSeverityRegistry()
)

// ActiveSeverityRegistry returns the registry that Severity.Valid() and
// other validation paths consult. Defaults to DefaultSeverityRegistry()
// until SetActiveSeverityRegistry is called (typically by the CLI bootstrap).
func ActiveSeverityRegistry() *SeverityRegistry {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return activeRegistry
}

// SetActiveSeverityRegistry installs r as the process-wide registry.
// Passing nil restores the default. Tests that override the registry
// should restore the default in t.Cleanup.
func SetActiveSeverityRegistry(r *SeverityRegistry) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if r == nil {
		activeRegistry = DefaultSeverityRegistry()
		return
	}
	activeRegistry = r
}
