package main

import (
	"strings"
	"testing"
)

func TestResolveReviewEngine(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		useClaude bool
		useCodex  bool
		want      reviewEngine
		wantErr   bool
	}{
		{"neither defaults to claude", false, false, engineClaude, false},
		{"explicit claude", true, false, engineClaude, false},
		{"explicit codex", false, true, engineCodex, false},
		{"both rejected", true, true, "", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveReviewEngine(tc.useClaude, tc.useCodex)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Fatalf("engine = %q, want %q", got, tc.want)
			}
			if tc.wantErr && !strings.Contains(err.Error(), "mutually exclusive") {
				t.Fatalf("error should mention mutually exclusive, got %q", err.Error())
			}
		})
	}
}

func TestReviewCmdFlagsRegistered(t *testing.T) {
	t.Parallel()
	cmd := newReviewCmd()
	for _, name := range []string{"claude", "codex", "focus"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("review cmd missing --%s flag", name)
		}
	}
	// Cobra rejects --claude --codex together via MarkFlagsMutuallyExclusive.
	// We verify the annotation is in place by checking the flag set.
	claudeFlag := cmd.Flags().Lookup("claude")
	codexFlag := cmd.Flags().Lookup("codex")
	if claudeFlag.Annotations["cobra_annotation_mutually_exclusive"] == nil {
		t.Errorf("--claude not marked mutually exclusive")
	}
	if codexFlag.Annotations["cobra_annotation_mutually_exclusive"] == nil {
		t.Errorf("--codex not marked mutually exclusive")
	}
}
