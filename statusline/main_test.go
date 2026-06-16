package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestFmtWinK(t *testing.T) {
	cases := map[int64]string{
		0:       "0",
		999:     "999",
		1000:    "1k",
		200000:  "200k",
		1500000: "1500k",
	}
	for in, want := range cases {
		if got := fmtWinK(in); got != want {
			t.Errorf("fmtWinK(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestFmtReset(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		unix int64
		want string
	}{
		{"unset", 0, ""},
		{"past", now.Add(-time.Hour).Unix(), ""},
		{"hours and minutes", now.Add(2*time.Hour + 5*time.Minute + 30*time.Second).Unix(), "2h 5m"},
		{"minutes only", now.Add(45*time.Minute + 30*time.Second).Unix(), "45m"},
	}
	for _, tt := range tests {
		if got := fmtReset(tt.unix); got != tt.want {
			t.Errorf("%s: fmtReset = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestShortenPath(t *testing.T) {
	t.Setenv("HOME", "/home/user")
	tests := []struct {
		in, want string
	}{
		{"/home/user", "~"},
		{"/home/user/project", "~/project"},
		{"/home/user/a/b/c", ".../b/c"},
		{"/var/lib/foo/bar", ".../foo/bar"},
		{"/tmp", "/tmp"},
	}
	for _, tt := range tests {
		if got := shortenPath(tt.in); got != tt.want {
			t.Errorf("shortenPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestGitInfo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Non-repo directory yields no git segment.
	if got := gitInfo(t.TempDir()); got != "" {
		t.Errorf("gitInfo(non-repo) = %q, want \"\"", got)
	}

	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "t@t.test")
	run("config", "user.name", "Test")
	run("commit", "--allow-empty", "-m", "init")

	// Clean repo: branch, no dirty marker.
	if got := gitInfo(dir); got != " (main)" {
		t.Errorf("gitInfo(clean) = %q, want %q", got, " (main)")
	}

	// Untracked file alone must mark the tree dirty (regression: the old
	// diff-based check missed untracked files).
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := gitInfo(dir); got != " (main*)" {
		t.Errorf("gitInfo(untracked) = %q, want %q", got, " (main*)")
	}
}
