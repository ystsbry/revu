package github

import (
	"reflect"
	"testing"
)

func TestPRListArgs(t *testing.T) {
	jsonFields := "number,title,url,baseRefName,headRefName,author,updatedAt"

	got := prListArgs("o/r", "label:x")
	want := []string{
		"pr", "list", "--state", "open",
		"--repo", "o/r",
		"--search", "label:x",
		"--json", jsonFields, "--limit", "50",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}

	// Empty slug targets cwd (no --repo); empty search lists everything.
	got = prListArgs("", "")
	want = []string{"pr", "list", "--state", "open", "--json", jsonFields, "--limit", "50"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("bare args = %v, want %v", got, want)
	}
}

// The fake must satisfy the widened Client interface.
var _ Client = (*FakeClient)(nil)
