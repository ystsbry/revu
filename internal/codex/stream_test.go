package codex

import (
	"bytes"
	"strings"
	"testing"
)

func TestRelayProgress(t *testing.T) {
	t.Parallel()
	// One JSONL event per line, as `codex exec --json` emits them.
	input := strings.Join([]string{
		`{"type":"thread.started","thread_id":"019e9ff6-5662-7162-b698-c898990a7435"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.completed","item":{"id":"i0","type":"agent_message","text":"I'll start by reading SKILL.md.\nThen run revu."}}`,
		`{"type":"item.completed","item":{"id":"i1","type":"command_executed","command":"revu pr prepare 231","exit_code":0}}`,
		`{"type":"item.completed","item":{"id":"i2","type":"file_write","path":"/home/u/.revu/o/r/pr-1/comments/c1-app-86.md"}}`,
		`{"type":"item.completed","item":{"id":"i3","type":"reasoning","text":"internal monologue"}}`,
		`{"type":"unknown.event","ignored":true}`,
		`{"type":"turn.completed","usage":{"input_tokens":11191,"cached_input_tokens":9600,"output_tokens":5}}`,
	}, "\n")

	var buf bytes.Buffer
	sessionID, err := relayProgress(strings.NewReader(input), &buf)
	if err != nil {
		t.Fatalf("relayProgress: %v", err)
	}
	if sessionID != "019e9ff6-5662-7162-b698-c898990a7435" {
		t.Errorf("sessionID = %q, want thread_id from thread.started", sessionID)
	}
	got := buf.String()

	wants := []string{
		"codex session ready",
		"I'll start by reading SKILL.md.",
		"Bash: revu pr prepare 231",
		"Write:",
		"c1-app-86.md",
		"done",
		"11191",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in output:\n%s", w, got)
		}
	}

	// Reasoning items are intentionally muted.
	if strings.Contains(got, "internal monologue") {
		t.Errorf("reasoning text leaked into progress output:\n%s", got)
	}
}

func TestRelayProgressIgnoresGarbageLines(t *testing.T) {
	t.Parallel()
	input := "not json\n" +
		`{"type":"item.completed","item":{"type":"command_executed","command":"echo hi"}}` + "\n" +
		"<<malformed>>\n"
	var buf bytes.Buffer
	if _, err := relayProgress(strings.NewReader(input), &buf); err != nil {
		t.Fatalf("relayProgress: %v", err)
	}
	if !strings.Contains(buf.String(), "Bash: echo hi") {
		t.Errorf("expected Bash event despite garbage lines: %s", buf.String())
	}
}

func TestRelayProgressErrorEvent(t *testing.T) {
	t.Parallel()
	input := `{"type":"error","message":"sandbox denied write to /etc/passwd"}` + "\n"
	var buf bytes.Buffer
	if _, err := relayProgress(strings.NewReader(input), &buf); err != nil {
		t.Fatalf("relayProgress: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "codex error") || !strings.Contains(out, "sandbox denied") {
		t.Errorf("expected codex error line, got: %s", out)
	}
}

func TestSummarizeItemFallback(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		item    streamItem
		summary string
	}{
		{"unknown type", streamItem{Type: "exotic_item"}, "exotic_item"},
		{"empty type", streamItem{Type: ""}, ""},
		{"agent_message empty text", streamItem{Type: "agent_message", Text: "   "}, ""},
		{"command no name", streamItem{Type: "command_executed"}, "Bash"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, _ := summarizeItem(tc.item)
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
