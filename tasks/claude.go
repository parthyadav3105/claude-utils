package main

import (
	"bytes"
	"cmp"
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
			file, err := editSettings(func(settings map[string]any) error {
				statusLine, _ := settings["statusLine"].(map[string]any)
				command, _ := statusLine["command"].(string)
				if runtime.GOOS != "windows" && !strings.Contains(command, statuslineCommand) {
					if err := errors.Join(os.MkdirAll(claudeDir(), 0o755), os.WriteFile(statusLineFile(), []byte(command), 0o644)); err != nil {
						return err
					}
					statusLine = object(settings, "statusLine")
					statusLine["type"] = "command"
					statusLine["command"] = statuslineCommand
				}
				hooks := object(settings, "hooks")
				if !slices.ContainsFunc(array(hooks, "UserPromptSubmit"), hasOurs) {
					hooks["UserPromptSubmit"] = append(array(hooks, "UserPromptSubmit"),
						map[string]any{"hooks": []any{map[string]any{"type": "command", "command": hookCommand}}})
				}
				permissions := object(settings, "permissions")
				if !slices.Contains(array(permissions, "allow"), any(allowRule)) {
					permissions["allow"] = append(array(permissions, "allow"), allowRule)
				}
				return nil
			})
			if err != nil {
				return err
			}
			fmt.Printf("Updated %s:\n  UserPromptSubmit hook: %s\n  allowed: %s\n", file, hookCommand, allowRule)
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
			file, err := editSettings(func(settings map[string]any) error {
				if statusLine, _ := settings["statusLine"].(map[string]any); statusLine != nil {
					if command, _ := statusLine["command"].(string); strings.Contains(command, statuslineCommand) {
						original, err := os.ReadFile(statusLineFile())
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
				hooks["UserPromptSubmit"] = slices.DeleteFunc(array(hooks, "UserPromptSubmit"), func(group any) bool {
					if !hasOurs(group) {
						return false
					}
					g := group.(map[string]any)
					g["hooks"] = slices.DeleteFunc(array(g, "hooks"), ours)
					return len(array(g, "hooks")) == 0
				})
				permissions := object(settings, "permissions")
				permissions["allow"] = slices.DeleteFunc(array(permissions, "allow"), func(rule any) bool { return rule == allowRule })
				prune(settings, "hooks", "UserPromptSubmit")
				prune(settings, "permissions", "allow")
				return nil
			})
			if err != nil {
				return err
			}
			if err := os.Remove(statusLineFile()); err != nil && !errors.Is(err, os.ErrNotExist) {
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
			if original, err := os.ReadFile(statusLineFile()); err == nil && len(original) > 0 {
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

func shorten(name string) string {
	if runes := []rune(name); len(runes) > 32 {
		return string(runes[:31]) + "…"
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

func statusLineFile() string {
	return filepath.Join(claudeDir(), "task-statusline")
}

func hookCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "hook claude-code",
		Hidden:    true,
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"claude-code"},
		Run: func(cmd *cobra.Command, args []string) {
			me, err := readSession(filepath.Join(claudeDir(), "sessions", os.Getenv("CLAUDE_PID")+".json"))
			if err == nil && me.Name != "" {
				followRenames(me)
				fmt.Printf("You are %s. Use this name as OWNER in task.\n", me.Name)
			}
			if err := showList(false, false, nil, 5); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
			}
		},
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
	return cmp.Or(os.Getenv("CLAUDE_CONFIG_DIR"), filepath.Join(home(), ".claude"))
}

func editSettings(edit func(settings map[string]any) error) (string, error) {
	file := filepath.Join(claudeDir(), "settings.json")
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
	return file, os.WriteFile(file, out.Bytes(), 0o644)
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
