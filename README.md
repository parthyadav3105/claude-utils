# claude-utils

A collection of utilities for [Claude Code](https://claude.ai/code).

## Tools

### [statusline](statusline/README.md)

Terminal status line showing cwd, model, context usage, rate limits, and
session cost.

**Linux / macOS**

```bash
curl -fsSL https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/statusline/install.sh | bash
```

**Windows (PowerShell)**

```powershell
irm https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/statusline/install.ps1 | iex
```

Restart Claude Code after installing. See the [statusline README](statusline/README.md) for details.

## Skills

### [commit-message](skills/commit-message/README.md)

Suggest a commit message from the session's work.

```bash
curl -fsSL https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/skills/commit-message/install.sh | bash
```

Restart Claude Code, then invoke with `/commit-message`. See the
[commit-message README](skills/commit-message/README.md) for project-level
install and usage.
