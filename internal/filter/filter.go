// Package filter provides a minimal expression parser for narrowing the
// comment list. Expressions are AND-combined whitespace-separated terms
// of the form "key:value[,value...]".
//
// Supported keys:
//
//	severity   nit | minor | major | critical
//	category   bug | design | style | perf | security | test | doc
//	status     pending | accepted | rejected | edited
//	path       substring match (case-insensitive) on Comment.Path
//
// Examples:
//
//	"severity:major,critical"
//	"status:pending category:bug"
//	"path:application.py severity:major"
package filter

import (
	"fmt"
	"strings"

	"github.com/ystsbry/revu/internal/model"
)

// Filter is the parsed form of an expression. The zero value matches all
// comments (i.e. no filter).
type Filter struct {
	severities map[model.Severity]struct{}
	categories map[model.Category]struct{}
	statuses   map[model.Status]struct{}
	pathSubs   []string // lowercased path substrings (OR within, AND with other dimensions)
	raw        string
}

// IsEmpty reports whether this Filter accepts everything.
func (f Filter) IsEmpty() bool {
	return len(f.severities) == 0 && len(f.categories) == 0 && len(f.statuses) == 0 && len(f.pathSubs) == 0
}

// String returns the original expression (canonicalised by Parse).
func (f Filter) String() string { return f.raw }

// Match returns true iff the comment satisfies every dimension of the filter.
// An unset dimension is treated as wildcard.
func (f Filter) Match(c *model.Comment) bool {
	if c == nil {
		return false
	}
	if len(f.severities) > 0 {
		if _, ok := f.severities[c.Severity]; !ok {
			return false
		}
	}
	if len(f.categories) > 0 {
		if _, ok := f.categories[c.Category]; !ok {
			return false
		}
	}
	if len(f.statuses) > 0 {
		if _, ok := f.statuses[c.Status]; !ok {
			return false
		}
	}
	if len(f.pathSubs) > 0 {
		path := strings.ToLower(c.Path)
		hit := false
		for _, sub := range f.pathSubs {
			if strings.Contains(path, sub) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return true
}

// Parse turns an expression like "severity:major,critical category:bug"
// into a Filter. Whitespace separates AND terms; comma separates OR values
// within a single term. Empty input returns the zero Filter.
func Parse(expr string) (Filter, error) {
	expr = strings.TrimSpace(expr)
	out := Filter{raw: expr}
	if expr == "" {
		return out, nil
	}

	for _, term := range strings.Fields(expr) {
		key, val, ok := strings.Cut(term, ":")
		if !ok || val == "" {
			return Filter{}, fmt.Errorf("bad term %q (expected key:value)", term)
		}
		values := strings.Split(val, ",")
		switch strings.ToLower(key) {
		case "severity":
			if out.severities == nil {
				out.severities = map[model.Severity]struct{}{}
			}
			for _, v := range values {
				s := model.Severity(strings.TrimSpace(v))
				if !s.Valid() {
					return Filter{}, fmt.Errorf("invalid severity %q", v)
				}
				out.severities[s] = struct{}{}
			}
		case "category":
			if out.categories == nil {
				out.categories = map[model.Category]struct{}{}
			}
			for _, v := range values {
				c := model.Category(strings.TrimSpace(v))
				if !c.Valid() {
					return Filter{}, fmt.Errorf("invalid category %q", v)
				}
				out.categories[c] = struct{}{}
			}
		case "status":
			if out.statuses == nil {
				out.statuses = map[model.Status]struct{}{}
			}
			for _, v := range values {
				s := model.Status(strings.TrimSpace(v))
				if !s.Valid() {
					return Filter{}, fmt.Errorf("invalid status %q", v)
				}
				out.statuses[s] = struct{}{}
			}
		case "path":
			for _, v := range values {
				v = strings.TrimSpace(v)
				if v == "" {
					continue
				}
				out.pathSubs = append(out.pathSubs, strings.ToLower(v))
			}
		default:
			return Filter{}, fmt.Errorf("unknown filter key %q", key)
		}
	}
	return out, nil
}

// VisibleIndices applies f to a slice and returns the indices of comments
// that match, preserving order. Useful for List views that need to map
// filtered positions back to original indices.
func (f Filter) VisibleIndices(comments []model.Comment) []int {
	if f.IsEmpty() {
		out := make([]int, len(comments))
		for i := range comments {
			out[i] = i
		}
		return out
	}
	out := make([]int, 0, len(comments))
	for i := range comments {
		if f.Match(&comments[i]) {
			out = append(out, i)
		}
	}
	return out
}
