# claudeline

A status line for [Claude Code](https://claude.ai/code) showing current directory, model, context usage, and rate limits.

```
~/projects/myapp (main*) Sonnet 4.6 | ctx: 47/200k (17%) · session: 22% · week: 30% · resets in 2h 15m
```

## Install

**Linux / macOS**

```bash
curl -fsSL https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/statusline/install.sh | bash
```

**Windows (PowerShell)**

```powershell
irm https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/statusline/install.ps1 | iex
```

Restart Claude Code after installing.

## What it shows

| Segment | Description |
|---|---|
| `~/path/to/dir` | Current working directory (`~` for home, deep paths truncated to last 2 dirs) |
| `(branch*)` | Git branch, `*` if there are uncommitted changes |
| `Sonnet 4.6` | Current model |
| `ctx: 47/200k (17%)` | Tokens used / context window size (turns yellow above 75%) |
| `session: 22%` | 5-hour rate limit usage — Claude.ai subscribers only (turns yellow above 80%) |
| `week: 30%` | 7-day rate limit usage — Claude.ai subscribers only (turns yellow above 80%) |
| `resets in 2h 15m` | Time remaining until the 5-hour session window resets |

## Uninstall

Remove the binary and revert `settings.json`:

```bash
rm ~/.claude/claudeline
```

Then remove the `statusLine` entry from `~/.claude/settings.json`.
