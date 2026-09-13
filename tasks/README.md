# task

A small task list for AI coding agents, in one portable binary. An agent writes
larger work down as tasks, claims the one it is working on, and finishes them
one at a time, so a new request from you gets recorded instead of derailing the
work in flight.

```
~$ task create "Fix login redirect" -l area=auth -f - <<'EOF'
Users land on /home after login instead of the page they came from.
EOF
#3 created

~$ task list
ID  NAME                AGE  WORDS  OWNER      BLOCKED BY  LABELS
#1  Prototype new logo  2h   32     spider-man             area=brand
#3  Fix login redirect  4s   12                            area=auth
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
task create NAME [-l KEY=VALUE]... [-b ID]... [-f FILE|-]
task list   [-A] [-l SELECTOR] [--global] [--limit N]
task view   ID
task edit   ID [--name NAME] [--set-owner OWNER] [-l KEY=VALUE|KEY-]... [-b ID|ID-]... [-f FILE|-]
task done   ID...
task delete ID...
```

| Command | Does |
|---|---|
| `create` | Adds a task. `-f -` reads the body from stdin; `-b` names tasks that must be done first |
| `list` | Open tasks. `-A` adds done ones, `-l area=auth` filters by label, `--global` shows every project |
| `view` | One task, with its body |
| `edit` | Changes only what you pass. `KEY-` removes a label, `ID-` removes a blocker |
| `done` | Marks tasks finished |
| `delete` | Removes tasks. Their IDs are never reused |

A few rules keep it predictable for an agent:

- **A task is referred to as `#ID`**, so one task's body can point at another.
  On the command line, type the bare number (`task view 3`): an unquoted `#3`
  starts a shell comment.
- **`WORDS`** is the length of the body, so an agent can judge the cost of
  `task view` before running it.
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

This changes three things in `~/.claude/settings.json` (or
`$CLAUDE_CONFIG_DIR`), leaving everything else in it as it was:

- a `UserPromptSubmit` hook that runs `task hook claude-code`;
- a permission rule, `Bash(task:*)`, so the agent can run `task` without asking;
- the status line, so you can see the top open tasks too (see below). On
  Windows the status line is left as it is, because the wrapper needs `sh`.

On every message you send, including one sent while the agent is working, the
hook adds this to the agent's context:

```
You are spider-man. Use this name as OWNER in task.
ID  NAME                AGE  WORDS  OWNER       BLOCKED BY  LABELS
#1  Prototype new logo  2h   32     spider-man              area=brand
```

- **Who the agent is.** The name is the Claude Code session name: the one other
  sessions use to message it.
- **What is on the list.** Five tasks at most, so the cost per message stays
  small. With no tasks, it prints a short note on when to use `task` instead.
- **Renames are followed.** When a session is renamed, by `/rename` or
  automatically, tasks owned under its old name move to the new one at its next
  message.

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
rows. Names longer than 32 characters end in `…`.

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
`task uninstall claude-code`.
