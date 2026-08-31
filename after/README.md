# after

Queue a message for a running [Claude Code](https://claude.ai/code) session
instead of interrupting it. You type the thought the moment you have it; the
agent gets it when it next comes up for air.

```
~$ after 5m go back and add tests for the parser
Queued for 19:31 (in 5m): go back and add tests for the parser

  (the agent carries on with what it was doing, and is told
   about this only once it has finished)
```

Type a message into Claude Code while it is working and it lands *inside* the
current turn. It arrives alongside the next tool result and derails whatever was
in flight. `after` is the other option: say it now, have it delivered at a turn
boundary.

## How it works

Two hooks, one small binary.

When you submit a prompt, a `UserPromptSubmit` hook sees the text first. If it
looks like an `after` command, the message is written to a queue file and the
prompt is **blocked**, so it never reaches the model. That is the whole trick,
and it is not a figure of speech: a queued message costs zero turns and zero
tokens, and leaves nothing in the conversation.

```
you type:   after 5m add tests for the parser
            |- UserPromptSubmit hook
               |- append to the queue file
               |- block  <- the model never sees it

            (the agent keeps working, undisturbed)

            |- Stop hook, when the turn ends
               |- anything due? hand it over as the next instruction
```

A `Stop` hook runs when the agent finishes a turn. Anything owed to that session
and past its time is handed back, and the agent picks it up and continues.

It is a hook rather than a slash command for exactly this reason: a slash
command is a prompt expansion, so its body is sent to the model as a message. It
would cost a turn and enter the thread, which is the very interruption being
avoided.

## Install

By default, the user-level install uses `~/.claude`. If your Claude
configuration lives somewhere else (for example `~/.claude-max`), set
`CLAUDE_DIR` before running the installer.

```bash
curl -fsSL https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/after/install.sh | bash
```

Custom Claude directory:

```bash
CLAUDE_DIR="$HOME/.claude-max" \
  bash -c "$(curl -fsSL https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/after/install.sh)"
```

Windows:

```powershell
iwr -useb https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/after/install.ps1 | iex
```

This drops the binary at `$CLAUDE_DIR/claudeafter` and adds two entries to
`$CLAUDE_DIR/settings.json`. Existing settings and anyone else's hooks are left
alone, and re-running never stacks duplicates. Restart Claude Code to apply.

## Usage

Type it straight into Claude Code, like any message:

```
after 5m   go back and add tests for the parser
after 20m  rebase onto main
after idle write up what we learned

after --list
after --cancel 3
```

| Form | Means |
|---|---|
| `<N>m` | Whole minutes. `5m`, `20m`, `90m`. Minutes are the only unit |
| `idle` | No delay: deliver at the very next turn boundary |
| `--list` | What this session has queued, with an id for each |
| `--cancel <id>` | Drop one, by the id `--list` shows |

Cancelling takes the id, not the text:

```
~$ after --list
2 queued:
  1  19:31     go back and add tests for the parser
  2  due now   rebase onto main

~$ after --cancel 1
Cancelled 1: go back and add tests for the parser
```

Each session numbers its own messages from 1, and a new one always takes the
lowest free number. A queue is only ever a few items deep, so the ids you have
to type stay in single digits instead of climbing forever. Other sessions have
their own 1, 2, 3; you only ever see or cancel your own.

Cancelling never renumbers the messages that remain. Cancel `2` out of `1 2 3`
and the survivors are still `1` and `3`; the next message you queue fills the
gap as `2`. That way an id you can see in a listing always cancels the message
you are looking at, and cancelling echoes the text back so you can see what
went:

```
~$ after --cancel 2
Cancelled 2: rebase onto main
```

`--list` and `--cancel` are blocked before the model like everything else, so
checking or clearing your queue costs nothing and leaves no trace either.

The grammar is deliberately narrow: `after` has to be followed by a bare `<N>m`
or one of the keywords. Anything else is ordinary text and goes to the model
untouched, so *"after you finish, run the tests"* and *"after 30 minutes of
this, check the log"* still mean what they say.

### From another shell

The queue is a file, so a session you cannot type into can still be reached,
including one suspended with ctrl-z where no hook can run:

```bash
claudeafter --list                                    # every session
claudeafter 5m "rebase onto main" --session <id>
claudeafter --cancel 2 --session <id>
```

## What it does not do

**It cannot wake an idle session.** Hooks only run when the session does
something. Queue a message while the agent is sitting at rest and no `Stop` will
fire five minutes later, because nothing is running. It waits until the session
next finishes a turn. The contract is *"at the first turn boundary at or after
this time"*, not *"at this time"*. In practice that is the same thing whenever
the agent is busy, which is the case this exists for; and when it is idle there
is no focus to protect.

**A suspended session is frozen.** ctrl-z stops the process, so no hook of ours
runs and nothing is queued or delivered. Nothing is lost either: the queue is on
disk, and everything that came due arrives at the first turn boundary after you
`fg`. To queue *into* a stopped session, use the command-line form above.

**Uninstalling keeps your queue.** `--uninstall` leaves the queue file in place
on purpose, so reinstalling picks up where you left off. Delete
`$CLAUDE_DIR/after-queue.json` if you want it gone.

## Uninstall

```bash
curl -fsSL https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/after/install.sh | bash -s -- --uninstall
```

Custom Claude directory:

```bash
CLAUDE_DIR="$HOME/.claude-max" \
  bash -c "$(curl -fsSL https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/after/install.sh)" -- --uninstall
```

Windows: `.\install.ps1 -Uninstall`

That removes the binary and both hook entries, and leaves everything else in
`settings.json` as it was.
