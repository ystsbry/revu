package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every tool Claude Code can report gets its own progress line. A missing
// case would silently degrade to the bare tool name, so each shape is
// pinned here.
func TestSummarizeToolUsePerTool(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		block       streamBlock
		wantSummary string
		wantDetail  string
	}{
		{
			name:        "Bash shows the command and its description",
			block:       streamBlock{Name: "Bash", Input: []byte(`{"command":"go test ./...","description":"run tests"}`)},
			wantSummary: "Bash: go test ./...",
			wantDetail:  "run tests",
		},
		{
			name:        "Read",
			block:       streamBlock{Name: "Read", Input: []byte(`{"file_path":"/tmp/x.go"}`)},
			wantSummary: "Read: /tmp/x.go",
		},
		{
			name:        "Write",
			block:       streamBlock{Name: "Write", Input: []byte(`{"file_path":"/tmp/x.go"}`)},
			wantSummary: "Write: /tmp/x.go",
		},
		{
			name:        "Edit",
			block:       streamBlock{Name: "Edit", Input: []byte(`{"file_path":"/tmp/x.go"}`)},
			wantSummary: "Edit: /tmp/x.go",
		},
		{
			name:        "MultiEdit",
			block:       streamBlock{Name: "MultiEdit", Input: []byte(`{"file_path":"/tmp/x.go"}`)},
			wantSummary: "MultiEdit: /tmp/x.go",
		},
		{
			name:        "NotebookEdit uses notebook_path",
			block:       streamBlock{Name: "NotebookEdit", Input: []byte(`{"notebook_path":"/tmp/n.ipynb"}`)},
			wantSummary: "NotebookEdit: /tmp/n.ipynb",
		},
		{
			name:        "Grep carries the search path as detail",
			block:       streamBlock{Name: "Grep", Input: []byte(`{"pattern":"TODO","path":"internal"}`)},
			wantSummary: "Grep: TODO",
			wantDetail:  "internal",
		},
		{
			name:        "WebFetch",
			block:       streamBlock{Name: "WebFetch", Input: []byte(`{"url":"https://example.com"}`)},
			wantSummary: "WebFetch: https://example.com",
		},
		{
			name:        "WebSearch",
			block:       streamBlock{Name: "WebSearch", Input: []byte(`{"query":"golang testing"}`)},
			wantSummary: "WebSearch: golang testing",
		},
		{
			name:        "Task prefers the description",
			block:       streamBlock{Name: "Task", Input: []byte(`{"description":"explore","subagent_type":"Explore"}`)},
			wantSummary: "Agent: explore",
		},
		{
			name:        "Task falls back to the subagent type",
			block:       streamBlock{Name: "Task", Input: []byte(`{"subagent_type":"Explore"}`)},
			wantSummary: "Agent: Explore",
		},
		{
			name:        "Agent is an alias of Task",
			block:       streamBlock{Name: "Agent", Input: []byte(`{"description":"explore"}`)},
			wantSummary: "Agent: explore",
		},
		{
			name:        "TodoWrite has no arguments worth showing",
			block:       streamBlock{Name: "TodoWrite", Input: []byte(`{"todos":[]}`)},
			wantSummary: "TodoWrite",
		},
		{
			name:        "Skill",
			block:       streamBlock{Name: "Skill", Input: []byte(`{"skill":"revu:pr"}`)},
			wantSummary: "Skill: revu:pr",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			summary, detail := summarizeToolUse(tt.block)
			if summary != tt.wantSummary {
				t.Errorf("summary = %q, want %q", summary, tt.wantSummary)
			}
			if detail != tt.wantDetail {
				t.Errorf("detail = %q, want %q", detail, tt.wantDetail)
			}
		})
	}
}

// Malformed or absent input must not lose the fact that a tool ran.
func TestSummarizeToolUseTolerantOfBadInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		block streamBlock
		want  string
	}{
		{name: "no input at all", block: streamBlock{Name: "Read"}, want: "Read: "},
		{name: "input is not an object", block: streamBlock{Name: "Read", Input: []byte(`"nope"`)}, want: "Read: "},
		{name: "field has the wrong type", block: streamBlock{Name: "Read", Input: []byte(`{"file_path":42}`)}, want: "Read: "},
		{name: "broken JSON", block: streamBlock{Name: "Bash", Input: []byte(`{"command":`)}, want: "Bash: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got, _ := summarizeToolUse(tt.block); got != tt.want {
				t.Fatalf("summary = %q, want %q", got, tt.want)
			}
		})
	}
}

// A long command would push the rest of the progress line off screen.
func TestSummarizeToolUseTruncatesLongValues(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 300)

	got, _ := summarizeToolUse(streamBlock{Name: "Bash", Input: []byte(`{"command":"` + long + `"}`)})
	if len([]rune(got)) > len("Bash: ")+100 {
		t.Fatalf("summary is %d runes, want the command capped at 100", len([]rune(got)))
	}

	got, _ = summarizeToolUse(streamBlock{Name: "WebSearch", Input: []byte(`{"query":"` + long + `"}`)})
	if len([]rune(got)) > len("WebSearch: ")+80 {
		t.Fatalf("summary is %d runes, want the query capped at 80", len([]rune(got)))
	}
}

func TestShortPath(t *testing.T) {
	t.Parallel()

	if got := shortPath(""); got != "" {
		t.Fatalf("shortPath(\"\") = %q, want empty", got)
	}

	// A path outside $HOME is shown as-is.
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
