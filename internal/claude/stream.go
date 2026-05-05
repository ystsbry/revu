package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// relayProgress reads stream-json events from r (claude's stdout when run
// with --output-format stream-json --verbose) and writes a compact human
// summary to w. Returns the session_id observed on the system/init event
// (empty string if the event was missing or unparseable) so callers can
// later resume the same conversation via `claude --resume <id>`.
//
// Unknown / unparseable lines are silently ignored — claude may add new
// event types over time, and we'd rather miss a line than crash mid-run.
func relayProgress(r io.Reader, w io.Writer) (string, error) {
	sc := bufio.NewScanner(r)
	// Stream-json lines can be large (full assistant messages embed
	// markdown bodies). Bump the scanner's max token size well past the
	// default 64 KiB.
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var sessionID string
	for sc.Scan() {
		var ev streamEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			continue
		}
		if ev.Type == "system" && ev.Subtype == "init" && ev.SessionID != "" {
			sessionID = ev.SessionID
		}
		renderEvent(w, ev)
	}
	return sessionID, sc.Err()
}

type streamEvent struct {
	Type      string         `json:"type"`
	Subtype   string         `json:"subtype,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	Message   *streamMessage `json:"message,omitempty"`

	// result fields (only set on type=="result")
	DurationMs   int     `json:"duration_ms,omitempty"`
	TotalCostUSD float64 `json:"total_cost_usd,omitempty"`
	NumTurns     int     `json:"num_turns,omitempty"`
	IsError      bool    `json:"is_error,omitempty"`
	Result       string  `json:"result,omitempty"`
}

type streamMessage struct {
	Content []streamBlock `json:"content"`
}

type streamBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

func renderEvent(w io.Writer, ev streamEvent) {
	switch ev.Type {
	case "system":
		if ev.Subtype == "init" {
			fmt.Fprintln(w, "  ▸ claude session ready")
		}
	case "assistant":
		if ev.Message == nil {
			return
		}
		for _, b := range ev.Message.Content {
			switch b.Type {
			case "tool_use":
				summary, detail := summarizeToolUse(b)
				if summary == "" {
					continue
				}
				fmt.Fprintf(w, "  ▸ %s\n", summary)
				if detail != "" {
					fmt.Fprintf(w, "    └ %s\n", detail)
				}
			case "text":
				if s := firstLine(b.Text); s != "" {
					fmt.Fprintf(w, "    %s\n", truncate(s, 100))
				}
			}
		}
	case "result":
		secs := ev.DurationMs / 1000
		marker := "✓"
		status := "done"
		if ev.IsError {
			marker = "✗"
			status = "failed"
		}
		fmt.Fprintf(w, "\n  %s %s in %ds · cost $%.4f · %d turns\n",
			marker, status, secs, ev.TotalCostUSD, ev.NumTurns)
	}
}

// summarizeToolUse returns (one-line summary, optional sub-line detail).
// Falls back to "<tool name>" when the input shape isn't recognised so
// the user still sees something happened.
func summarizeToolUse(b streamBlock) (string, string) {
	var in map[string]any
	if len(b.Input) > 0 {
		_ = json.Unmarshal(b.Input, &in)
	}
	getStr := func(k string) string {
		v, _ := in[k].(string)
		return v
	}

	switch b.Name {
	case "Bash":
		cmd := getStr("command")
		desc := getStr("description")
		return "Bash: " + truncate(cmd, 100), desc
	case "Read":
		return "Read: " + shortPath(getStr("file_path")), ""
	case "Write":
		return "Write: " + shortPath(getStr("file_path")), ""
	case "Edit":
		return "Edit: " + shortPath(getStr("file_path")), ""
	case "MultiEdit":
		return "MultiEdit: " + shortPath(getStr("file_path")), ""
	case "NotebookEdit":
		return "NotebookEdit: " + shortPath(getStr("notebook_path")), ""
	case "Glob":
		return "Glob: " + getStr("pattern"), getStr("path")
	case "Grep":
		return "Grep: " + getStr("pattern"), getStr("path")
	case "WebFetch":
		return "WebFetch: " + getStr("url"), ""
	case "WebSearch":
		return "WebSearch: " + truncate(getStr("query"), 80), ""
	case "Task", "Agent":
		desc := getStr("description")
		if desc == "" {
			desc = getStr("subagent_type")
		}
		return "Agent: " + desc, ""
	case "TodoWrite":
		return "TodoWrite", ""
	case "Skill":
		return "Skill: " + getStr("skill"), ""
	case "":
		return "", ""
	default:
		return b.Name, ""
	}
}

// shortPath replaces $HOME with ~ and trims excessively long paths, so
// the progress line stays scannable.
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
	// Operate on runes so we don't slice mid-codepoint on multibyte text.
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
