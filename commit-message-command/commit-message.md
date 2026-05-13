---
description: Suggest a commit message from this session's work, then offer to commit
argument-hint: [optional extra context, e.g. issue ref or emphasis]
model: claude-sonnet-4-6
---

# /commit-message

## Roles

Two cooperating agents — parent (judgment) and sub-agent (execution).

### You — the parent agent (author)

**Think like a senior Google distinguished engineer — you know exactly
what needs to be told.**

You are the agent that did the work in this conversation. You hold the
*why* — intent, discarded alternatives, bug being fixed, trade-off
accepted. A diff cannot reconstruct it. Capturing it well is the whole
point of a good commit message.

Draft the message *content*, present it, confirm, redraft on edit. Do
not call `git`. You produce content; the sub-agent typesets it and
persists it.

### The sub-agent (executor)

Receives concrete instructions and a finished draft. Does not exercise
editorial judgment over content. It:

- Reads `git log`, infers the local convention, rewrites the draft
  *minimally* to match.
- On confirmation, stages and commits under the user's git identity.

All git output stays inside the sub-agent's transcript.

### Output discipline

Visible output to the user is exactly: the proposed message (fenced),
the `Convention:` line, the `Atomicity:` line if applicable, the
confirmation prompt, and finally the resulting short SHA. No preamble
("I'll now…", "Sure!"), no narration of the steps below, no closing
summary.

---

## Step 1 — Draft from session context (no tool calls)

Recall what you and the user did in this conversation. The diff shows
*what* changed; only this conversation knows *why*. Draft the message
from memory — do not pre-fetch the diff.

If `$ARGUMENTS` is non-empty, treat it as additional context (issue ref,
framing, or explicit style override). An explicit style override there
**takes precedence** over whatever the styling sub-agent infers from
`git log` — flag this clearly when handing off in Step 2.

If your session memory is genuinely thin (fresh session, or files edited
outside the agent), say so when handing off — the sub-agent will read
the diff to fill in.

### The skill: what makes a commit message good

A commit message is durable documentation read by reviewers, `git
blame`, future-you, and on-call engineers who lack all your current
context. Every message has a **subject** (the first line) and may have a
**body** (paragraphs after a blank line) for context that doesn't fit in
50 chars.

### The seven rules (cbea.ms baseline)

These are universal. The repo's local convention layers on top in
Step 2; it does not replace them.

1. **Separate subject from body with a blank line.** Many git tools
   only parse title and body correctly when the blank line is present.

2. **Limit the subject to 50 characters** (hard cap 72). The 50-char
   target is a forcing function: difficulty hitting it is a signal that
   the commit is doing too many things (see *atomicity*).

3. **Capitalise the subject.** Unless the repo uses Conventional
   Commits with lowercase subjects — Step 2 will lowercase if so.

4. **No trailing period on the subject.** It's a title, not a sentence.

5. **Imperative mood in the subject.** Write it as a command: *"Add
   user authentication"*, *"Fix off-by-one in pagination"*, *"Refactor
   cache invalidation"*. Apply the **sentence test** — the subject must
   complete:

   > *"If applied, this commit will _________."*

   If it doesn't read naturally, rewrite. The imperative applies only
   to the subject; the body is regular prose.

6. **Wrap the body at 72 characters.** Git does not auto-wrap.

7. **Body explains *what* and *why*, not *how*.** The diff already
   shows *how*. The body's job is the context the diff cannot convey:
   the problem, the rejected alternative, the prior incident, the
   trade-off, the related ticket or RFC.

### Specificity over vagueness

The single most common failure mode is a vague subject. Do not write
`update`, `fix bug`, `changes`, `wip`, `misc`, `improvements`,
`cleanup`, `tweaks`, or any close paraphrase. They all fail the
sentence test and tell the reader nothing the file path didn't already
say.

The upgrade you should perform:

- Bad: `fix bug` → Good: `Fix off-by-one when paginating empty result sets`
- Bad: `update auth` → Good: `Reject expired refresh tokens at session start`
- Bad: `cleanup` → Good: `Remove unused legacy SAML middleware`
- Bad: `wip image stuff` → Good: `Lazy-load product gallery images on scroll`
- Bad: `Changed style` → Good: `Wrap pricing card at narrow viewports`

Vague verbs (`update`, `change`, `modify`, `improve`) are acceptable
*only* with a specific object and effect. *"Update README"* is bad.
*"Update README install steps for Node 20"* is fine.

### When to add a body

Add a body when any of these are true; otherwise skip it.

- The *why* is non-obvious from the diff alone.
- A reviewer needs to know why the obvious alternative was rejected.
- There is an incident, ticket, RFC, or external reference worth
  recording.
- The change has a side effect or migration step the reader needs.
- The change is intentionally narrow and the body explains why a
  broader fix is not in scope.

Body guidance:

- Lead with problem or motivation, not implementation.
- One short paragraph is often enough; multi-paragraph is fine when the
  change deserves it.
- Bullets (`-`, consistent throughout) for distinct points.
- **Do not narrate the diff.** *"Changed `foo()` to take a `Context`
  argument and updated all callers"* is restating the diff. *"Threading
  request context through `foo()` so we can attribute slow queries to a
  tenant"* explains the *why*.
- Footers (`Closes #123`, `Refs: ABC-456`, `Signed-off-by:`) only when
  the repo's convention uses them or `$ARGUMENTS` names a ticket. The
  sub-agent will format them; you can mention the ref in your draft and
  let it be styled.

### Atomicity

If the work spans clearly unrelated changes — a feature plus an
unrelated bug fix, or a refactor plus a behaviour change — and the only
honest subject would need *"and"* or a comma, that is a signal the work
should be **two commits, not one**. Mention this in your handoff to the
sub-agent so it appends an `Atomicity:` warning line. Do not silently
bundle.

### Forbidden in the commit message

- Filler: *maybe*, *I think*, *kind of*, *just*, *some*, *a bit*.
- AI self-references: *as an AI*, *generated by*, *I have*.
- Co-author / "Generated with" trailers — never include unless the
  user explicitly asks.
- Past tense (*"Added"*, *"Fixed"*) or gerunds (*"Adding"*, *"Fixing"*)
  in the subject.
- Trailing period on the subject.
- Marketing tone (*"Greatly improves"*, *"Massively simplifies"*).
- Invented issue numbers, tickets, scopes, or types not present in the
  conversation or `$ARGUMENTS`.

Hold the draft in working memory. Do not show it yet — Step 2 applies
repo-specific style first.

## Step 2 — Hand off to the styling sub-agent

Use the Agent tool with `subagent_type: "general-purpose"` and
`model: "sonnet"`. Use this prompt verbatim, substituting each
bracketed slot from your Step 1 work:

> You are an executor. Take the draft below, detect this repo's commit
> convention from `git log`, and rewrite the draft *minimally* to match.
> Preserve meaning. Do not invent or reword content; only reformat.
>
> **Draft:**
> ```
> [draft subject]
>
> [draft body if any]
> ```
> **User extra context:** [$ARGUMENTS]
> **Session memory thin:** [yes / no]
> **Parent atomicity concern:** [yes / no — and the reason if yes]
>
> Steps:
>
> 1. Run, in one Bash call:
>    `git rev-parse --is-inside-work-tree && git log -n 20 --pretty=format:'%h %s' && echo --- && git log -n 3`
>    If not in a git repo, return `ERROR: not in a git repo` and stop.
>
> 2. Detect from the sample:
>    - **Format:** Conventional Commits (`type(scope): subject`, types
>      `feat`/`fix`/`refactor`/`docs`/`chore`/`test`/`style`/`perf`/
>      `build`/`ci`/`revert`), plain imperative, ticket-prefixed
>      (`[ABC-123] subject`), emoji-prefixed, or mixed.
>    - **Subject case:** capitalised vs. lowercase.
>    - **Body norms:** usually present / absent / mixed.
>    - **Footer norms:** `Closes #N`, `Refs:`, `Signed-off-by:`, none.
>    - **Scope vocabulary** if Conventional Commits.
>    - Weak signal (<5 usable commits or fully inconsistent) → keep the
>      draft as-is (cbea.ms baseline).
>
> 3. Apply convention to the draft:
>    - **If user extra context names an explicit style override, honor
>      it over the inferred convention** (e.g. "use lowercase",
>      "no scope", "skip prefix").
>    - Otherwise reformat subject case and prefix to match the inferred
>      convention.
>    - If repo norms are subjects-only and the body adds nothing
>      non-obvious, drop the body.
>    - If `$ARGUMENTS` names an issue ref AND the log shows footer
>      style, append a footer in that exact format.
>    - Never fabricate scopes, tickets, or types not present in the
>      input or `$ARGUMENTS`. If unsure, omit.
>
> 4. If session memory was thin, also run
>    `git diff --staged --stat && git diff --stat`. If the draft clearly
>    contradicts the diff, note it on `Atomicity:`.
>
> 5. Return ONLY this, no preamble:
>
>    ```
>    [final subject]
>
>    [final body if any]
>    ```
>
>    Then: `Convention: <one-line summary of detected style + key choice>`
>    Then (only if relevant or parent flagged): `Atomicity: <warning>`

Show the sub-agent's output to the user verbatim. If the sub-agent
returns `ERROR: not in a git repo` (or any other `ERROR:` prefix), relay
the error in one line and stop — do not proceed to Step 3.

## Step 3 — Confirm with the user

Use the AskUserQuestion tool:

- **header:** `Commit?`
- **question:** `Use this commit message?`
- **options:**
  - `Yes` — commit with this message
  - `Edit` — tell me what to change, then redraft
  - `No` — leave the working tree as-is

Branching:

- **Yes** → Step 4.
- **Edit** → ask in one short follow-up what to change. Adjust the
  draft yourself (sub-agent restyles, never authors). Re-run Step 2 with
  the new draft. Loop until Yes or No.
- **No** → stop. Confirm the working tree is unchanged.

## Step 4 — Hand off to the commit sub-agent (only on confirmed "Yes")

Use the Agent tool again, `subagent_type: "general-purpose"`,
`model: "sonnet"`. Pass the final approved message verbatim.

Use this sub-agent prompt verbatim (substitute the message):

> You are an executor. Commit the message below under the user's
> existing git identity.
>
> **Message:**
> ```
> [final message verbatim]
> ```
>
> Steps:
>
> 1. Run `git status --short`.
> 2. If anything is already staged → skip staging; commit only what is
>    staged. Do not silently add unstaged changes.
> 3. If nothing is staged → use AskUserQuestion:
>    - `Stage tracked changes` → run `git add -u` (modifications and
>      deletions of tracked files only; no untracked).
>    - `Cancel` → return `CANCELLED` and stop.
>    Never run `git add -A` or `git add .`. (Risk: secrets, build
>    artifacts, unrelated files.) If the user wants surgical staging,
>    they will cancel, stage manually, and re-run `/commit-message`.
> 4. Commit with a HEREDOC to preserve multi-line formatting:
>    ```bash
>    git commit -m "$(cat <<'EOF'
>    [message verbatim]
>    EOF
>    )"
>    ```
> 5. If a hook fails, return the hook output verbatim and stop. Do not
>    retry, do not pass `--no-verify`.
> 6. On success, run `git log -1 --oneline` and return ONLY that line
>    (short SHA + subject).

Report the sub-agent's result in one line. Stop.

---

## Non-negotiables (operational)

- Never push, amend, or `--no-verify`.
- Never claim the commit is done before a SHA is reported.

(Content rules — no AI trailers, no invented IDs, no past tense — live
in *Forbidden in the commit message* and the sub-agent prompts.)
