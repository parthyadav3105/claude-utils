// Command claudeafter queues a message for a Claude Code session instead of
// sending it now.
//
// On UserPromptSubmit it recognises `after 5m <message>`, files it away and
// blocks the prompt, so the text never reaches the model. On Stop it hands back
// whatever has come due, which the CLI feeds to the model as a continuation.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var version = "dev"

// Past a week a delay is likelier to be a typo than an intention, and the
// session it belongs to is long gone.
const maxMinutes = 7 * 24 * 60

// hookInput is the JSON Claude Code writes to our stdin. Only the fields we act
// on are listed.
type hookInput struct {
	HookEventName string `json:"hook_event_name"`
	SessionID     string `json:"session_id"`
	Prompt        string `json:"prompt"`
	// Absent for ordinary interactive prompts, so empty means a real user typed it.
	Source string `json:"source"`
	// True when we are being re-entered after our own block. Absent rather than
	// false on the first Stop, which the zero value handles.
	StopHookActive bool `json:"stop_hook_active"`
}

// Turns the machine authored. Reading our own delivery back as a fresh command
// would loop.
var injectedSources = map[string]bool{
	"system":          true,
	"loop_wakeup":     true,
	"schedule_wakeup": true,
	"poll_event":      true,
}

type hookSpecific struct {
	HookEventName string `json:"hookEventName"`
	// Hides the blocked text from the CLI's own output. Only takes effect nested
	// in here; set at the top level it is silently ignored.
	SuppressOriginalPrompt bool `json:"suppressOriginalPrompt,omitempty"`
}

type hookOutput struct {
	Decision     string        `json:"decision,omitempty"`
	Reason       string        `json:"reason,omitempty"`
	HookSpecific *hookSpecific `json:"hookSpecificOutput,omitempty"`
}

type entry struct {
	ID          int    `json:"id"`
	SessionID   string `json:"session_id"`
	DueUnix     int64  `json:"due_unix"`
	CreatedUnix int64  `json:"created_unix"`
	Text        string `json:"text"`
}

type queue struct {
	Entries []entry `json:"entries"`
}

// nextID takes the lowest number this session is not already using. A queue is
// only ever a few items deep, so ids you have to type stay in single digits
// instead of climbing forever, and each session numbers its own from 1.
//
// Only new messages fill a gap. The ids already in the queue never change, so
// cancelling by an id you can still see always hits what you meant.
func nextID(q *queue, session string) int {
	used := map[int]bool{}
	for _, e := range q.Entries {
		if e.SessionID == session {
			used[e.ID] = true
		}
	}
	for i := 1; ; i++ {
		if !used[i] {
			return i
		}
	}
}

// ---------------------------------------------------------------- parsing

type reqKind int

const (
	reqSchedule reqKind = iota
	reqList
	reqCancel
)

type request struct {
	kind     reqKind
	minutes  int
	cancelID int
	text     string
}

var (
	trigger  = regexp.MustCompile(`(?is)^\s*after\s+(.*)$`)
	duration = regexp.MustCompile(`^(\d+)m$`)
)

// parse returns (nil, nil) for anything that is not ours, which is the case
// worth being careful about: this sees every prompt the user types. `after` must
// be followed by a bare <N>m or a keyword, so ordinary English is left alone.
func parse(prompt string) (*request, error) {
	m := trigger.FindStringSubmatch(prompt)
	if m == nil {
		return nil, nil
	}
	tok, rest := cutToken(strings.TrimSpace(m[1]))

	switch {
	case strings.EqualFold(tok, "--list"):
		return &request{kind: reqList}, nil

	case strings.EqualFold(tok, "--cancel"):
		id, _ := cutToken(rest)
		if id == "" {
			return nil, errors.New("which one? try: after --cancel 3   (after --list shows the ids)")
		}
		n, err := strconv.Atoi(id)
		if err != nil {
			return nil, fmt.Errorf("%q is not an id, and after --list shows them", id)
		}
		return &request{kind: reqCancel, cancelID: n}, nil

	case strings.EqualFold(tok, "idle"):
		return scheduleReq(0, rest)

	default:
		d := duration.FindStringSubmatch(tok)
		if d == nil {
			return nil, nil // not ours; hand it to the model untouched
		}
		n, err := strconv.Atoi(d[1])
		if err != nil || n > maxMinutes {
			return nil, fmt.Errorf("%s is too far off, the longest delay is %dm (a week)", tok, maxMinutes)
		}
		return scheduleReq(n, rest)
	}
}

func scheduleReq(minutes int, text string) (*request, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("nothing to queue, try: after 5m go back and add tests")
	}
	return &request{kind: reqSchedule, minutes: minutes, text: text}, nil
}

func cutToken(s string) (tok, rest string) {
	s = strings.TrimLeft(s, " \t")
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimLeft(s[i:], " \t")
}

// ---------------------------------------------------------------- queue file

// queuePath follows CLAUDE_CONFIG_DIR at run time, not the directory the
// installer wrote to: the Profile Manager points it at a different config per
// directory, and a queued message belongs to the profile it was made under.
func queuePath() (string, error) {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch {
	case dir == "":
		dir = filepath.Join(home, ".claude")
	case dir == "~" || strings.HasPrefix(dir, "~/"):
		// a quoted CLAUDE_CONFIG_DIR reaches us with a literal ~
		dir = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(dir, "~"), "/"))
	}
	return filepath.Join(dir, "after-queue.json"), nil
}

func loadQueue(path string) (*queue, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &queue{}, nil
	}
	if err != nil {
		return nil, err
	}
	q := &queue{}
	if err := json.Unmarshal(b, q); err != nil {
		// A corrupt queue must not wedge every session on the machine. Losing
		// queued messages is a nuisance; refusing to run is not.
		return &queue{}, nil
	}
	return q, nil
}

// saveQueue writes through a temp file so a reader never sees a half-written
// queue, and drops entries old enough that their session is certainly gone.
func saveQueue(path string, q *queue) error {
	cutoff := time.Now().Add(-7 * 24 * time.Hour).Unix()
	kept := q.Entries[:0]
	for _, e := range q.Entries {
		if e.CreatedUnix >= cutoff {
			kept = append(kept, e)
		}
	}
	q.Entries = kept

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(q, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".after-queue-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// withQueue serialises read-modify-write across sessions. Every session on the
// machine runs this hook, so two finishing a turn at once is ordinary.
func withQueue(path string, fn func(*queue) error) error {
	lock := path + ".lock"
	deadline := time.Now().Add(2 * time.Second)
	for {
		f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			f.Close()
			break
		}
		// Break a lock left behind by a process that died holding it.
		if fi, statErr := os.Stat(lock); statErr == nil && time.Since(fi.ModTime()) > 30*time.Second {
			os.Remove(lock)
			continue
		}
		if time.Now().After(deadline) {
			return errors.New("queue is locked")
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer os.Remove(lock)

	q, err := loadQueue(path)
	if err != nil {
		return err
	}
	if err := fn(q); err != nil {
		return err
	}
	return saveQueue(path, q)
}

// takeDue removes and returns everything owed to a session, soonest first.
func takeDue(q *queue, session string, now int64) []entry {
	var due, rest []entry
	for _, e := range q.Entries {
		if e.SessionID == session && e.DueUnix <= now {
			due = append(due, e)
		} else {
			rest = append(rest, e)
		}
	}
	q.Entries = rest
	sort.Slice(due, func(i, j int) bool { return due[i].DueUnix < due[j].DueUnix })
	return due
}

// ---------------------------------------------------------------- requests

// deliverable phrases queued text for the model. It has to read as the user
// talking, not the system inventing work.
func deliverable(due []entry) string {
	var b strings.Builder
	if len(due) == 1 {
		b.WriteString("The user queued this message earlier, to reach you once you were free. ")
		b.WriteString("Treat it as their next instruction:\n\n")
		b.WriteString(due[0].Text)
		return b.String()
	}
	fmt.Fprintf(&b, "The user queued %d messages earlier, to reach you once you were free. ", len(due))
	b.WriteString("Treat them as their next instructions:\n\n")
	for i, e := range due {
		fmt.Fprintf(&b, "%d. %s\n", i+1, e.Text)
	}
	return strings.TrimRight(b.String(), "\n")
}

// listing shows one session's queue, or every session's when all is set, which
// only the command line asks for. Ids are only unique within a session, so the
// cross-session view has to name the session each one belongs to: it is the
// only place to get the id that --cancel --session needs.
func listing(q *queue, session string, all bool) string {
	now := time.Now()

	// Reusing a freed id means the queue is not in id order, so sort for
	// display: the numbers are what the reader is about to type.
	shown := make([]entry, 0, len(q.Entries))
	for _, e := range q.Entries {
		if all || e.SessionID == session {
			shown = append(shown, e)
		}
	}
	sort.Slice(shown, func(i, j int) bool {
		if shown[i].SessionID != shown[j].SessionID {
			return shown[i].SessionID < shown[j].SessionID
		}
		return shown[i].ID < shown[j].ID
	})

	var lines []string
	for _, e := range shown {
		when := "due now"
		if due := time.Unix(e.DueUnix, 0); due.After(now) {
			when = due.Format("15:04")
		}
		if all {
			lines = append(lines, fmt.Sprintf("  %s  %d  %-8s  %s", e.SessionID, e.ID, when, e.Text))
			continue
		}
		lines = append(lines, fmt.Sprintf("  %d  %-8s  %s", e.ID, when, e.Text))
	}
	if len(lines) == 0 {
		return "Nothing queued."
	}
	return fmt.Sprintf("%d queued:\n%s", len(lines), strings.Join(lines, "\n"))
}

func runRequest(req *request, session, path string, all bool) string {
	var msg string
	err := withQueue(path, func(q *queue) error {
		switch req.kind {
		case reqList:
			msg = listing(q, session, all)

		case reqCancel:
			for i, e := range q.Entries {
				if e.ID == req.cancelID && e.SessionID == session {
					q.Entries = append(q.Entries[:i], q.Entries[i+1:]...)
					msg = fmt.Sprintf("Cancelled %d: %s", e.ID, e.Text)
					return nil
				}
			}
			msg = fmt.Sprintf("No queued message with id %d.", req.cancelID)

		case reqSchedule:
			now := time.Now()
			due := now.Add(time.Duration(req.minutes) * time.Minute)
			q.Entries = append(q.Entries, entry{
				ID:          nextID(q, session),
				SessionID:   session,
				DueUnix:     due.Unix(),
				CreatedUnix: now.Unix(),
				Text:        req.text,
			})
			when := "the next time you stop"
			if req.minutes > 0 {
				when = fmt.Sprintf("%s (in %dm)", due.Format("15:04"), req.minutes)
			}
			msg = fmt.Sprintf("Queued for %s: %s", when, req.text)
		}
		return nil
	})
	if err != nil {
		return "after: " + err.Error()
	}
	return msg
}

// ---------------------------------------------------------------- hooks

func handleUserPromptSubmit(in hookInput, path string) hookOutput {
	if injectedSources[in.Source] {
		return hookOutput{}
	}

	// A recognised command, well formed or not, is answered here and never sent
	// on. Blocking is what keeps it out of the thread.
	req, err := parse(in.Prompt)
	if err != nil {
		return blocked(err.Error())
	}
	if req != nil {
		return blocked(runRequest(req, in.SessionID, path, false))
	}

	// Not ours, so hand it to the model untouched. Nothing is delivered here
	// even when due: the user's prompt runs first and Stop carries the queued
	// message afterwards, rather than derailing what they just asked for.
	return hookOutput{}
}

func handleStop(in hookInput, path string) hookOutput {
	// Re-entered after our own block. Blocking again is how a session gets stuck.
	if in.StopHookActive {
		return hookOutput{}
	}
	var due []entry
	if err := withQueue(path, func(q *queue) error {
		due = takeDue(q, in.SessionID, time.Now().Unix())
		return nil
	}); err != nil || len(due) == 0 {
		return hookOutput{}
	}
	// One block carrying every due message, never one block each: the CLI caps
	// consecutive stop-hook blocks at 8 and overrides us past that.
	return hookOutput{Decision: "block", Reason: deliverable(due)}
}

func blocked(reason string) hookOutput {
	return hookOutput{
		Decision: "block",
		Reason:   reason,
		HookSpecific: &hookSpecific{
			HookEventName:          "UserPromptSubmit",
			SuppressOriginalPrompt: true,
		},
	}
}

func dispatch(in hookInput, path string) hookOutput {
	switch in.HookEventName {
	case "UserPromptSubmit":
		return handleUserPromptSubmit(in, path)
	case "Stop":
		return handleStop(in, path)
	default:
		return hookOutput{}
	}
}

// ---------------------------------------------------------------- entry point

func main() {
	// Fail open, without exception. This runs on every prompt and every turn end
	// in every session, so a panic here is a panic in all of them.
	defer func() {
		if recover() != nil {
			fmt.Println("{}")
		}
	}()

	if len(os.Args) > 1 {
		os.Exit(cli(os.Args[1:]))
	}

	out := hookOutput{}
	if path, err := queuePath(); err == nil {
		var in hookInput
		if raw, err := io.ReadAll(os.Stdin); err == nil && json.Unmarshal(raw, &in) == nil {
			out = dispatch(in, path)
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		fmt.Println("{}")
		return
	}
	fmt.Println(string(b))
}

// cli is the same queue from another shell, the way to reach a session you
// cannot type into, including one suspended by the shell where no hook runs.
func cli(args []string) int {
	switch args[0] {
	case "-h", "--help":
		fmt.Print(usage)
		return 0
	case "-v", "--version":
		fmt.Println(version)
		return 0
	}

	// Pull --session out wherever it appears; what remains is the command.
	session := ""
	var rest []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--session" && i+1 < len(args) {
			session = args[i+1]
			i++
			continue
		}
		rest = append(rest, args[i])
	}

	path, err := queuePath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "after:", err)
		return 1
	}
	req, perr := parse("after " + strings.Join(rest, " "))
	if perr != nil {
		fmt.Fprintln(os.Stderr, "after:", perr)
		return 1
	}
	if req == nil {
		fmt.Print(usage)
		return 1
	}
	if req.kind != reqList && session == "" {
		fmt.Fprintln(os.Stderr, "after: --session <id> is required from the command line")
		fmt.Fprintln(os.Stderr, "       claudeafter --list  shows the queue and its session ids")
		return 1
	}
	// With no --session, listing covers every session rather than none.
	fmt.Println(runRequest(req, session, path, session == ""))
	return 0
}

const usage = `claudeafter: queue a message for a Claude Code session instead of interrupting it.

Normally you never run this. It is installed as a hook, and you type
"after 5m <message>" straight into Claude Code.

From another shell, to reach a session you cannot type into:

  claudeafter 5m "rebase onto main" --session <id>
  claudeafter --cancel <n> --session <id>
  claudeafter --list                  every session
  claudeafter --list --session <id>   just one

  -h, --help      this text
  -v, --version   version
`
