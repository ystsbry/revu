package codex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// relayProgress reads JSONL events from r (codex's stdout when run with
// `codex exec --json`) and writes a compact human summary to w. Returns
// the thread_id observed on the `thread.started` event (empty string if
// the event was missing or unparseable) so callers can later resume the
// same conversation via `codex resume <id>`.
//
// Unknown / unparseable lines are silently ignored — codex may add new
// event types over time, and we'd rather miss a line than crash mid-run.
func relayProgress(r io.Reader, w io.Writer) (string, error) {
	sc := bufio.NewScanner(r)
	// Item payloads can embed large text blobs (e.g. agent_message bodies
	// or command stdout). Bump the scanner buffer well past the default
	// 64 KiB.
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var sessionID string
	for sc.Scan() {
		var ev streamEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			continue
		}
		if ev.Type == "thread.started" && ev.ThreadID != "" {
			sessionID = ev.ThreadID
		}
		renderEvent(w, ev)
	}
	return sessionID, sc.Err()
}

type streamEvent struct {
	Type     string      `json:"type"`
	ThreadID string      `json:"thread_id,omitempty"`
	Item     *streamItem `json:"item,omitempty"`
	Usage    *streamUsage `json:"usage,omitempty"`

	// Some codex builds attach a top-level `error` / `message` field on
	// the `error` event.
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

type streamItem struct {
	ID   string `json:"id,omitempty"`
	Type string `json:"type,omitempty"`

	// agent_message
	Text string `json:"text,omitempty"`

	// command_executed
	Command  string `json:"command,omitempty"`
	ExitCode *int   `json:"exit_code,omitempty"`

	// file_edit / file_change variants
	Path string `json:"path,omitempty"`

	// web_search and similar
	Query string `json:"query,omitempty"`
	URL   string `json:"url,omitempty"`

	// generic name field on tool-call style items
	Name string `json:"name,omitempty"`
}

type streamUsage struct {
	InputTokens          int `json:"input_tokens,omitempty"`
	CachedInputTokens    int `json:"cached_input_tokens,omitempty"`
	OutputTokens         int `json:"output_tokens,omitempty"`
	ReasoningOutputTokens int `json:"reasoning_output_tokens,omitempty"`
}

func renderEvent(w io.Writer, ev streamEvent) {
	switch ev.Type {
	case "thread.started":
		fmt.Fprintln(w, "  ▸ codex session ready")
	case "turn.started":
		// Intentionally quiet — turn boundaries usually correspond 1:1
		// with thread.started in single-turn runs and are noisy
		// otherwise.
	case "item.completed":
		if ev.Item == nil {
			return
		}
		summary, detail := summarizeItem(*ev.Item)
		if summary == "" {
			return
		}
		fmt.Fprintf(w, "  ▸ %s\n", summary)
		if detail != "" {
			fmt.Fprintf(w, "    └ %s\n", detail)
		}
	case "turn.completed":
		if ev.Usage == nil {
			fmt.Fprintln(w, "\n  ✓ done")
			return
		}
		fmt.Fprintf(w, "\n  ✓ done · in %d / cached %d / out %d tokens\n",
			ev.Usage.InputTokens, ev.Usage.CachedInputTokens, ev.Usage.OutputTokens)
	case "error":
		msg := ev.Error
		if msg == "" {
			msg = ev.Message
		}
		if msg == "" {
			msg = "(no message)"
		}
		fmt.Fprintf(w, "  ✗ codex error: %s\n", truncate(msg, 200))
	}
}

// summarizeItem returns (one-line summary, optional detail) for an
// item.completed payload. Falls back to "<type>" so the user still sees
// something happened when codex introduces a new item kind.
func summarizeItem(it streamItem) (string, string) {
	switch it.Type {
	case "agent_message":
		if s := firstLine(it.Text); s != "" {
			return truncate(s, 100), ""
		}
		return "", ""
	case "reasoning":
		// Reasoning text is usually internal monologue — keep it
		// quiet unless we're in --verbose territory.
		return "", ""
	case "command_executed":
		if it.Command == "" {
			return "Bash", ""
		}
		detail := ""
		if it.ExitCode != nil && *it.ExitCode != 0 {
			detail = fmt.Sprintf("exit %d", *it.ExitCode)
		}
		return "Bash: " + truncate(it.Command, 100), detail
	case "file_edit", "file_change", "patch_applied":
		return "Edit: " + shortPath(it.Path), ""
	case "file_read":
		return "Read: " + shortPath(it.Path), ""
	case "file_write":
		return "Write: " + shortPath(it.Path), ""
	case "web_search":
		return "WebSearch: " + truncate(it.Query, 80), ""
	case "web_fetch":
		return "WebFetch: " + it.URL, ""
	case "tool_call":
		name := it.Name
		if name == "" {
			name = "tool"
		}
		return "Tool: " + name, ""
	case "":
		return "", ""
	default:
		return it.Type, ""
	}
}

// shortPath replaces $HOME with ~ so the progress line stays scannable.
func shortPath(p string) string {
	if p == "" {
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
			return "~/" + rel
		}
	}
	return p
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
