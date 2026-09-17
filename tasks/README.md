# task

A small task list for AI coding agents, in one portable binary. An agent writes
larger work down as tasks, claims the one it is working on, and finishes them
one at a time, so a new request from you gets recorded instead of derailing the
work in flight.

```
~$ task create "Fix login redirect"
#3 created

~$ task list
ID  NAME                AGE  OWNER       BLOCKED BY
#1  Prototype new logo  2h   spider-man
#3  Fix login redirect  4s
```

`task` knows nothing about any particular agent. The only agent-specific part is
an optional hook for Claude Code, installed with `task setup claude-code`.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/tasks/install.sh | bash
```

This puts `task` in `~/.local/bin`. Set `INSTALL_DIR` to put it somewhere else,
and make sure that folder is on your `PATH`.

Windows:

```powershell
iwr -useb https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/tasks/install.ps1 | iex
```

This puts `task.exe` in `%USERPROFILE%\.local\bin` and adds that folder to your
user `PATH`.

From source, with Go 1.25 or later:

```bash
git clone https://github.com/parthyadav3105/claude-utils
cd claude-utils/tasks && go build -o ~/.local/bin/task .
```

The binary has to be called `task`: the Claude Code hook runs it by that name.

## Usage

```
task create NAME [-b ID]...
task list   [-A] [--global] [--limit N]
task edit   ID [--name NAME] [--set-owner OWNER] [-b ID|ID-]...
task done   ID...
task delete ID...
```

| Command | Does |
|---|---|
| `create` | Adds a task. `-b` names tasks that must be done first |
| `list`, `ls` | Open tasks. `-A` adds done ones, `--global` shows every project |
| `edit` | Changes only what you pass. `ID-` removes a blocker |
| `done` | Marks tasks finished |
| `delete` | Removes tasks. Their IDs are never reused |

A few rules keep it predictable for an agent:

- **A task is a reminder, not a record.** It has a name and no description or
  labels, so the agent spends its effort on the work instead of on the list.
- **A name is at most 80 characters.** A longer one is rejected, not cut, so the
  agent rewrites it.
- **A task is referred to as `#ID`.** On the command line, type the bare number
  (`task done 3`): an unquoted `#3` starts a shell comment.
- **An owner is a name.** Before starting a task, an agent claims it with
  `task edit ID --set-owner NAME`, using the name other agents reach it by.
- **Several agents can run `task` at once.** Two creates never get the same ID.

`task --help` is written for agents: it says when to use the tool and how.

## Projects

There is no setup per project. `task` works out the project from the current
folder:

1. **Inside a git repository**, the project is the repository. All worktrees of
   one repository share a list, and a submodule has its own.
2. **Outside git**, it is the nearest folder above that already had tasks. Your
   home folder never counts, so tasks kept there don't show up everywhere.
3. **Otherwise** it is the current folder.

Tasks are stored per user, outside your projects, so they never show up in
`git status`.

## Claude Code

```bash
task setup claude-code
```

By default this uses `~/.claude`. If your Claude configuration lives somewhere
else (for example `~/.claude-max`, or a profile), set `CLAUDE_DIR`:

```bash
CLAUDE_DIR="$HOME/.claude-max" task setup claude-code
```

Run it once for each Claude directory you use. It changes three things in
`$CLAUDE_DIR/settings.json`, leaving everything else in it as it was:

- `UserPromptSubmit` and `SessionStart` hooks that run `task hook claude-code`;
- a permission rule, `Bash(task:*)`, so the agent can run `task` without asking;
- the status line, so you can see the top open tasks too (see below). On
  Windows the status line is left as it is, because the wrapper needs `sh`.

The hook adds this to the agent's context:

```
You are spider-man. Use this name as OWNER in task.
ID  NAME                AGE  OWNER       BLOCKED BY
#1  Prototype new logo  2h   spider-man
```

- **Who the agent is.** The name is the Claude Code session name: the one other
  sessions use to message it.
- **What is on the list.** Five tasks at most, so each copy stays
  small. With no tasks, it prints a short note on when to use `task` instead.
- **Renames are followed.** When a session is renamed, by `/rename` or
  automatically, tasks owned under its old name move to the new one at its next
  message.

### When the list is added

Every injected list stays in the conversation, so repeating it on every message
only piles up copies. The hook adds it when a session starts, resumes, is
cleared or is compacted, and on a message you send when:

- **you send it while the agent is working**, the moment a new request could
  derail the current task;
- **the open tasks changed** since the list was last added, by any agent;
- **the context grew by 40k tokens** since then, so the last copy is far back.

Otherwise the message goes through with nothing added. The hook remembers what
it last added per session in your user cache folder (`task/sessions`), and
deletes those notes after a week.

Knowing whether the agent is working, and how big the context is, comes from
Claude Code's transcript file. Its format is not documented. If the token counts
in it cannot be read, the hook uses the file's growth instead, adding the list
again after about 1 MB. That is rough, since tool output makes some transcripts
far bigger than their context, but it still brings the list back.

Settings from an older `task setup` have only the `UserPromptSubmit` hook. The
hook adds the `SessionStart` entry itself, on the next message, in that same
settings file.

### Status line

Claude Code has a single status line command. Setup saves the one you have (for
example [claudeline](../statusline/README.md)) and puts `task statusline
claude-code` in its place. That runs your saved status line first, prints its
output unchanged, and adds up to three task rows below it:

```
~/games/ijs (main*) Opus 5 | ctx: 47/1000k (5%) · session: 22%

Tasks · 2 open · 1 waiting
├ ◼ Prototype a new logo           #1 · 3h ago     assigned to spider-man
├ ◼ Tune the cascade drop speed    #3 · 22m ago    assigned to quiet-harbor · waiting on #1
└ ✔ Profile boot time on the J7    #6 · 22m ago    done
```

The header counts every open task, including ones that don't fit in the three
rows. Names longer than 48 characters end in `…`.

- **◼** is a task with an owner: `assigned to NAME`. The
  square always has the same colour for the same owner, so you can tell agents
  apart at a glance.
- **◻** is an open task nobody has claimed: `pending`.
- Either can add `waiting on #N` while another task has to finish first.
- **✔** is a task finished in the last hour, struck through. Open tasks come
  first; finished ones only fill spare rows.

With nothing to show, it adds a single `No pending tasks` line. `NO_COLOR` turns
the colours off. `task uninstall claude-code` puts your saved status line back.

If you install another status line tool after `task setup claude-code`, it
replaces the task rows. Run `task setup claude-code` again: it saves the new
status line and adds the tasks below it.

### Limits of the Claude Code hook

- **Names can be reused.** Owners are matched by name only. A new session that
  gets the name of one that has ended treats that session's tasks as its own.
- **Only recent names are followed.** A session remembers its last three names.
  Rename it more than three times without sending a message, and older tasks
  keep the old name.
- **Subagents get no hook.** The lead gives a subagent a name and a task ID when
  handing work over.

## Uninstall

```bash
curl -fsSL https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/tasks/install.sh | bash -s -- --uninstall
```

Windows: `.\install.ps1 -Uninstall`

This removes the Claude Code hook and permission, restores your status line, then
removes the binary. Your tasks are kept. To remove only the Claude Code integration, run
`task uninstall claude-code`, with the same `CLAUDE_DIR` you set it up with.
