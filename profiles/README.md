# Claude Code Profile Manager

Run different Claude Code logins in different directories. A **profile** is one
full Claude config — its own login, settings and history. One profile is
**global** (used wherever nothing more specific matches); any directory can be
mapped to another, including every folder beneath it.

```
~$ cd ~/dev/compass && claude-profile
compass -> ~/.claude-profiles/compass

~$ cd ~/dev/knotes && claude-profile
default -> ~/.claude

~$ claude-profile --list
* default  /home/you/.claude
  compass  /home/you/dev/compass
```

`*` marks the global profile. `default` is Claude's own `~/.claude` — a profile
like any other, which is why it is always listed.

You keep typing `claude`. Nothing else changes.

## How it works

Claude keeps **all** of its state — credentials included — in `$CLAUDE_CONFIG_DIR`
(default `~/.claude`). Point that variable at another directory and you get
another profile, which Claude creates and logs into on first run.

The catch: it must be set *before* the process starts, so it cannot come from a
project's `.claude/settings.json`, and `SessionStart` hooks run too late. So the
installer defines a `claude` shell function — the only seam early enough:

```bash
claude() {
  name="$(_claude_profile_for)" || name=default             # longest match in ~/.claude-profiles/profiles
  if [ "$name" = default ]; then
    env -u CLAUDE_CONFIG_DIR claude "$@"                    # ~/.claude
  else
    CLAUDE_CONFIG_DIR="$CLAUDE_PROFILES_DIR/$name" command claude "$@"
  fi
}
```

Your shell rc gains exactly one sourced line; the mappings live in a plain text
file you can read and edit.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/parthyadav3105/claude-utils/main/profiles/install.sh | bash
```

Needs `bash` (not `sh`). Nothing to download or keep — the installer copies a
small `switch.sh` into `~/.claude-profiles/` and adds one line to your shell rc.
Or clone the repo and run `./install.sh`.

There are no flags; the list of profiles *is* the menu, and every row is
something you can change:

```
  Claude Code Profile Manager

  ──────────────────────────────────────

  Enter opens what you can do with a row.
  * is the global profile, used everywhere not listed.

       PROFILE    WHERE  ↑↓ Enter
  ▶  * default    ~/.claude
       compass    ~/dev/compass
       work       ~/dev/acme             (no login yet)

     Add new    Uninstall    Quit (ctrl+c)
```

One row per profile, never two for the same one. `default` is always there, and
`*` moves to whichever profile is global.

Pick **Add new**, give it a path — Tab cycles through matching directories — and
a profile name (defaults to the directory's basename). Then open a new shell,
`cd` in and run `claude` — it prompts for login as that profile by itself.

## The row menu

Pick any row — `default` included — and the list turns into the same two things
you can do with it:

```
  work — ~/dev/acme  ↑↓ Enter
  ▶  Mark as global   use 'work' in every directory not listed
     Delete           unmap ~/dev/acme

     Cancel
```

**Mark as global** moves the `*` to that row: the way to change what every
unlisted directory uses is to pick a profile you can already see. **Delete**
unmaps the directory and asks whether to delete that profile's stored login too.

The verb is *global*, never *default*, because `default` is the name of a
profile. "Make `parth` the default" is ambiguous; "make `parth` global" is not.

Every row gets the identical menu — one shape to learn, and no row that behaves
unlike its neighbours. Only the hints differ, because only the consequences do:

```
  default — ~/.claude                    parth — (global only)
  ▶  Mark as global   already global     ▶  Mark as global   already global
     Delete    Claude's own profile —       Delete    stop 'parth' being global
               nothing to remove
     Cancel                                 Cancel
```

Both entries are always present, even where they would do nothing — the hint says
so rather than the menu quietly changing shape. Marking `default` as global is
how you undo a global profile.

There is no "change this directory's profile" either, because **Add new** already
is one: give it a directory that is already mapped and it replaces that entry,
with the current profile pre-filled as the answer. Name the profile `default` to
pin a directory back to Claude's own `~/.claude`.

The list comes back after every action — redrawn in place, with what just
happened above it — so map several directories, change one, or just look at what
is mapped and **Quit** (or ctrl+c). Re-running the installer is always safe too:
shell-rc lines are never duplicated, and re-adding a directory updates it in
place.

## The rows

Arrow keys move through the profile rows and along the buttons underneath — the
one you are on is highlighted — and Enter picks.

| Row | What it is |
|---|---|
| `default` | Claude's own `~/.claude`. Always listed, and can never be deleted |
| a directory | A profile and the directory (plus children) mapped to it |
| `(global only)` | A profile that is global without owning a directory |
| **Add new** | Map a directory to a profile — or re-point one that is mapped |
| **Uninstall** | Drops the `claude()` function entirely; keeps every mapping and login |
| **Quit** | Leaves — ctrl+c does the same |

`default` is a real profile name, so a global profile plus one directory pinned
back to `default` is a perfectly good setup: the longest matching directory wins.

**Delete** asks whether to remove that profile's stored login too. The answer
defaults to no, so mapping it again later reuses the existing login instead of
making you log in again — and if you keep it, the installer prints the `rm -rf`
command in case you want it gone later. A profile still mapped somewhere else
keeps its login without asking.

Nested mappings work — the longest matching directory wins, so `~/dev` on one
profile and `~/dev/special` on another behave as you'd expect.

Two shell functions come with it: `claude-profile` prints the profile for the
current directory, and `claude-profile --list` prints every profile.

## What lives where

| Path | Contents |
|---|---|
| `~/.claude` | The `default` profile, never touched by this tool |
| `~/.claude-profiles/switch.sh` | The wrapper, sourced from your shell rc |
| `~/.claude-profiles/profiles` | Mappings, one `directory\|profile` per line (`/` names the global one) |
| `~/.claude-profiles/<name>/` | That profile's credentials, settings, history |
| `~/.bashrc` or `~/.zshrc` | One sourced line, added once |

Set `CLAUDE_PROFILES_DIR` before installing to keep all of that somewhere else.

Each profile is a full, independent config directory: its own settings, theme,
skills, MCP servers and history. Nothing is shared with `default`, and nothing is
symlinked. Copy anything you want in common, e.g.
`cp ~/.claude/settings.json ~/.claude-profiles/compass/`.

## Limits

Only shells that read your rc file get the function. Claude launched from a VS
Code extension, the desktop app or cron does **not** go through it and will use
`~/.claude` — including when another profile is global here.

If another `claude` alias or function is already defined in your shell rc, the
installer warns you — whichever is defined last wins.

## Uninstall

Run the installer and choose **Uninstall**. That removes the wrapper and the shell-rc line;
logins stay in `~/.claude-profiles/` so you can wire it back later. Delete a
profile for good with `rm -rf ~/.claude-profiles/<name>`.
