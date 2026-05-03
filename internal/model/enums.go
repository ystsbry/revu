package model

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusAccepted Status = "accepted"
	StatusRejected Status = "rejected"
	StatusEdited   Status = "edited"
)

func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusAccepted, StatusRejected, StatusEdited:
		return true
	}
	return false
}

func (s *Status) UnmarshalYAML(node *yaml.Node) error {
	var raw string
	if err := node.Decode(&raw); err != nil {
		return err
	}
	v := Status(raw)
	if !v.Valid() {
		return fmt.Errorf("invalid status %q (line %d)", raw, node.Line)
	}
	*s = v
	return nil
}

type Severity string

const (
	SeverityNit      Severity = "nit"
	SeverityMinor    Severity = "minor"
	SeverityMajor    Severity = "major"
	SeverityCritical Severity = "critical"
)

func (s Severity) Valid() bool {
	switch s {
	case SeverityNit, SeverityMinor, SeverityMajor, SeverityCritical:
		return true
	}
	return false
}

func (s *Severity) UnmarshalYAML(node *yaml.Node) error {
	var raw string
	if err := node.Decode(&raw); err != nil {
		return err
	}
	v := Severity(raw)
	if !v.Valid() {
		return fmt.Errorf("invalid severity %q (line %d)", raw, node.Line)
	}
	*s = v
	return nil
}

type Category string

const (
	CategoryBug      Category = "bug"
	CategoryDesign   Category = "design"
	CategoryStyle    Category = "style"
	CategoryPerf     Category = "perf"
	CategorySecurity Category = "security"
	CategoryTest     Category = "test"
	CategoryDoc      Category = "doc"
)

func (c Category) Valid() bool {
	switch c {
	case CategoryBug, CategoryDesign, CategoryStyle, CategoryPerf,
		CategorySecurity, CategoryTest, CategoryDoc:
		return true
	}
	return false
}

func (c *Category) UnmarshalYAML(node *yaml.Node) error {
	var raw string
	if err := node.Decode(&raw); err != nil {
		return err
	}
	v := Category(raw)
	if !v.Valid() {
		return fmt.Errorf("invalid category %q (line %d)", raw, node.Line)
	}
	*c = v
	return nil
}

type Side string

const (
	SideRight Side = "RIGHT"
	SideLeft  Side = "LEFT"
)

func (s Side) Valid() bool {
	switch s {
	case SideRight, SideLeft:
		return true
	}
	return false
}

func (s *Side) UnmarshalYAML(node *yaml.Node) error {
	var raw string
	if err := node.Decode(&raw); err != nil {
		return err
	}
	v := Side(raw)
	if !v.Valid() {
		return fmt.Errorf("invalid side %q (line %d, expected RIGHT or LEFT)", raw, node.Line)
	}
	*s = v
	return nil
}

type ReviewEvent string

const (
	EventApprove        ReviewEvent = "APPROVE"
	EventComment        ReviewEvent = "COMMENT"
	EventRequestChanges ReviewEvent = "REQUEST_CHANGES"
)

func (e ReviewEvent) Valid() bool {
	switch e {
	case EventApprove, EventComment, EventRequestChanges:
		return true
	}
	return false
}

func (e *ReviewEvent) UnmarshalYAML(node *yaml.Node) error {
	var raw string
	if err := node.Decode(&raw); err != nil {
		return err
	}
	v := ReviewEvent(raw)
	if !v.Valid() {
		return fmt.Errorf("invalid review_event %q (line %d)", raw, node.Line)
	}
	*e = v
	return nil
}
