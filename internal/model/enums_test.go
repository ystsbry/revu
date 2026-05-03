package model

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestStatusUnmarshal(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    Status
		wantErr bool
	}{
		{"pending", StatusPending, false},
		{"accepted", StatusAccepted, false},
		{"rejected", StatusRejected, false},
		{"edited", StatusEdited, false},
		{"bogus", "", true},
		{`""`, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			var got Status
			err := yaml.Unmarshal([]byte(tc.in+"\n"), &got)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Unmarshal(%q) err=%v wantErr=%v", tc.in, err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestSeverityUnmarshal(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"nit", false},
		{"minor", false},
		{"major", false},
		{"critical", false},
		{"BLOCKER", true},
	}
	for _, tc := range cases {
		var got Severity
		err := yaml.Unmarshal([]byte(tc.in+"\n"), &got)
		if (err != nil) != tc.wantErr {
			t.Errorf("Severity(%q) err=%v wantErr=%v", tc.in, err, tc.wantErr)
		}
	}
}

func TestCategoryUnmarshal(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"bug", "design", "style", "perf", "security", "test", "doc"} {
		var got Category
		if err := yaml.Unmarshal([]byte(in+"\n"), &got); err != nil {
			t.Errorf("Category(%q) unexpected err: %v", in, err)
		}
	}
	var bad Category
	err := yaml.Unmarshal([]byte("ux\n"), &bad)
	if err == nil || !strings.Contains(err.Error(), "invalid category") {
		t.Errorf("expected invalid category error, got: %v", err)
	}
}

func TestSideUnmarshal(t *testing.T) {
	t.Parallel()
	var s Side
	if err := yaml.Unmarshal([]byte("RIGHT\n"), &s); err != nil || s != SideRight {
		t.Fatalf("RIGHT: err=%v s=%q", err, s)
	}
	if err := yaml.Unmarshal([]byte("LEFT\n"), &s); err != nil || s != SideLeft {
		t.Fatalf("LEFT: err=%v s=%q", err, s)
	}
	if err := yaml.Unmarshal([]byte("right\n"), &s); err == nil {
		t.Fatalf("lowercase right should fail")
	}
}

func TestReviewEventUnmarshal(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"APPROVE", "COMMENT", "REQUEST_CHANGES"} {
		var got ReviewEvent
		if err := yaml.Unmarshal([]byte(in+"\n"), &got); err != nil {
			t.Errorf("ReviewEvent(%q) unexpected err: %v", in, err)
		}
	}
	var bad ReviewEvent
	if err := yaml.Unmarshal([]byte("BLOCK\n"), &bad); err == nil {
		t.Errorf("expected error for BLOCK")
	}
}
