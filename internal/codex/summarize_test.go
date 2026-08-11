package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func exitCode(n int) *int { return &n }

// Each stream item type codex emits maps to one progress line.
func TestSummarizeItemPerType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		item        streamItem
		wantSummary string
		wantDetail  string
	}{
		{
			name:        "agent message shows its first line",
			item:        streamItem{Type: "agent_message", Text: "looking at the diff\nthen the tests"},
			wantSummary: "looking at the diff",
		},
		{
			name: "an empty agent message stays quiet",
			item: streamItem{Type: "agent_message", Text: "   \n"},
		},
		{
			// Internal monologue is deliberately not surfaced.
			name: "reasoning is suppressed",
			item: streamItem{Type: "reasoning", Text: "hmm"},
		},
		{
			name:        "command with a zero exit has no detail",
			item:        streamItem{Type: "command_executed", Command: "go build ./...", ExitCode: exitCode(0)},
			wantSummary: "Bash: go build ./...",
		},
		{
			name:        "a failing command carries its exit code",
			item:        streamItem{Type: "command_executed", Command: "go test ./...", ExitCode: exitCode(2)},
			wantSummary: "Bash: go test ./...",
			wantDetail:  "exit 2",
		},
		{
			name:        "a command with no text still reports Bash",
			item:        streamItem{Type: "command_executed"},
			wantSummary: "Bash",
		},
		{
			name:        "file_edit",
			item:        streamItem{Type: "file_edit", Path: "/tmp/x.go"},
			wantSummary: "Edit: /tmp/x.go",
		},
		{
			name:        "file_change is the same line as file_edit",
			item:        streamItem{Type: "file_change", Path: "/tmp/x.go"},
			wantSummary: "Edit: /tmp/x.go",
		},
		{
			name:        "patch_applied is the same line as file_edit",
			item:        streamItem{Type: "patch_applied", Path: "/tmp/x.go"},
			wantSummary: "Edit: /tmp/x.go",
		},
		{
			name:        "file_read",
			item:        streamItem{Type: "file_read", Path: "/tmp/x.go"},
			wantSummary: "Read: /tmp/x.go",
		},
		{
			name:        "file_write",
			item:        streamItem{Type: "file_write", Path: "/tmp/x.go"},
			wantSummary: "Write: /tmp/x.go",
		},
		{
			name:        "web_search",
			item:        streamItem{Type: "web_search", Query: "golang testing"},
			wantSummary: "WebSearch: golang testing",
		},
		{
			name:        "web_fetch",
			item:        streamItem{Type: "web_fetch", URL: "https://example.com"},
			wantSummary: "WebFetch: https://example.com",
		},
		{
			name:        "tool_call",
			item:        streamItem{Type: "tool_call", Name: "search"},
			wantSummary: "Tool: search",
		},
		{
			name:        "an unnamed tool_call still says a tool ran",
			item:        streamItem{Type: "tool_call"},
			wantSummary: "Tool: tool",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			summary, detail := summarizeItem(tt.item)
			if summary != tt.wantSummary {
				t.Errorf("summary = %q, want %q", summary, tt.wantSummary)
			}
			if detail != tt.wantDetail {
				t.Errorf("detail = %q, want %q", detail, tt.wantDetail)
			}
		})
	}
}

func TestSummarizeItemTruncatesLongValues(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 300)

	got, _ := summarizeItem(streamItem{Type: "command_executed", Command: long})
	if len([]rune(got)) > len("Bash: ")+100 {
		t.Fatalf("summary is %d runes, want the command capped at 100", len([]rune(got)))
	}

	got, _ = summarizeItem(streamItem{Type: "agent_message", Text: long})
	if len([]rune(got)) > 100 {
		t.Fatalf("summary is %d runes, want the message capped at 100", len([]rune(got)))
	}

	got, _ = summarizeItem(streamItem{Type: "web_search", Query: long})
	if len([]rune(got)) > len("WebSearch: ")+80 {
		t.Fatalf("summary is %d runes, want the query capped at 80", len([]rune(got)))
	}
}

func TestShortPath(t *testing.T) {
	t.Parallel()

	if got := shortPath(""); got != "" {
		t.Fatalf("shortPath(\"\") = %q, want empty", got)
	}
	if got := shortPath("/etc/hosts"); got != "/etc/hosts" {
		t.Fatalf("shortPath = %q, want the path unchanged", got)
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory in this environment")
	}
	if got := shortPath(filepath.Join(home, "work", "x.go")); got != "~/work/x.go" {
		t.Fatalf("shortPath = %q, want it rooted at ~", got)
	}
}
