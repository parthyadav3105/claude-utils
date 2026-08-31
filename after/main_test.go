package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in      string
		kind    reqKind
		minutes int
		text    string
		cancel  int
		none    bool // passed through to the model untouched
		err     bool // recognised but unusable
	}{
		{in: "after 5m add tests for the parser", kind: reqSchedule, minutes: 5, text: "add tests for the parser"},
		{in: "after 20m rebase onto main", kind: reqSchedule, minutes: 20, text: "rebase onto main"},
		{in: "  after   90m   note the flaky test  ", kind: reqSchedule, minutes: 90, text: "note the flaky test"},
		{in: "AFTER 5m case does not matter", kind: reqSchedule, minutes: 5, text: "case does not matter"},
		{in: "after 0m right away", kind: reqSchedule, minutes: 0, text: "right away"},
		{in: "after idle write up what we learned", kind: reqSchedule, minutes: 0, text: "write up what we learned"},
		{in: "after --list", kind: reqList},
		{in: "after --cancel 3", kind: reqCancel, cancel: 3},

		// Ordinary English must reach the model untouched. These are the cases
		// that matter: the hook sees every prompt the user types.
		{in: "after you finish, run the tests", none: true},
		// The CLI rejects a bare /after as an unknown command before any hook
		// runs, so the slash form never reaches us.
		{in: "/after 5m leading slash", none: true},
		{in: "after 30 minutes of running this, check the log", none: true},
		{in: "rename the after helper", none: true},
		{in: "look at after/main.go", none: true},
		{in: "afterwards, commit", none: true},
		{in: "", none: true},
		{in: "after", none: true},
		{in: "after 5 things happened", none: true},
		{in: "after 5s too fine grained", none: true},
		{in: "after 2h no hours", none: true},

		// Recognised, but there is nothing to queue or nothing to act on.
		{in: "after 5m", err: true},
		{in: "after 5m    ", err: true},
		{in: "after idle", err: true},
		{in: "after --cancel", err: true},
		{in: "after --cancel banana", err: true},
		{in: "after 99999999m too far off", err: true},
	}

	for _, c := range cases {
		got, err := parse(c.in)
		switch {
		case c.none:
			if got != nil || err != nil {
				t.Errorf("parse(%q) = (%v, %v), want passthrough", c.in, got, err)
			}
		case c.err:
			if err == nil {
				t.Errorf("parse(%q) = (%v, nil), want an error", c.in, got)
			}
		default:
			if err != nil {
				t.Errorf("parse(%q) unexpected error: %v", c.in, err)
				continue
			}
			if got == nil {
				t.Errorf("parse(%q) = nil, want a request", c.in)
				continue
			}
			if got.kind != c.kind || got.minutes != c.minutes || got.text != c.text || got.cancelID != c.cancel {
				t.Errorf("parse(%q) = %+v, want kind=%v minutes=%d cancel=%d text=%q",
					c.in, *got, c.kind, c.minutes, c.cancel, c.text)
			}
		}
	}
}

func TestTakeDue(t *testing.T) {
	now := time.Now().Unix()
	q := &queue{Entries: []entry{
		{ID: 1, SessionID: "a", DueUnix: now - 60, Text: "overdue"},
		{ID: 2, SessionID: "a", DueUnix: now + 600, Text: "not yet"},
		{ID: 3, SessionID: "b", DueUnix: now - 60, Text: "someone else"},
	}}

	due := takeDue(q, "a", now)
	if len(due) != 1 || due[0].ID != 1 {
		t.Fatalf("takeDue = %+v, want just entry 1", due)
	}
	if len(q.Entries) != 2 {
		t.Fatalf("queue keeps %d entries, want 2 (the pending one and the other session's)", len(q.Entries))
	}
	for _, e := range q.Entries {
		if e.ID == 1 {
			t.Error("a delivered entry is still in the queue")
		}
	}
}

func TestQueueRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)

	path, err := queuePath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "after-queue.json"); path != want {
		t.Fatalf("queuePath = %q, want %q", path, want)
	}

	// A missing file is an empty queue, not an error.
	if got := runRequest(&request{kind: reqList}, "s1", path, false); got != "Nothing queued." {
		t.Errorf("empty listing = %q", got)
	}

	if got := runRequest(&request{kind: reqSchedule, minutes: 5, text: "add tests"}, "s1", path, false); !strings.Contains(got, "add tests") {
		t.Errorf("schedule said %q", got)
	}
	runRequest(&request{kind: reqSchedule, minutes: 0, text: "right away"}, "s1", path, false)

	if got := runRequest(&request{kind: reqList}, "s1", path, false); !strings.Contains(got, "2 queued") {
		t.Errorf("listing = %q, want 2 entries", got)
	}
	// One session cannot see or cancel another's messages.
	if got := runRequest(&request{kind: reqList}, "s2", path, false); got != "Nothing queued." {
		t.Errorf("other session sees %q", got)
	}
	if got := runRequest(&request{kind: reqCancel, cancelID: 1}, "s2", path, false); !strings.Contains(got, "No queued message") {
		t.Errorf("cross-session cancel = %q, want a refusal", got)
	}

	if got := runRequest(&request{kind: reqCancel, cancelID: 1}, "s1", path, false); !strings.Contains(got, "Cancelled 1") {
		t.Errorf("cancel = %q", got)
	}
	if got := runRequest(&request{kind: reqList}, "s1", path, false); !strings.Contains(got, "1 queued") {
		t.Errorf("after cancel: %q", got)
	}
}

func TestDispatch(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	path := filepath.Join(dir, "after-queue.json")

	// Scheduling blocks the prompt, and hides it.
	out := dispatch(hookInput{
		HookEventName: "UserPromptSubmit",
		SessionID:     "s1",
		Prompt:        "after 0m say pineapple",
	}, path)
	if out.Decision != "block" {
		t.Fatalf("schedule did not block: %+v", out)
	}
	if out.HookSpecific == nil || !out.HookSpecific.SuppressOriginalPrompt {
		t.Error("suppressOriginalPrompt not set, so the prompt would be echoed back")
	}

	// Stop delivers it.
	out = dispatch(hookInput{HookEventName: "Stop", SessionID: "s1"}, path)
	if out.Decision != "block" || !strings.Contains(out.Reason, "say pineapple") {
		t.Fatalf("Stop did not deliver: %+v", out)
	}
	// ...once. The queue is empty now.
	if out = dispatch(hookInput{HookEventName: "Stop", SessionID: "s1"}, path); out.Decision != "" {
		t.Errorf("Stop delivered twice: %+v", out)
	}

	// Re-entry after our own block must never block again.
	dispatch(hookInput{HookEventName: "UserPromptSubmit", SessionID: "s1", Prompt: "after 0m again"}, path)
	out = dispatch(hookInput{HookEventName: "Stop", SessionID: "s1", StopHookActive: true}, path)
	if out.Decision != "" {
		t.Errorf("blocked while stop_hook_active: %+v", out)
	}

	// A machine-authored turn is never treated as a command.
	out = dispatch(hookInput{
		HookEventName: "UserPromptSubmit",
		SessionID:     "s1",
		Source:        "schedule_wakeup",
		Prompt:        "after 5m this should be ignored",
	}, path)
	if out.Decision != "" {
		t.Errorf("acted on an injected turn: %+v", out)
	}

	// An ordinary prompt passes through untouched, even with something due --
	// it runs first, and Stop delivers afterwards.
	out = dispatch(hookInput{HookEventName: "UserPromptSubmit", SessionID: "s1", Prompt: "what does this do?"}, path)
	if out.Decision != "" || out.HookSpecific != nil {
		t.Fatalf("an ordinary prompt was not left alone: %+v", out)
	}
	// ...and the due message is still queued, not swallowed by that prompt.
	out = dispatch(hookInput{HookEventName: "Stop", SessionID: "s1"}, path)
	if out.Decision != "block" || !strings.Contains(out.Reason, "again") {
		t.Errorf("due message lost across an ordinary prompt: %+v", out)
	}

	// An event we do not handle is silence.
	if out = dispatch(hookInput{HookEventName: "PreToolUse", SessionID: "s1"}, path); out.Decision != "" || out.HookSpecific != nil {
		t.Errorf("PreToolUse produced %+v, want nothing", out)
	}
}

// The empty output must marshal to exactly {}, which the CLI reads as "no
// opinion", and it is what every fail-open path prints.
func TestEmptyOutputMarshalsToNothing(t *testing.T) {
	b, err := json.Marshal(hookOutput{})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "{}" {
		t.Errorf("empty hookOutput = %s, want {}", b)
	}
}

func TestCorruptQueueIsNotFatal(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	path := filepath.Join(dir, "after-queue.json")
	if err := os.WriteFile(path, []byte("this is not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	q, err := loadQueue(path)
	if err != nil {
		t.Fatalf("corrupt queue returned an error: %v", err)
	}
	if len(q.Entries) != 0 {
		t.Errorf("corrupt queue = %+v, want a fresh one", q)
	}
}

// Ids are per session and always the lowest free number, so they stay in single
// digits. Cancelling one never renumbers the others.
func TestIDsStaySmallAndPerSession(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	path := filepath.Join(dir, "after-queue.json")

	sched := func(session, text string) string {
		return runRequest(&request{kind: reqSchedule, minutes: 5, text: text}, session, path, false)
	}

	// Interleave two sessions. Each numbers from 1, unaffected by the other.
	sched("s1", "a1")
	sched("s2", "b1")
	sched("s1", "a2")
	sched("s2", "b2")

	ids := func(session string) []int {
		q, err := loadQueue(path)
		if err != nil {
			t.Fatal(err)
		}
		var got []int
		for _, e := range q.Entries {
			if e.SessionID == session {
				got = append(got, e.ID)
			}
		}
		return got
	}
	for _, session := range []string{"s1", "s2"} {
		got := ids(session)
		if len(got) != 2 || got[0] != 1 || got[1] != 2 {
			t.Errorf("%s ids = %v, want [1 2]", session, got)
		}
	}

	// Cancelling 1 must leave 2 as 2. Renumbering the survivors is what would
	// make a stale id silently cancel the wrong message.
	runRequest(&request{kind: reqCancel, cancelID: 1}, "s1", path, false)
	if got := ids("s1"); len(got) != 1 || got[0] != 2 {
		t.Fatalf("s1 ids after cancel = %v, want [2]", got)
	}
	// A new message fills the gap rather than climbing, so ids stay small.
	sched("s1", "a3")
	if got := ids("s1"); len(got) != 2 || got[0] != 2 || got[1] != 1 {
		t.Errorf("s1 ids after a new message = %v, want the freed 1 reused", got)
	}
	// The other session is untouched throughout.
	if got := ids("s2"); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("s2 ids = %v, want [1 2]", got)
	}

	// Churn must not inflate the numbers: a few items deep stays single digit.
	for i := 0; i < 30; i++ {
		sched("s3", "churn")
		runRequest(&request{kind: reqCancel, cancelID: 1}, "s3", path, false)
	}
	sched("s3", "still small")
	if got := ids("s3"); len(got) != 1 || got[0] != 1 {
		t.Errorf("s3 ids after 30 rounds of churn = %v, want [1]", got)
	}
}

// The cross-session listing is the only place to learn a session id, which
// --cancel --session needs, so it has to print them.
func TestCrossSessionListingNamesSessions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	path := filepath.Join(dir, "after-queue.json")

	runRequest(&request{kind: reqSchedule, minutes: 5, text: "mine"}, "session-aaa", path, false)
	runRequest(&request{kind: reqSchedule, minutes: 5, text: "theirs"}, "session-bbb", path, false)

	q, err := loadQueue(path)
	if err != nil {
		t.Fatal(err)
	}
	all := listing(q, "", true)
	for _, want := range []string{"session-aaa", "session-bbb", "mine", "theirs"} {
		if !strings.Contains(all, want) {
			t.Errorf("cross-session listing missing %q:\n%s", want, all)
		}
	}
	// A single session's listing stays uncluttered by ids it cannot use.
	one := listing(q, "session-aaa", false)
	if strings.Contains(one, "session-aaa") || strings.Contains(one, "theirs") {
		t.Errorf("single-session listing leaked other sessions:\n%s", one)
	}
}

func TestDeliverable(t *testing.T) {
	one := deliverable([]entry{{Text: "add tests"}})
	if !strings.Contains(one, "add tests") || !strings.Contains(one, "user queued this message") {
		t.Errorf("single delivery reads badly: %q", one)
	}
	many := deliverable([]entry{{Text: "add tests"}, {Text: "rebase"}})
	if !strings.Contains(many, "1. add tests") || !strings.Contains(many, "2. rebase") {
		t.Errorf("multiple delivery reads badly: %q", many)
	}
}
