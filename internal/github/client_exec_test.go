package github

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// call records one invocation of the stubbed runner.
type call struct {
	bin   string
	args  []string
	stdin []byte
}

// stub is an in-memory runner. Tests set the canned output and then assert
// on what the client asked gh to do.
type stub struct {
	stdout string
	stderr string
	err    error

	calls []call
}

func (s *stub) runner() runner {
	return func(_ context.Context, bin string, stdin []byte, args ...string) ([]byte, []byte, error) {
		s.calls = append(s.calls, call{bin: bin, args: args, stdin: stdin})
		return []byte(s.stdout), []byte(s.stderr), s.err
	}
}

// only returns the single call the test expects, failing otherwise.
func (s *stub) only(t *testing.T) call {
	t.Helper()
	if len(s.calls) != 1 {
		t.Fatalf("runner was called %d times, want exactly 1: %+v", len(s.calls), s.calls)
	}
	return s.calls[0]
}

func clientWith(s *stub) *GhClient {
	return &GhClient{run: s.runner()}
}

// argv is the joined argument list, for readable assertions on the whole
// command line.
func (c call) argv() string { return strings.Join(c.args, " ") }

// The default client must keep spawning the real gh; the injection point
// is opt-in and only tests set it.
func TestDefaultClientUsesTheRealExec(t *testing.T) {
	t.Parallel()
	if New().run != nil {
		t.Fatal("New() must not install a runner override")
	}
	if got := New().bin(); got != "gh" {
		t.Fatalf("bin() = %q, want gh", got)
	}
	if got := (&GhClient{Bin: "/usr/local/bin/gh"}).bin(); got != "/usr/local/bin/gh" {
		t.Fatalf("bin() = %q, want the configured path", got)
	}
}

func TestAuthStatus(t *testing.T) {
	t.Parallel()

	t.Run("authenticated", func(t *testing.T) {
		t.Parallel()
		s := &stub{}
		if err := clientWith(s).AuthStatus(context.Background()); err != nil {
			t.Fatalf("AuthStatus = %v, want nil", err)
		}
		if got := s.only(t).argv(); got != "auth status" {
			t.Fatalf("argv = %q, want %q", got, "auth status")
		}
	})

	// gh explains what is wrong on stderr ("You are not logged in..."), and
	// that message is the only actionable thing the user gets.
	t.Run("stderr is surfaced", func(t *testing.T) {
		t.Parallel()
		s := &stub{stderr: "You are not logged into any GitHub hosts\n", err: errors.New("exit status 1")}

		err := clientWith(s).AuthStatus(context.Background())
		if err == nil {
			t.Fatal("AuthStatus should fail")
		}
		if !strings.Contains(err.Error(), "not logged into") {
			t.Fatalf("error = %q, want gh's message", err)
		}
	})

	t.Run("silent failure falls back to the exit error", func(t *testing.T) {
		t.Parallel()
		s := &stub{err: errors.New("exit status 4")}

		err := clientWith(s).AuthStatus(context.Background())
		if err == nil || !strings.Contains(err.Error(), "exit status 4") {
			t.Fatalf("error = %v, want the exit status", err)
		}
	})
}

func TestPRHead(t *testing.T) {
	t.Parallel()

	t.Run("asks for headRefOid scoped to the repo", func(t *testing.T) {
		t.Parallel()
		s := &stub{stdout: `{"headRefOid":"abc1234567890"}`}

		got, err := clientWith(s).PRHead(context.Background(), "o/r", 42)
		if err != nil {
			t.Fatal(err)
		}
		if got != "abc1234567890" {
			t.Fatalf("PRHead = %q, want the SHA from the JSON", got)
		}
		if want := "pr view 42 --repo o/r --json headRefOid"; s.only(t).argv() != want {
			t.Fatalf("argv = %q, want %q", s.only(t).argv(), want)
		}
	})

	t.Run("non-zero exit wraps stderr", func(t *testing.T) {
		t.Parallel()
		s := &stub{stderr: "could not find pull request\n", err: errors.New("exit status 1")}

		_, err := clientWith(s).PRHead(context.Background(), "o/r", 42)
		if err == nil || !strings.Contains(err.Error(), "could not find pull request") {
			t.Fatalf("error = %v, want gh's stderr", err)
		}
	})

	t.Run("broken JSON is reported", func(t *testing.T) {
		t.Parallel()
		s := &stub{stdout: "{not json"}

		_, err := clientWith(s).PRHead(context.Background(), "o/r", 42)
		if err == nil || !strings.Contains(err.Error(), "parse") {
			t.Fatalf("error = %v, want a parse failure", err)
		}
	})

	// An empty SHA would silently produce a review pinned to nothing, so it
	// is rejected rather than passed on.
	t.Run("empty headRefOid is rejected", func(t *testing.T) {
		t.Parallel()
		s := &stub{stdout: `{"headRefOid":""}`}

		_, err := clientWith(s).PRHead(context.Background(), "o/r", 42)
		if err == nil || !strings.Contains(err.Error(), "empty headRefOid") {
			t.Fatalf("error = %v, want an empty-SHA complaint", err)
		}
	})
}

func TestPRState(t *testing.T) {
	t.Parallel()

	t.Run("returns the state", func(t *testing.T) {
		t.Parallel()
		s := &stub{stdout: `{"state":"MERGED"}`}

		got, err := clientWith(s).PRState(context.Background(), "o/r", 7)
		if err != nil {
			t.Fatal(err)
		}
		if got != PRStateMerged {
			t.Fatalf("PRState = %q, want MERGED", got)
		}
		if want := "pr view 7 --repo o/r --json state"; s.only(t).argv() != want {
			t.Fatalf("argv = %q, want %q", s.only(t).argv(), want)
		}
	})

	t.Run("failure names the PR", func(t *testing.T) {
		t.Parallel()
		s := &stub{stderr: "no such PR", err: errors.New("exit status 1")}

		_, err := clientWith(s).PRState(context.Background(), "o/r", 7)
		if err == nil || !strings.Contains(err.Error(), "7") {
			t.Fatalf("error = %v, want the PR number in it", err)
		}
	})

	t.Run("empty state is rejected", func(t *testing.T) {
		t.Parallel()
		s := &stub{stdout: `{"state":""}`}

		_, err := clientWith(s).PRState(context.Background(), "o/r", 7)
		if err == nil || !strings.Contains(err.Error(), "empty state") {
			t.Fatalf("error = %v, want an empty-state complaint", err)
		}
	})

	t.Run("broken JSON is reported", func(t *testing.T) {
		t.Parallel()
		s := &stub{stdout: "nope"}

		_, err := clientWith(s).PRState(context.Background(), "o/r", 7)
		if err == nil || !strings.Contains(err.Error(), "parse") {
			t.Fatalf("error = %v, want a parse failure", err)
		}
	})
}

func TestPRTitle(t *testing.T) {
	t.Parallel()

	t.Run("returns the title", func(t *testing.T) {
		t.Parallel()
		s := &stub{stdout: `{"title":"add dashboard"}`}

		got, err := clientWith(s).PRTitle(context.Background(), "o/r", 3)
		if err != nil {
			t.Fatal(err)
		}
		if got != "add dashboard" {
			t.Fatalf("PRTitle = %q", got)
		}
		if want := "pr view 3 --repo o/r --json title"; s.only(t).argv() != want {
			t.Fatalf("argv = %q, want %q", s.only(t).argv(), want)
		}
	})

	// An untitled PR is odd but not an error: the job list just shows less.
	t.Run("an empty title is not an error", func(t *testing.T) {
		t.Parallel()
		s := &stub{stdout: `{"title":""}`}

		got, err := clientWith(s).PRTitle(context.Background(), "o/r", 3)
		if err != nil || got != "" {
			t.Fatalf("PRTitle = (%q, %v), want an empty title and no error", got, err)
		}
	})

	t.Run("failure wraps stderr", func(t *testing.T) {
		t.Parallel()
		s := &stub{stderr: "boom", err: errors.New("exit status 1")}

		_, err := clientWith(s).PRTitle(context.Background(), "o/r", 3)
		if err == nil || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("error = %v, want gh's stderr", err)
		}
	})
}

func TestPRMeta(t *testing.T) {
	t.Parallel()

	// The base slug is derived from the PR url because gh pr view has no
	// baseRepository field — the one non-obvious thing this method does.
	t.Run("derives the base repo from the url", func(t *testing.T) {
		t.Parallel()
		s := &stub{stdout: `{
			"number": 12,
			"headRefOid": "abc1234",
			"baseRefName": "main",
			"title": "fix submit",
			"body": "why",
			"url": "https://github.com/owner/repo/pull/12"
		}`}

		got, err := clientWith(s).PRMeta(context.Background(), 12)
		if err != nil {
			t.Fatal(err)
		}
		want := PRMeta{
			Number: 12, HeadSha: "abc1234", BaseBranch: "main",
			Title: "fix submit", Body: "why", BaseRepo: "owner/repo",
		}
		if got != want {
			t.Fatalf("PRMeta = %+v, want %+v", got, want)
		}
	})

	// No --repo here: this one deliberately targets the cwd's repo.
	t.Run("asks for every field in one call", func(t *testing.T) {
		t.Parallel()
		s := &stub{stdout: `{"headRefOid":"a","url":"https://github.com/o/r/pull/1"}`}

		if _, err := clientWith(s).PRMeta(context.Background(), 1); err != nil {
			t.Fatal(err)
		}
		got := s.only(t).argv()
		if want := "pr view 1 --json number,headRefOid,baseRefName,title,body,url"; got != want {
			t.Fatalf("argv = %q, want %q", got, want)
		}
		if strings.Contains(got, "--repo") {
			t.Fatal("PRMeta targets the cwd's repo and must not pass --repo")
		}
	})

	t.Run("empty headRefOid is rejected", func(t *testing.T) {
		t.Parallel()
		s := &stub{stdout: `{"url":"https://github.com/o/r/pull/1"}`}

		_, err := clientWith(s).PRMeta(context.Background(), 1)
		if err == nil || !strings.Contains(err.Error(), "empty headRefOid") {
			t.Fatalf("error = %v, want an empty-SHA complaint", err)
		}
	})

	t.Run("an unusable url is reported", func(t *testing.T) {
		t.Parallel()
		s := &stub{stdout: `{"headRefOid":"a","url":"https://github.com/o/r"}`}

		_, err := clientWith(s).PRMeta(context.Background(), 1)
		if err == nil || !strings.Contains(err.Error(), "unexpected PR url") {
			t.Fatalf("error = %v, want a url complaint", err)
		}
	})

	t.Run("broken JSON is reported", func(t *testing.T) {
		t.Parallel()
		s := &stub{stdout: "<html>"}

		_, err := clientWith(s).PRMeta(context.Background(), 1)
		if err == nil || !strings.Contains(err.Error(), "parse") {
			t.Fatalf("error = %v, want a parse failure", err)
		}
	})

	t.Run("failure wraps stderr", func(t *testing.T) {
		t.Parallel()
		s := &stub{stderr: "not found", err: errors.New("exit status 1")}

		_, err := clientWith(s).PRMeta(context.Background(), 1)
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("error = %v, want gh's stderr", err)
		}
	})
}

func TestPRDiff(t *testing.T) {
	t.Parallel()

	t.Run("returns stdout verbatim", func(t *testing.T) {
		t.Parallel()
		diff := "diff --git a/x b/x\n@@ -1 +1 @@\n-old\n+new\n"
		s := &stub{stdout: diff}

		got, err := clientWith(s).PRDiff(context.Background(), 5)
		if err != nil {
			t.Fatal(err)
		}
		if got != diff {
			t.Fatalf("PRDiff = %q, want the diff unchanged", got)
		}
		if want := "pr diff 5"; s.only(t).argv() != want {
			t.Fatalf("argv = %q, want %q", s.only(t).argv(), want)
		}
	})

	t.Run("failure wraps stderr", func(t *testing.T) {
		t.Parallel()
		s := &stub{stderr: "no diff", err: errors.New("exit status 1")}

		_, err := clientWith(s).PRDiff(context.Background(), 5)
		if err == nil || !strings.Contains(err.Error(), "no diff") {
			t.Fatalf("error = %v, want gh's stderr", err)
		}
	})
}

func TestPostReview(t *testing.T) {
	t.Parallel()

	t.Run("posts the payload and returns the review id", func(t *testing.T) {
		t.Parallel()
		s := &stub{stdout: `{"id":987654}`}
		p := Payload{Body: "looks good", Event: "APPROVE"}

		id, err := clientWith(s).PostReview(context.Background(), "o/r", 9, p)
		if err != nil {
			t.Fatal(err)
		}
		if id != 987654 {
			t.Fatalf("review id = %d, want 987654", id)
		}

		c := s.only(t)
		if want := "api -X POST /repos/o/r/pulls/9/reviews --input -"; c.argv() != want {
			t.Fatalf("argv = %q, want %q", c.argv(), want)
		}
		// The payload goes in on stdin, not as an argument.
		if !strings.Contains(string(c.stdin), `"body":"looks good"`) {
			t.Fatalf("stdin = %s, want the marshalled payload", c.stdin)
		}
		if !strings.Contains(string(c.stdin), `"event":"APPROVE"`) {
			t.Fatalf("stdin = %s, want the review event", c.stdin)
		}
	})

	// gh puts the short line on stderr but the actionable GitHub message
	// (errors[].message) on stdout, so both have to reach the user.
	t.Run("failure carries the response body", func(t *testing.T) {
		t.Parallel()
		s := &stub{
			stdout: `{"message":"Validation Failed","errors":[{"message":"line must be part of the diff"}]}`,
			stderr: "gh: Validation Failed (HTTP 422)",
			err:    errors.New("exit status 1"),
		}

		_, err := clientWith(s).PostReview(context.Background(), "o/r", 9, Payload{})
		if err == nil {
			t.Fatal("PostReview should fail")
		}
		if !strings.Contains(err.Error(), "HTTP 422") {
			t.Errorf("error = %q, want gh's stderr line", err)
		}
		if !strings.Contains(err.Error(), "line must be part of the diff") {
			t.Errorf("error = %q, want the GitHub message from stdout", err)
		}
	})

	t.Run("failure without a body falls back to stderr", func(t *testing.T) {
		t.Parallel()
		s := &stub{stderr: "connection refused", err: errors.New("exit status 1")}

		_, err := clientWith(s).PostReview(context.Background(), "o/r", 9, Payload{})
		if err == nil || !strings.Contains(err.Error(), "connection refused") {
			t.Fatalf("error = %v, want gh's stderr", err)
		}
	})

	// A 200 with no id means the review was not created; reporting success
	// would record a submitted_at for something that does not exist.
	t.Run("a response without an id is an error", func(t *testing.T) {
		t.Parallel()
		s := &stub{stdout: `{"state":"PENDING"}`}

		_, err := clientWith(s).PostReview(context.Background(), "o/r", 9, Payload{})
		if err == nil || !strings.Contains(err.Error(), "no review id") {
			t.Fatalf("error = %v, want a missing-id complaint", err)
		}
	})

	t.Run("broken JSON is reported", func(t *testing.T) {
		t.Parallel()
		s := &stub{stdout: "not json"}

		_, err := clientWith(s).PostReview(context.Background(), "o/r", 9, Payload{})
		if err == nil || !strings.Contains(err.Error(), "parse review response") {
			t.Fatalf("error = %v, want a parse failure", err)
		}
	})
}

func TestListPRs(t *testing.T) {
	t.Parallel()

	t.Run("parses the list", func(t *testing.T) {
		t.Parallel()
		s := &stub{stdout: `[
			{"number":1,"title":"one","url":"u1","baseRefName":"main","headRefName":"f1","author":{"login":"alice"},"updatedAt":"2026-08-01T00:00:00Z"},
			{"number":2,"title":"two","url":"u2","baseRefName":"main","headRefName":"f2","author":{"login":"bob"},"updatedAt":"2026-08-02T00:00:00Z"}
		]`}

		items, err := clientWith(s).ListPRs(context.Background(), "o/r", "review-requested:@me")
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 2 {
			t.Fatalf("got %d items, want 2", len(items))
		}
		if items[0].Number != 1 || items[0].Author.Login != "alice" {
			t.Fatalf("first item = %+v", items[0])
		}
		if want := strings.Join(prListArgs("o/r", "review-requested:@me"), " "); s.only(t).argv() != want {
			t.Fatalf("argv = %q, want %q", s.only(t).argv(), want)
		}
	})

	t.Run("no matches is an empty list, not an error", func(t *testing.T) {
		t.Parallel()
		s := &stub{stdout: `[]`}

		items, err := clientWith(s).ListPRs(context.Background(), "o/r", "")
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 0 {
			t.Fatalf("got %d items, want none", len(items))
		}
	})

	t.Run("failure wraps stderr", func(t *testing.T) {
		t.Parallel()
		s := &stub{stderr: "unknown repo", err: errors.New("exit status 1")}

		_, err := clientWith(s).ListPRs(context.Background(), "o/r", "")
		if err == nil || !strings.Contains(err.Error(), "unknown repo") {
			t.Fatalf("error = %v, want gh's stderr", err)
		}
	})

	t.Run("broken JSON is reported", func(t *testing.T) {
		t.Parallel()
		s := &stub{stdout: "{"}

		_, err := clientWith(s).ListPRs(context.Background(), "o/r", "")
		if err == nil || !strings.Contains(err.Error(), "parse") {
			t.Fatalf("error = %v, want a parse failure", err)
		}
	})
}

// ListReviewRequestedPRs is a thin alias, so what matters is that it keeps
// passing the default search and no repo.
func TestListReviewRequestedPRs(t *testing.T) {
	t.Parallel()
	s := &stub{stdout: `[]`}

	if _, err := clientWith(s).ListReviewRequestedPRs(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := s.only(t).argv()
	if !strings.Contains(got, "--search "+DefaultPRSearch) {
		t.Fatalf("argv = %q, want the default search", got)
	}
	if strings.Contains(got, "--repo") {
		t.Fatal("the cwd's repo is the target; --repo must not appear")
	}
}

// execRunner is the path production takes. These tests use small POSIX
// tools rather than gh so they can assert the wiring — argv, stdin,
// stdout/stderr separation, exit status — without a GitHub account.
// (revu ships for linux/darwin only, so assuming echo/cat/sh is safe.)
func TestExecRunner(t *testing.T) {
	t.Parallel()

	t.Run("returns stdout", func(t *testing.T) {
		t.Parallel()
		stdout, stderr, err := execRunner(context.Background(), "echo", nil, "hello")
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(stdout)) != "hello" {
			t.Fatalf("stdout = %q, want hello", stdout)
		}
		if len(stderr) != 0 {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
	})

	t.Run("feeds stdin", func(t *testing.T) {
		t.Parallel()
		stdout, _, err := execRunner(context.Background(), "cat", []byte("piped"))
		if err != nil {
			t.Fatal(err)
		}
		if string(stdout) != "piped" {
			t.Fatalf("stdout = %q, want the stdin echoed back", stdout)
		}
	})

	t.Run("keeps stderr separate and reports the exit status", func(t *testing.T) {
		t.Parallel()
		stdout, stderr, err := execRunner(context.Background(), "sh", nil,
			"-c", "echo out; echo err >&2; exit 3")
		if err == nil {
			t.Fatal("a non-zero exit should be an error")
		}
		if strings.TrimSpace(string(stdout)) != "out" {
			t.Fatalf("stdout = %q, want out", stdout)
		}
		if strings.TrimSpace(string(stderr)) != "err" {
			t.Fatalf("stderr = %q, want err", stderr)
		}
	})

	t.Run("a missing binary is an error", func(t *testing.T) {
		t.Parallel()
		if _, _, err := execRunner(context.Background(), "revu-no-such-binary", nil); err == nil {
			t.Fatal("a missing binary should fail")
		}
	})
}

// The default client (no runner override) must really spawn the process it
// is pointed at — this is the wiring New() relies on.
func TestDefaultClientSpawnsTheConfiguredBinary(t *testing.T) {
	t.Parallel()
	c := &GhClient{Bin: "echo"}

	stdout, _, err := c.exec(context.Background(), nil, "auth", "status")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(stdout)); got != "auth status" {
		t.Fatalf("stdout = %q, want the argv echoed back", got)
	}
}
