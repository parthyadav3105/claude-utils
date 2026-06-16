---
name: commit-message
description: Suggest a commit message from this session's work. Use when the user asks to write a commit message, and want help drafting a good commit subject/body for the work done in this session.
---

# commit-message

Draft a commit message from what you and the user did this session and show
it. You hold the *why* — the intent, the bug, the trade-off — that the diff
can't show. Capturing it is the whole point.

Before drafting, skim `git log -n 20 --pretty='%h %s'` and follow the repo's
existing style (Conventional Commits, plain imperative, ticket prefixes,
subject case). A style override in the arguments wins over what you infer.

## The seven rules of a great commit message

Adapted from [cbea.ms/git-commit](https://cbea.ms/git-commit/).

**1. Separate subject from body with a blank line.** The first line is the
subject; everything after a blank line is the body. Many git tools rely on
that blank line, so don't skip it.

**2. Limit the subject line to 50 characters.** It's a soft target, not a
hard rule (72 is the real ceiling). If you can't say it in 50, the commit is
probably doing too much.

**3. Capitalize the subject line.** Write `Add CONTRIBUTING.md`, not
`add CONTRIBUTING.md`. (Unless the repo's convention is lowercase.)

**4. Do not end the subject line with a period.** It's a title.
`Fix the build`, not `Fix the build.`

**5. Use the imperative mood in the subject line.** Write it as a command,
as if giving an order. A good test: it should complete the sentence
*"If applied, this commit will …"*

```
Good:  Refactor subsystem X for readability
       Remove deprecated methods
       Release version 1.0.0

Bad:   Fixed bug with Y          (past tense)
       Changing behavior of X    (gerund)
       More fixes for broken stuff
       Sweet new API methods
```

**6. Wrap the body at 72 characters.** Git doesn't wrap text for you, so do
it yourself so the log stays readable in a terminal.

**7. Use the body to explain *what* and *why*, not *how*.** The diff already
shows how. In a few months no one will remember why — the body is where you
tell them. Don't narrate the change:

```
Bad:   Changed foo() to take a Context and updated callers
Good:  Thread request context through foo() so slow queries
       can be attributed to the tenant that triggered them
```

You don't always need a body — a small, obvious change is fine as a subject
alone.

## A full example

```
Summarize changes in around 50 characters or less

More detailed explanatory text, if necessary, wrapped to about 72
characters. The blank line separating the summary from the body is
critical; tools like `log`, `shortlog` and `rebase` can get confused
if you run the two together.

Explain the problem this commit solves. Focus on why you made this
change as opposed to how — the code explains how. Are there side
effects or other unintuitive consequences of this change? Here's
the place to explain them.

 - Bullet points are okay, too
 - Typically a hyphen or asterisk, with a blank line between

If you use an issue tracker, reference it at the bottom:

Resolves: #123
See also: #456, #789
```
