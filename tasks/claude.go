package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"
)

const hookCommand = "task hook claude-code"

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
		Short:     "Add the task hook and permission to Claude Code",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"claude-code"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := exec.LookPath("task"); err != nil {
				return errors.New("task is not on your PATH, and the Claude Code hook runs it by name")
			}
			file, err := editSettings(func(settings map[string]any) {
				hooks := object(settings, "hooks")
				if !slices.ContainsFunc(array(hooks, "UserPromptSubmit"), hasOurs) {
					hooks["UserPromptSubmit"] = append(array(hooks, "UserPromptSubmit"),
						map[string]any{"hooks": []any{map[string]any{"type": "command", "command": hookCommand}}})
				}
				permissions := object(settings, "permissions")
				if !slices.Contains(array(permissions, "allow"), any(allowRule)) {
					permissions["allow"] = append(array(permissions, "allow"), allowRule)
				}
			})
			if err != nil {
				return err
			}
			fmt.Printf("Updated %s:\n  UserPromptSubmit hook: %s\n  allowed: %s\n", file, hookCommand, allowRule)
			return nil
		},
	}
}

func uninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "uninstall claude-code",
		Short:     "Remove the task hook and permission from Claude Code",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"claude-code"},
		RunE: func(cmd *cobra.Command, args []string) error {
			file, err := editSettings(func(settings map[string]any) {
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
			})
			if err != nil {
				return err
			}
			fmt.Printf("Removed the task hook and %s from %s\n", allowRule, file)
			return nil
		},
	}
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

func editSettings(edit func(settings map[string]any)) (string, error) {
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
	edit(settings)
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
