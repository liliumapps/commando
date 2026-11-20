---
title: Argument memory
---

# Argument memory

Commando remembers the values you enter for a variable and offers them back next
time as ranked candidates — most useful values first. **Memory is on by
default**; you don't have to configure anything.

![Run a command once; on the next summon the value you entered is offered as a remembered candidate](../assets/gifs/argument-memory.gif)

<p class="gif-caption">Enter a branch name once — on the next summon it's waiting as a remembered candidate, no retyping.</p>

## How it works

Every time you finish a command, Commando records each variable's value in
`~/.local/share/commando/memory.toml`, keyed by
`(command, variable, scope)`. The next time you fill in that variable, the
remembered values appear as candidates above the input, **most-recently-used
first**.

The command is identified by its title and template, or by its
[`id`](cheat-sheets.md#stable-ids) when the cheat sets one. Set an `id` if you
expect to edit a cheat's title or template and want to keep its memory.

```toml
# ~/.local/share/commando/memory.toml (excerpt — plain and editable)
[[entry]]
id        = "..."
var       = "branch"
scope     = "project:/Users/you/repo"
value     = "feature/login"
freq      = 7
last_used = 1751500000
```

## Frecency ranking

Commando scores how "hot" something is by **frecency**: its use count weighted
by how long ago it was last used (the same idea behind `z`/zoxide).

| Last used | Weight |
|---|---|
| within the last hour | count × 4 |
| within the last day | count × 2 |
| within the last week | count × 0.5 |
| older | count × 0.25 |

Frecency orders the search list when you haven't typed anything, and breaks
ties among close matches when you have, so commands you run often and recently
rise. For remembered values, it decides which ones are kept: each
`(command, variable, scope)` keeps its 25 hottest values, so a value you use
constantly isn't pushed out by a burst of one-offs. The form itself lists the
kept values most-recently-used first.

## Scopes — the `remember` field

The `remember` field on a variable controls **where** a value is remembered:

| `remember` | Behavior |
|---|---|
| *(unset)* | **Default.** Remember globally — offered everywhere. |
| `global` | Same as unset: remembered across every directory. |
| `project` | Remembered per git project (keyed by the repository root). |
| `dir` | Remembered per working directory. |
| `none` | **Opt out** — never store values for this variable. |

```toml
[[cmd.var]]
name     = "branch"
src      = "git branch --format='%(refname:short)'"
remember = "project"   # branch names are per-repo, so scope memory to the repo
```

Choosing a scope is about **relevance**: a branch name only makes sense inside
its repo (`project`), a build output path might be per-directory (`dir`), while
your favorite AWS region is the same everywhere (`global`).

## Secrets — always use `remember = "none"`

!!! danger "Never remember sensitive input"

    Any variable that can hold a token, password, or other secret should set
    `remember = "none"` so its values are **never** written to
    `memory.toml`:

    ```toml
    [[cmd.var]]
    name     = "token"
    remember = "none"
    ```

    As a backstop, a value that looks like a secret is never remembered, even
    without `remember = "none"`: the [redaction](shell-history.md#redaction)
    patterns and built-in detectors are checked against the value and against
    `<variable>=<value>`, so a variable named `token` or a value shaped like an
    API key is skipped. Don't rely on detection alone; `remember = "none"` is
    the reliable control.

## How remembered values behave in the form

- They appear as candidates in the list above the input, ordered most-recent
  first. They are **never** typed into the input automatically.
- For a **list field** (one with a `src` command), pressing ++enter++ on an empty
  input picks the top candidate.
- For a **plain field** (no `src`), ++enter++ never auto-fills a remembered
  value — select one deliberately with ++up++ then ++enter++.
- An author-defined `default` still pre-fills the input regardless of memory.

## Recent fills

Besides each variable's values, Commando remembers every completed cheat form
**as a whole**. The next time you open that cheat, a **Recent** block sits
directly above the first input and offers the value sets you used before, so a
five-variable command you run with the same values every day is one keystroke
away:

```text
  recent
    pod=web-1  namespace=shop  since=1h   (1×) from history
  ❯ pod=api-7d9f  namespace=payments   (4×, 2h ago)
❯ pod = ▊
  ↑↓ recent · ⏎ fill · alt+⏎ run · ^d delete · type skip · esc back
```

Each row lists `name=value` pairs in field order, with how often and how recently the
set was used. Stored fills come first, hottest by [frecency](#frecency-ranking);
fills found in shell history follow, newest first. Up to five rows show at once
and the block scrolls; in a short terminal it shrinks or hides before the
candidate list does.

| Key | Action |
|---|---|
| ++up++ / ++down++ | Move within the block. ++down++ on the bottom row hides the block |
| ++enter++ | Fill every field, then focus the first field still empty or invalid, or the last field when everything is set, so one more ++enter++ runs it |
| ++alt+enter++ / ++ctrl+j++ | Fill every field and run at once, after the same validation as a normal run |
| ++ctrl+d++ | Delete a stored fill: the first press asks, a second press deletes. History rows can't be deleted |
| Typing | Hides the block; the keystroke goes to the first field |
| ++esc++ | Return to the search list |

A fill sets every remembered field. An optional field the fill doesn't include
is skipped (it was skipped that time too); multi-select and `remember = "none"`
fields are left for you to fill in, and validation stops at them if they are
required.

### What gets stored

Running a cheat form (not bookmarking it) records one row in
`~/.local/share/commando/fills.toml`:

```toml
[[fill]]
id     = "..."          # the same entry key memory uses
scope  = "global"
values = { namespace = "payments", pod = "api-7d9f" }
count  = 4
last   = 1763631000
```

- An identical value set bumps `count` and `last` instead of adding a row.
- The **scope** is the narrowest one among the cheat's remembered variables: a
  cheat with one `remember = "project"` variable keeps its fills per project, a
  `dir` variable makes them per directory, otherwise they are global.
- Each `(cheat, scope)` keeps its 20 hottest fills by frecency.
- Only authored cheat templates with at least one variable record fills;
  history, bookmarks, and project commands don't.

### Matching shell history

When the form opens, Commando also looks through your newest 5,000 history
commands, in the background, for lines that are instances of the cheat's
template:

- literal text must match exactly, but any run of spaces matches any run of
  spaces;
- each `<var>` matches one shell word: a run of non-space characters, or a
  single- or double-quoted string. Quotes around a plain word are dropped
  (`'payments'` becomes `payments`); a value that needs them, such as
  `"fix login bug"`, keeps them, since form values are inserted as typed;
- an optional `[ ... ]` segment may be absent;
- a variable used twice must have the same value both times.

So `kubectl logs <pod> -n <ns>` matches `kubectl logs api-7d9f -n payments` but
not `kubectl logs api-7d9f` or `echo kubectl logs api -n x`. A template with no
fixed word outside optional segments (such as a bare `<cmd>`) is never matched.
Matches with the same values form one row, counted per matching line; a match
equal to a stored fill merges into it. History fills are shown, never stored.

### Privacy

Fills follow the same rules as single values, applied to the whole record:

- `remember = "none"` values are never stored, nor taken from history.
- If any value looks like a secret (the [redaction](shell-history.md#redaction)
  patterns and detectors, checked against the value and `<variable>=<value>`),
  the whole fill is skipped. Secret-looking history lines are skipped as well.

## Editing or clearing memory

`memory.toml` and `fills.toml` are plain TOML files — open them, edit them, or
delete entries by hand. To wipe all memory, run `commando state clear memory`
(which clears both files) or delete the files; Commando recreates them as
needed. A file that fails to parse is moved aside to
`<name>.corrupt-<unix time>` and Commando starts with it empty.

## Next

<div class="grid cards" markdown>

-   :material-tune-variant: __[Variables](variables.md)__

    Where `remember` sits among a variable's other options.

-   :material-file-cog: __[Files & storage](../reference/files.md)__

    The full layout of Commando's plain-file state.

</div>
