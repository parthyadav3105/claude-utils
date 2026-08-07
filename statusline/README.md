# claudeline

A status line for [Claude Code](https://claude.ai/code) showing current directory, model, context usage, and rate limits.

```
~/projects/myapp (main*) Sonnet 4.6 | ctx: 47/200k (17%) · session: 22% · week: 30% · resets in 2h 15m
```

## Install

By default, `claudeline` installs into `~/.claude` (or `%USERPROFILE%\.claude` on Windows).

If your Claude configuration lives somewhere else (for example `~/.claude-max`), set `CLAUDE_DIR` before running the installer.

**Linux / macOS**

Default install:

```bash
curl -fsSL https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/statusline/install.sh | bash
```

Custom config directory:

```bash
CLAUDE_DIR="$HOME/.claude-max" \
  bash -c "$(curl -fsSL https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/statusline/install.sh)"
```

**Windows (PowerShell)**

Default install:

```powershell
irm https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/statusline/install.ps1 | iex
```

Custom config directory:

```powershell
$env:CLAUDE_DIR="$HOME\.claude-max"
irm https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/statusline/install.ps1 | iex
Remove-Item Env:CLAUDE_DIR
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

If you installed to the default location:

```bash
rm ~/.claude/claudeline
```

If you installed using `CLAUDE_DIR`, remove the binary from that directory instead.

Then remove the `statusLine` entry from `settings.json` in the same configuration directory.
