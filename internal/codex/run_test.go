package codex

import (
	"strings"
	"testing"
)

func TestBuildExecArgsIncludesNetworkOverride(t *testing.T) {
	t.Parallel()
	args := buildExecArgs("$review-pr 42", "/work", "/home/u/.revu")

	if args[0] != "exec" {
		t.Fatalf("first arg = %q, want \"exec\" (subcommand must come first)", args[0])
	}
	if args[len(args)-1] != "$review-pr 42" {
		t.Fatalf("last arg = %q, want the prompt at the end (variadic-flag safety)", args[len(args)-1])
	}

	// The network-egress override is load-bearing: without it, the
	// review-pr skill cannot reach api.github.com via gh, and revu
	// review --codex fails with no useful diagnostic. Pin it.
	if !containsPair(args, "-c", "sandbox_workspace_write.network_access=true") {
		t.Errorf("missing -c sandbox_workspace_write.network_access=true override in args: %v", args)
	}

	// The reasoning-effort bump is the only thing standing between us
	// and codex's known "I'll narrow to 1 finding" shortcut on the
	// review-pr skill. If this drops out, codex review quality
	// silently collapses — pin it.
	if !containsPair(args, "-c", `model_reasoning_effort="high"`) {
		t.Errorf("missing -c model_reasoning_effort=\"high\" override in args: %v", args)
	}

	// Sandbox mode and writable_root extension are equally required.
	if !containsPair(args, "--sandbox", "workspace-write") {
		t.Errorf("missing --sandbox workspace-write in args: %v", args)
	}
	if !containsPair(args, "--add-dir", "/home/u/.revu") {
		t.Errorf("missing --add-dir /home/u/.revu in args: %v", args)
	}
	if !containsPair(args, "--cd", "/work") {
		t.Errorf("missing --cd /work in args: %v", args)
	}
	if !contains(args, "--json") {
		t.Errorf("missing --json in args: %v", args)
	}
}

func TestBuildExecArgsPromptStaysLast(t *testing.T) {
	t.Parallel()
	// Confirm that no variadic-style flag captures the prompt by
	// accident. --add-dir is the only variadic risk on `codex exec`;
	// keeping --json (a non-variadic boolean) between it and the prompt
	// terminates the capture cleanly.
	args := buildExecArgs("$review-pr 7 --focus security,perf", "/x", "/y")
	addDirIdx := indexOf(args, "--add-dir")
	jsonIdx := indexOf(args, "--json")
	if addDirIdx < 0 || jsonIdx < 0 {
		t.Fatalf("missing --add-dir or --json in args: %v", args)
	}
	if jsonIdx < addDirIdx {
		t.Fatalf("--json (%d) must come AFTER --add-dir (%d) to terminate its variadic capture: %v", jsonIdx, addDirIdx, args)
	}
	last := args[len(args)-1]
	if !strings.HasPrefix(last, "$review-pr") {
		t.Errorf("prompt should be the very last arg, got last=%q", last)
	}
}

func contains(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func containsPair(args []string, k, v string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == k && args[i+1] == v {
			return true
		}
	}
	return false
}

func indexOf(args []string, want string) int {
	for i, a := range args {
		if a == want {
			return i
		}
	}
	return -1
}
