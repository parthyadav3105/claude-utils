package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestShouldInject(t *testing.T) {
	last := &injection{ListHash: "same", ContextTokens: 100_000}
	cases := []struct {
		name  string
		event string
		last  *injection
		hash  string
		now   conversation
		want  bool
	}{
		{name: "session start always injects", event: "SessionStart", last: last, hash: "same", now: conversation{contextTokens: 100_000}, want: true},
		{name: "first message of a session", event: "UserPromptSubmit", last: nil, hash: "same", want: true},
		{name: "message sent mid-turn", event: "UserPromptSubmit", last: last, hash: "same", now: conversation{contextTokens: 100_001, midTurn: true}, want: true},
		{name: "list changed", event: "UserPromptSubmit", last: last, hash: "other", now: conversation{contextTokens: 100_001}, want: true},
		{name: "context grew by the threshold", event: "UserPromptSubmit", last: last, hash: "same", now: conversation{contextTokens: 140_000}, want: true},
		{name: "context grew less than the threshold", event: "UserPromptSubmit", last: last, hash: "same", now: conversation{contextTokens: 139_999}},
		{name: "context shrank after compaction", event: "UserPromptSubmit", last: last, hash: "same", now: conversation{contextTokens: 30_000}},
		{name: "context size unknown when injected", event: "UserPromptSubmit", last: &injection{ListHash: "same"}, hash: "same", now: conversation{contextTokens: 90_000}},
		{name: "context size unknown now", event: "UserPromptSubmit", last: last, hash: "same"},
		{name: "tokens unknown, transcript grew by the byte threshold", event: "UserPromptSubmit", last: &injection{ListHash: "same", TranscriptBytes: 5_000}, hash: "same", now: conversation{transcriptBytes: 5_000 + reinjectAfterTranscriptBytes}, want: true},
		{name: "tokens unknown, transcript grew less than the byte threshold", event: "UserPromptSubmit", last: &injection{ListHash: "same", TranscriptBytes: 5_000}, hash: "same", now: conversation{transcriptBytes: 4_999 + reinjectAfterTranscriptBytes}},
		{name: "tokens known now but not when injected, so bytes decide", event: "UserPromptSubmit", last: &injection{ListHash: "same", TranscriptBytes: 5_000}, hash: "same", now: conversation{contextTokens: 90_000, transcriptBytes: 5_000 + reinjectAfterTranscriptBytes}, want: true},
		{name: "tokens known on both sides win over bytes", event: "UserPromptSubmit", last: &injection{ListHash: "same", ContextTokens: 100_000}, hash: "same", now: conversation{contextTokens: 100_001, transcriptBytes: 10 * reinjectAfterTranscriptBytes}},
	}
	for _, c := range cases {
		if got := shouldInject(c.event, c.last, c.hash, c.now); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func transcriptLine(t *testing.T, record any) string {
	t.Helper()
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func assistant(block string, tokens int) map[string]any {
	return map[string]any{"type": "assistant", "message": map[string]any{
		"content": []any{map[string]any{"type": block}},
		"usage":   map[string]any{"input_tokens": 2, "cache_read_input_tokens": tokens - 2, "cache_creation_input_tokens": 0},
	}}
}

func user(content any) map[string]any {
	return map[string]any{"type": "user", "message": map[string]any{"content": content}}
}

func TestReadConversation(t *testing.T) {
	toolResult := []any{map[string]any{"type": "tool_result"}}
	turnEnd := map[string]any{"type": "system", "subtype": "turn_duration"}
	cases := []struct {
		name    string
		records []any
		want    conversation
	}{
		{name: "idle after a finished turn", records: []any{user("hi"), assistant("text", 30_000), turnEnd}, want: conversation{contextTokens: 30_000}},
		{name: "running a tool", records: []any{user("hi"), assistant("thinking", 31_000), assistant("tool_use", 31_000)}, want: conversation{contextTokens: 31_000, midTurn: true}},
		{name: "tool just returned", records: []any{assistant("tool_use", 31_000), user(toolResult)}, want: conversation{contextTokens: 31_000, midTurn: true}},
		{name: "no assistant yet", records: []any{user("hi")}, want: conversation{}},
	}
	for _, c := range cases {
		var lines []string
		for _, r := range c.records {
			lines = append(lines, transcriptLine(t, r))
		}
		file := filepath.Join(t.TempDir(), "transcript.jsonl")
		content := strings.Join(lines, "\n") + "\n"
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		c.want.transcriptBytes = int64(len(content))
		if got := readConversation(file); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
	if got := readConversation(filepath.Join(t.TempDir(), "missing.jsonl")); got != (conversation{}) {
		t.Errorf("missing transcript: got %+v", got)
	}
}

func readSettings(t *testing.T, file string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	return settings
}

func hookedEvents(settings map[string]any) []string {
	hooks, _ := settings["hooks"].(map[string]any)
	var events []string
	for event := range hooks {
		if slices.ContainsFunc(array(hooks, event), hasOurs) {
			events = append(events, event)
		}
	}
	slices.Sort(events)
	return events
}

func TestUpgradeWiringAddsSessionStart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	file := filepath.Join(dir, "settings.json")
	old := `{"theme":"light","hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"task hook claude-code"}]}]}}`
	if err := os.WriteFile(file, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	upgradeWiring()
	settings := readSettings(t, file)
	if got := hookedEvents(settings); !slices.Equal(got, []string{"SessionStart", "UserPromptSubmit"}) {
		t.Errorf("hooked events: %v", got)
	}
	if settings["theme"] != "light" {
		t.Error("other settings were lost")
	}
	if info, _ := os.Stat(file); info.Mode().Perm() != 0o600 {
		t.Errorf("mode changed to %v", info.Mode().Perm())
	}
}

func TestUpgradeWiringLeavesOtherSettingsAlone(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	file := filepath.Join(dir, "settings.json")
	original := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"task hook claude-code"}]}]}}`
	if err := os.WriteFile(file, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	upgradeWiring()
	if data, _ := os.ReadFile(file); string(data) != original {
		t.Errorf("settings without our UserPromptSubmit hook were rewritten:\n%s", data)
	}
}
