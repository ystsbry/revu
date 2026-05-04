package claude

import (
	"bytes"
	"strings"
	"testing"
)

func TestRelayProgress(t *testing.T) {
	t.Parallel()
	// One stream-json event per line, as claude emits them.
	input := strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"x"}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"I'll start by reading the SKILL.md.\nThen run revu."}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"revu pr prepare 231","description":"fetch PR metadata"}}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/home/u/.claude/skills/review-pr/SKILL.md"}}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Write","input":{"file_path":"/home/u/.revu/o/r/pr-1/comments/c1-app-86.md"}}]}}`,
		`{"type":"rate_limit_event","ignored":true}`,
		`{"type":"result","subtype":"success","duration_ms":45123,"total_cost_usd":0.456,"num_turns":12}`,
	}, "\n")

	var buf bytes.Buffer
	if err := relayProgress(strings.NewReader(input), &buf); err != nil {
		t.Fatalf("relayProgress: %v", err)
	}
	got := buf.String()

	wants := []string{
		"claude session ready",
		"I'll start by reading the SKILL.md.",
		"Bash: revu pr prepare 231",
		"fetch PR metadata",
		"Read:",
		"SKILL.md",
		"Write:",
		"c1-app-86.md",
		"done in 45s",
		"$0.4560",
		"12 turns",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in output:\n%s", w, got)
		}
	}
}

func TestRelayProgressIgnoresGarbageLines(t *testing.T) {
	t.Parallel()
	input := "not json\n" +
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"echo hi"}}]}}` + "\n" +
		"<<malformed>>\n"
	var buf bytes.Buffer
	if err := relayProgress(strings.NewReader(input), &buf); err != nil {
		t.Fatalf("relayProgress: %v", err)
	}
	if !strings.Contains(buf.String(), "Bash: echo hi") {
		t.Errorf("expected Bash event despite garbage lines: %s", buf.String())
	}
}

func TestSummarizeToolUseFallback(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		block    streamBlock
		summary  string
		hasInput bool
	}{
		{"unknown tool", streamBlock{Type: "tool_use", Name: "ExoticTool"}, "ExoticTool", false},
		{"empty name", streamBlock{Type: "tool_use", Name: ""}, "", false},
		{"glob with path", streamBlock{Type: "tool_use", Name: "Glob", Input: []byte(`{"pattern":"**/*.go","path":"/x"}`)}, "Glob: **/*.go", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, _ := summarizeToolUse(tc.block)
			if got != tc.summary {
				t.Errorf("summary = %q, want %q", got, tc.summary)
			}
		})
	}
}

func TestTruncateRespectsRunes(t *testing.T) {
	t.Parallel()
	in := "あいうえおかきくけこ" // 10 runes, 30 bytes
	if got := truncate(in, 5); got != "あいうえ…" {
		t.Errorf("truncate = %q, want %q", got, "あいうえ…")
	}
	if got := truncate(in, 100); got != in {
		t.Errorf("truncate should leave unchanged, got %q", got)
	}
}
