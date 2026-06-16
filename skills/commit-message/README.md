# commit-message

A [Claude Code](https://claude.ai/code) skill that drafts a commit message
from the work done in the current session — capturing the *why* a diff
can't — and matches your repo's convention.

## Install

**User-level** (available in every project):

```bash
curl -fsSL https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/skills/commit-message/install.sh | bash
```

This installs the skill to `~/.claude/skills/commit-message/SKILL.md`.
Restart Claude Code, then invoke it with `/commit-message`.

**Project-level** (commit the skill into one repo):

```bash
curl -fsSL https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/skills/commit-message/install.sh | bash -s -- --project /path/to/your/repo
```

This installs to `<repo>/.claude/skills/commit-message/SKILL.md`.

## Usage

In Claude Code, after doing some work:

```
/commit-message
```

Pass optional extra context — an issue ref or a style override:

```
/commit-message refs ABC-123, use lowercase subject
```

It drafts the message and shows it for your review.

## Uninstall

```bash
curl -fsSL https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/skills/commit-message/install.sh | bash -s -- --uninstall
```
