package main

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

// Never rename these two: settings written by any older version call them, so they are how a new
// binary gets to run after an upgrade. A version that changes the wiring upgrades the user's
// settings from inside these commands, rewriting only its own outdated entries.
const hookCommand = "task hook claude-code"

const statuslineCommand = "task statusline claude-code"

const allowRule = "Bash(task:*)"

var hookEvents = []string{"UserPromptSubmit", "SessionStart"}

const reinjectAfterTokens = 40_000

const reinjectAfterTranscriptBytes = reinjectAfterTokens * 25

const transcriptTailBytes = 256 << 10

type claudeSession struct {
	Name        string `json:"name"`
	FormerNames []struct {
		Name string `json:"name"`
	} `json:"formerNames"`
}

func setupCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "setup claude-code",
		Short:     "Add the task hook, permission and status line to Claude Code",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"claude-code"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := exec.LookPath("task"); err != nil {
				return errors.New("task is not on your PATH, and the Claude Code hook runs it by name")
			}
			dir := installDir()
			file, err := editSettings(dir, func(settings map[string]any) error {
				statusLine, _ := settings["statusLine"].(map[string]any)
				command, _ := statusLine["command"].(string)
				if runtime.GOOS != "windows" && !strings.Contains(command, statuslineCommand) {
					if err := errors.Join(os.MkdirAll(dir, 0o755), os.WriteFile(statusLineFile(dir), []byte(command), 0o644)); err != nil {
						return err
					}
					statusLine = object(settings, "statusLine")
					statusLine["type"] = "command"
					statusLine["command"] = statuslineCommand
				}
				wireHooks(settings)
				permissions := object(settings, "permissions")
				if !slices.Contains(array(permissions, "allow"), any(allowRule)) {
					permissions["allow"] = append(array(permissions, "allow"), allowRule)
				}
				return nil
			})
			if err != nil {
				return err
			}
			fmt.Printf("Updated %s:\n  %s hooks: %s\n  allowed: %s\n", file, strings.Join(hookEvents, " and "), hookCommand, allowRule)
			if runtime.GOOS == "windows" {
				fmt.Println("  status line: left unchanged, as it needs sh")
			} else {
				fmt.Printf("  status line: %s\n", statuslineCommand)
			}
			return nil
		},
	}
}

func uninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "uninstall claude-code",
		Short:     "Remove the task hook, permission and status line from Claude Code",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"claude-code"},
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := installDir()
			file, err := editSettings(dir, func(settings map[string]any) error {
				if statusLine, _ := settings["statusLine"].(map[string]any); statusLine != nil {
					if command, _ := statusLine["command"].(string); strings.Contains(command, statuslineCommand) {
						original, err := os.ReadFile(statusLineFile(dir))
						switch {
						case err != nil && !errors.Is(err, os.ErrNotExist):
							return err
						case len(original) > 0:
							statusLine["command"] = string(original)
						default:
							delete(settings, "statusLine")
						}
					}
				}
				hooks := object(settings, "hooks")
				for _, event := range hookEvents {
					hooks[event] = slices.DeleteFunc(array(hooks, event), func(group any) bool {
						if !hasOurs(group) {
							return false
						}
						g := group.(map[string]any)
						g["hooks"] = slices.DeleteFunc(array(g, "hooks"), ours)
						return len(array(g, "hooks")) == 0
					})
				}
				permissions := object(settings, "permissions")
				permissions["allow"] = slices.DeleteFunc(array(permissions, "allow"), func(rule any) bool { return rule == allowRule })
				for _, event := range hookEvents {
					prune(settings, "hooks", event)
				}
				prune(settings, "permissions", "allow")
				return nil
			})
			if err != nil {
				return err
			}
			if err := os.Remove(statusLineFile(dir)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			fmt.Printf("Removed the task hook, status line and %s from %s\n", allowRule, file)
			return nil
		},
	}
}

// Stopgap until Claude Code function hooks are generally available. Rewrite this with them then:
// they can draw the task list in the band above the prompt (AbovePrompt) instead of wrapping the
// user's status line.
func statuslineCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "statusline claude-code",
		Hidden:    true,
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"claude-code"},
		Run: func(cmd *cobra.Command, args []string) {
			if os.Getenv("TASK_STATUSLINE") != "" {
				return
			}
			input, _ := io.ReadAll(os.Stdin)
			if original, err := os.ReadFile(statusLineFile(claudeDir())); err == nil && len(original) > 0 {
				run := exec.Command("sh", "-c", string(original))
				run.Env = append(os.Environ(), "TASK_STATUSLINE=1")
				run.Stdin = bytes.NewReader(input)
				run.Stderr = os.Stderr
				out, _ := run.Output()
				if line := strings.TrimRight(string(out), "\r\n"); line != "" {
					fmt.Println(line)
					fmt.Println("⠀")
				}
			}
			var session struct {
				Workspace struct {
					CurrentDir string `json:"current_dir"`
				} `json:"workspace"`
			}
			if json.Unmarshal(input, &session) == nil && session.Workspace.CurrentDir != "" {
				os.Chdir(session.Workspace.CurrentDir)
			}
			if dir, err := project(); err == nil {
				tasks, _ := loadAll(dir)
				open := openIDs(tasks)
				var pending, done []Task
				for _, t := range tasks {
					if open[t.ID] {
						pending = append(pending, t)
					} else if time.Since(t.Done) < time.Hour {
						done = append(done, t)
					}
				}
				slices.SortFunc(done, func(a, b Task) int { return b.Done.Compare(a.Done) })
				printStatusRows(pending, done, open)
			}
		},
	}
}

func printStatusRows(pending, done []Task, open map[int]bool) {
	if len(pending)+len(done) == 0 {
		fmt.Println(paint("2", "No pending tasks"))
		return
	}
	header := fmt.Sprintf("Tasks · %d open", len(pending))
	if blocked := len(slices.DeleteFunc(slices.Clone(pending), func(t Task) bool { return blockers(t, open) == "" })); blocked > 0 {
		header += fmt.Sprintf(" · %d waiting", blocked)
	}
	fmt.Println(paint("2", header))
	rows := append(slices.Clone(pending[:min(len(pending), 3)]), done...)
	rows = rows[:min(len(rows), 3)]
	var idWidth, nameWidth, ageWidth int
	for _, t := range rows {
		idWidth = max(idWidth, width(fmt.Sprintf("#%d", t.ID)))
		nameWidth = max(nameWidth, width(shorten(t.Name)))
		ageWidth = max(ageWidth, width(age(t)))
	}
	for i, t := range rows {
		guide := "├"
		if i == len(rows)-1 {
			guide = "└"
		}
		name := shorten(t.Name)
		box, styled := paint("2", "◻"), name
		switch {
		case !t.Done.IsZero():
			box, styled = paint("2", "✔"), paint("2", strike(name))
		case t.Owner != "":
			box = paint(agentColor(t.Owner), "◼")
		}
		meta := pad(fmt.Sprintf("#%d", t.ID), idWidth) + " · " + pad(age(t), ageWidth)
		row := paint("2", guide) + " " + box + " " + styled + pad("", nameWidth-width(name)) + "    " + paint("2", meta)
		status := []string{paint("2;3", "done")}
		if t.Done.IsZero() {
			status = nil
			if t.Owner != "" {
				status = append(status, paint("2;3", "assigned to "+t.Owner))
			}
			if b := blockers(t, open); b != "" {
				status = append(status, paint("2;3", "waiting on "+b))
			}
			if len(status) == 0 {
				status = []string{paint("2;3", "pending")}
			}
		}
		fmt.Println(row + "    " + strings.Join(status, paint("2;3", " · ")))
	}
}

func agentColor(name string) string {
	h := fnv.New32a()
	h.Write([]byte(name))
	return []string{"94", "35", "36", "32", "33"}[h.Sum32()%5]
}

func strike(s string) string {
	var b strings.Builder
	for _, r := range s {
		b.WriteRune(r)
		b.WriteRune('̶')
	}
	return b.String()
}

const statusNameLength = 48

func shorten(name string) string {
	if runes := []rune(name); len(runes) > statusNameLength {
		return string(runes[:statusNameLength-1]) + "…"
	}
	return name
}

func age(t Task) string {
	if !t.Done.IsZero() {
		return ago(t.Done) + " ago"
	}
	return ago(t.Created) + " ago"
}

func width(s string) int {
	return utf8.RuneCountInString(s)
}

func pad(s string, w int) string {
	return s + strings.Repeat(" ", max(0, w-width(s)))
}

func paint(code, text string) string {
	if os.Getenv("NO_COLOR") != "" {
		return text
	}
	return "\033[" + code + "m" + text + "\033[0m"
}

func statusLineFile(dir string) string {
	return filepath.Join(dir, "task-statusline")
}

func hookCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "hook claude-code",
		Hidden:    true,
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"claude-code"},
		Run: func(cmd *cobra.Command, args []string) {
			var in hookInput
			json.NewDecoder(os.Stdin).Decode(&in)
			if in.Event == "UserPromptSubmit" {
				upgradeWiring()
			}
			me, err := readSession(filepath.Join(claudeDir(), "sessions", os.Getenv("CLAUDE_PID")+".json"))
			if err == nil && me.Name != "" {
				followRenames(me)
			}
			hash := ""
			if dir, err := project(); err == nil {
				tasks, _ := loadAll(dir)
				hash = listHash(tasks)
			}
			now := readConversation(in.TranscriptPath)
			stateFile := injectionFile(in.SessionID)
			last := loadInjection(stateFile)
			if !shouldInject(in.Event, last, hash, now) {
				if now.contextTokens > 0 && (last.ContextTokens == 0 || now.contextTokens < last.ContextTokens) {
					last.ContextTokens = now.contextTokens
					saveInjection(stateFile, *last)
				}
				return
			}
			if me.Name != "" {
				fmt.Printf("You are %s. Use this name as OWNER in task.\n", me.Name)
			}
			if err := showList(false, false, 5); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
			}
			saveInjection(stateFile, injection{ListHash: hash, ContextTokens: now.contextTokens, TranscriptBytes: now.transcriptBytes})
		},
	}
}

type hookInput struct {
	Event          string `json:"hook_event_name"`
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
}

type injection struct {
	ListHash        string `json:"list_hash"`
	ContextTokens   int    `json:"context_tokens"`
	TranscriptBytes int64  `json:"transcript_bytes"`
}

type conversation struct {
	contextTokens   int
	transcriptBytes int64
	midTurn         bool
}

func shouldInject(event string, last *injection, listHash string, now conversation) bool {
	switch {
	case event == "SessionStart", last == nil, now.midTurn, listHash != last.ListHash:
		return true
	}
	if last.ContextTokens > 0 && now.contextTokens > 0 {
		return now.contextTokens-last.ContextTokens >= reinjectAfterTokens
	}
	return now.transcriptBytes-last.TranscriptBytes >= reinjectAfterTranscriptBytes
}

func listHash(tasks []Task) string {
	open := openIDs(tasks)
	h := sha256.New()
	for _, t := range tasks {
		if open[t.ID] {
			fmt.Fprintf(h, "%d\x00%s\x00%s\x00%v\n", t.ID, t.Name, t.Owner, t.BlockedBy)
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Reads Claude Code's transcript, which is not a documented format: when its records cannot be
// read, the context size stays 0 and the turn counts as idle, so the hook falls back to the file's
// growth in bytes, a rough stand-in for context growth.
func readConversation(transcript string) conversation {
	var c conversation
	f, err := os.Open(transcript)
	if err != nil {
		return c
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return c
	}
	c.transcriptBytes = info.Size()
	offset := max(0, info.Size()-transcriptTailBytes)
	data, err := io.ReadAll(io.NewSectionReader(f, offset, info.Size()-offset))
	if err != nil {
		return c
	}
	lines := strings.Split(string(data), "\n")
	if offset > 0 {
		lines = lines[1:]
	}
	for _, line := range lines {
		var record struct {
			Type    string `json:"type"`
			Message struct {
				Content json.RawMessage `json:"content"`
				Usage   *struct {
					InputTokens              int `json:"input_tokens"`
					CacheReadInputTokens     int `json:"cache_read_input_tokens"`
					CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(line), &record) != nil || record.Type != "user" && record.Type != "assistant" {
			continue
		}
		if u := record.Message.Usage; record.Type == "assistant" && u != nil {
			c.contextTokens = u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
		}
		var blocks []struct {
			Type string `json:"type"`
		}
		json.Unmarshal(record.Message.Content, &blocks)
		c.midTurn = len(blocks) > 0 && (blocks[0].Type == "tool_use" || blocks[0].Type == "tool_result")
	}
	return c
}

func injectionFile(sessionID string) string {
	cache, err := os.UserCacheDir()
	if sessionID == "" || err != nil {
		return ""
	}
	return filepath.Join(cache, "task", "sessions", filepath.Base(sessionID)+".json")
}

func loadInjection(file string) *injection {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	var last injection
	if json.Unmarshal(data, &last) != nil {
		return nil
	}
	return &last
}

func saveInjection(file string, state injection) {
	if file == "" {
		return
	}
	dir := filepath.Dir(file)
	if os.MkdirAll(dir, 0o755) != nil {
		return
	}
	data, _ := json.Marshal(state)
	tmp := fmt.Sprintf("%s.%d", file, os.Getpid())
	if os.WriteFile(tmp, data, 0o644) == nil {
		os.Rename(tmp, file)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > 7*24*time.Hour {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

func followRenames(me claudeSession) {
	files, _ := filepath.Glob(filepath.Join(claudeDir(), "sessions", "*.json"))
	held := map[string]bool{}
	for _, file := range files {
		if s, err := readSession(file); err == nil {
			held[s.Name] = true
		}
	}
	stale := map[string]bool{}
	for _, former := range me.FormerNames {
		if former.Name != "" && !held[former.Name] {
			stale[former.Name] = true
		}
	}
	if len(stale) == 0 {
		return
	}
	dirs, _ := projects()
	for _, dir := range dirs {
		tasks, _ := loadAll(dir)
		for _, t := range tasks {
			if !stale[t.Owner] {
				continue
			}
			if fresh, err := load(dir, t.ID); err == nil && fresh.Owner == t.Owner {
				fresh.Owner = me.Name
				save(dir, fresh)
			}
		}
	}
}

func readSession(file string) (claudeSession, error) {
	var s claudeSession
	data, err := os.ReadFile(file)
	if err == nil {
		err = json.Unmarshal(data, &s)
	}
	return s, err
}

func claudeDir() string {
	return expandHome(cmp.Or(os.Getenv("CLAUDE_CONFIG_DIR"), filepath.Join(home(), ".claude")))
}

// Setup and uninstall take CLAUDE_DIR first, like the other claude-utils installers. The hook and
// status line must not: at run time only CLAUDE_CONFIG_DIR says which profile Claude is using.
func installDir() string {
	return expandHome(cmp.Or(os.Getenv("CLAUDE_DIR"), os.Getenv("CLAUDE_CONFIG_DIR"), filepath.Join(home(), ".claude")))
}

func expandHome(dir string) string {
	if dir == "~" || strings.HasPrefix(dir, "~/") || strings.HasPrefix(dir, `~\`) {
		return filepath.Join(home(), dir[1:])
	}
	return dir
}

func editSettings(dir string, edit func(settings map[string]any) error) (string, error) {
	file := filepath.Join(dir, "settings.json")
	settings := map[string]any{}
	data, err := os.ReadFile(file)
	if err == nil {
		err = json.Unmarshal(data, &settings)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("%s: %w", file, err)
	}
	if settings == nil {
		return "", fmt.Errorf("%s: not a JSON object", file)
	}
	if err := edit(settings); err != nil {
		return "", err
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(settings); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return "", err
	}
	target := file
	if resolved, err := filepath.EvalSymlinks(file); err == nil {
		target = resolved
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(target); err == nil {
		mode = info.Mode().Perm()
	}
	tmp := fmt.Sprintf("%s.%d", target, os.Getpid())
	if err := os.WriteFile(tmp, out.Bytes(), mode); err != nil {
		return "", err
	}
	return file, os.Rename(tmp, target)
}

func object(parent map[string]any, key string) map[string]any {
	child, ok := parent[key].(map[string]any)
	if !ok {
		child = map[string]any{}
		parent[key] = child
	}
	return child
}

func array(parent map[string]any, key string) []any {
	list, _ := parent[key].([]any)
	return list
}

func prune(settings map[string]any, key, list string) {
	child := object(settings, key)
	if len(array(child, list)) == 0 {
		delete(child, list)
	}
	if len(child) == 0 {
		delete(settings, key)
	}
}

func ours(hook any) bool {
	h, _ := hook.(map[string]any)
	return h["command"] == hookCommand
}

func hasOurs(group any) bool {
	g, _ := group.(map[string]any)
	return slices.ContainsFunc(array(g, "hooks"), ours)
}

func wireHooks(settings map[string]any) {
	hooks := object(settings, "hooks")
	for _, event := range hookEvents {
		if !slices.ContainsFunc(array(hooks, event), hasOurs) {
			hooks[event] = append(array(hooks, event),
				map[string]any{"hooks": []any{map[string]any{"type": "command", "command": hookCommand}}})
		}
	}
}

func upgradeWiring() {
	dir := claudeDir()
	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		return
	}
	var settings map[string]any
	if json.Unmarshal(data, &settings) != nil {
		return
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if !slices.ContainsFunc(array(hooks, "UserPromptSubmit"), hasOurs) {
		return
	}
	if !slices.ContainsFunc(hookEvents, func(event string) bool { return !slices.ContainsFunc(array(hooks, event), hasOurs) }) {
		return
	}
	editSettings(dir, func(settings map[string]any) error {
		wireHooks(settings)
		return nil
	})
}
