package main

import "testing"

func TestTUICmdIsRegistered(t *testing.T) {
	t.Parallel()
	for _, c := range newRootCmd().Commands() {
		if c.Name() == "tui" {
			return
		}
	}
	t.Errorf("`revu tui` is not registered on the root command")
}

// The dashboard is the one command that must work outside a git clone, so
// it takes no positional arguments to resolve.
func TestTUICmdRejectsArgs(t *testing.T) {
	t.Parallel()
	cmd := newTUICmd()
	if err := cmd.Args(cmd, []string{"unexpected"}); err == nil {
		t.Errorf("expected `revu tui <arg>` to be rejected")
	}
}
