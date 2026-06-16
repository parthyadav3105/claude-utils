package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"strings"
	"time"
)

var version = "dev"

// Color codes; cleared by disableColors when NO_COLOR is set.
var (
	lightBlue = "\033[0;94m"
	dim       = "\033[2m"
	yellow    = "\033[0;33m"
	reset     = "\033[0m"
)

func disableColors() {
	lightBlue, dim, yellow, reset = "", "", "", ""
}

type statusInput struct {
	Workspace struct {
		CurrentDir string `json:"current_dir"`
	} `json:"workspace"`
	Model struct {
		DisplayName string `json:"display_name"`
		ID          string `json:"id"`
	} `json:"model"`
	ContextWindow struct {
		UsedPercentage    float64 `json:"used_percentage"`
		ContextWindowSize int64   `json:"context_window_size"`
	} `json:"context_window"`
	RateLimits struct {
		FiveHour struct {
			UsedPercentage float64 `json:"used_percentage"`
			ResetsAt       int64   `json:"resets_at"`
		} `json:"five_hour"`
		SevenDay struct {
			UsedPercentage float64 `json:"used_percentage"`
			ResetsAt       int64   `json:"resets_at"`
		} `json:"seven_day"`
	} `json:"rate_limits"`
	Cost struct {
		TotalCostUSD float64 `json:"total_cost_usd"`
	} `json:"cost"`
	Cwd string `json:"cwd"`
}

func shortenPath(path string) string {
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(path, home) {
		path = "~" + path[len(home):]
	}
	if strings.Count(path, "/") > 2 {
		parts := strings.Split(path, "/")
		var segments []string
		for _, p := range parts {
			if p != "" {
				segments = append(segments, p)
			}
		}
		if len(segments) >= 2 {
			return ".../" + segments[len(segments)-2] + "/" + segments[len(segments)-1]
		}
	}
	return path
}

func gitInfo(cwd string) string {
	// A single porcelain=v2 call yields the branch, the commit hash (for
	// detached HEAD), and the working-tree state. Bounded by a timeout so a
	// slow or hung git can't stall the status line.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", cwd, "status", "--porcelain=v2", "--branch")
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	var branch, oid, dirty string
	for l := range strings.SplitSeq(string(out), "\n") {
		switch {
		case strings.HasPrefix(l, "# branch.head "):
			branch = strings.TrimPrefix(l, "# branch.head ")
		case strings.HasPrefix(l, "# branch.oid "):
			oid = strings.TrimPrefix(l, "# branch.oid ")
		case l != "" && !strings.HasPrefix(l, "#"):
			// Any tracked change or untracked file marks the tree dirty.
			dirty = "*"
		}
	}

	// Detached HEAD reports "(detached)"; show the short commit hash instead.
	if branch == "(detached)" || branch == "" {
		if len(oid) >= 7 {
			branch = oid[:7]
		} else {
			branch = oid
		}
	}
	if branch == "" {
		return ""
	}
	return fmt.Sprintf(" (%s%s)", branch, dirty)
}

func fmtReset(unix int64) string {
	if unix == 0 {
		return ""
	}
	remaining := time.Until(time.Unix(unix, 0))
	if remaining <= 0 {
		return ""
	}
	hours := int(remaining.Hours())
	minutes := int(remaining.Minutes()) % 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func fmtWinK(tokens int64) string {
	if tokens >= 1000 {
		return fmt.Sprintf("%dk", tokens/1000)
	}
	return fmt.Sprintf("%d", tokens)
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(version)
		return
	}

	// https://no-color.org: honored when present and non-empty.
	if v, ok := os.LookupEnv("NO_COLOR"); ok && v != "" {
		disableColors()
	}

	raw, err := io.ReadAll(os.Stdin)
	if err != nil || len(strings.TrimSpace(string(raw))) == 0 {
		fmt.Println()
		return
	}

	var in statusInput
	_ = json.Unmarshal(raw, &in)

	cwd := in.Workspace.CurrentDir
	if cwd == "" {
		cwd = in.Cwd
	}
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	dir := shortenPath(cwd)
	git := gitInfo(cwd)
	model := in.Model.DisplayName
	if model == "" {
		model = in.Model.ID
	}

	var line strings.Builder
	line.WriteString(lightBlue + dir + reset)
	line.WriteString(git)
	if model != "" {
		line.WriteString(" " + dim + model + reset)
	}

	if in.ContextWindow.ContextWindowSize > 0 {
		pct := int(math.Round(in.ContextWindow.UsedPercentage))
		usedK := int64(float64(in.ContextWindow.ContextWindowSize)*in.ContextWindow.UsedPercentage/100) / 1000
		winK := fmtWinK(in.ContextWindow.ContextWindowSize)
		color := ""
		if pct > 75 {
			color = yellow
		}
		sep := dim + " | " + reset
		line.WriteString(sep + fmt.Sprintf("%sctx: %d/%s (%d%%)%s", color, usedK, winK, pct, reset))
	}

	if in.Cost.TotalCostUSD > 0 {
		line.WriteString(dim + " · " + reset + fmt.Sprintf("$%.2f", in.Cost.TotalCostUSD))
	}

	if in.RateLimits.FiveHour.UsedPercentage > 0 {
		session := int(math.Round(in.RateLimits.FiveHour.UsedPercentage))
		color := ""
		if session >= 80 {
			color = yellow
		}
		line.WriteString(dim + " · " + reset + fmt.Sprintf("%ssession: %d%%%s", color, session, reset))
	}
	if in.RateLimits.SevenDay.UsedPercentage > 0 {
		week := int(math.Round(in.RateLimits.SevenDay.UsedPercentage))
		color := ""
		if week >= 80 {
			color = yellow
		}
		line.WriteString(dim + " · " + reset + fmt.Sprintf("%sweek: %d%%%s", color, week, reset))
	}

	if resetIn := fmtReset(in.RateLimits.FiveHour.ResetsAt); resetIn != "" {
		line.WriteString(dim + " · resets in " + resetIn + reset)
	}

	line.WriteString("\n")
	fmt.Print(line.String())
}
