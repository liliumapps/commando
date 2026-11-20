---
title: The TUI
---

# The TUI

Commando's interface has two stages: the **search list** and the **fill-in
form**. This page documents every key in each.

## Stage 1 — the search list

When you summon Commando (++ctrl+g++ or `commando`), you land on a fuzzy search
list drawn bottom-up: the best match sits at the bottom, right next to your
input.

```text
  h  kubectl get pods -A             kubectl get pods -A
  c  Rollout status                  kubectl rollout status deploy/<name>
❯ c  Restart deployment              kubectl rollout restart deploy/<name> [-n <ns>]
scope: all────────────────────────────────────────────3/42 commando
Restart a Kubernetes deployment.
kubectl rollout restart deploy/<name> [-n <ns>]
────────────────────────────────────────────────────────
❯ rollout▊
↑↓ move  ←→ edit  ⏎ select  tab scope  ^b bookmark  esc quit
```

Just start typing to filter. The query fuzzy-matches across an entry's
title, description, command, and tags, spanning all four sources at once. Terms
are matched independently, so a multi-word query like `deploy cluster` finds an
entry tagged `deploy` whose description contains `cluster` — the words need not
be adjacent or in one column.

Rows use a compact source code (`c` cheat, `p` project, `h` history, `b`
bookmark), then a title column and a command column. The selected command's
description and full command are shown once in the footer, so the row and detail
area do not repeat the same information.

### Scoping to a source

To narrow the list to a single kind of command, lead your query with a `:`
scope prefix, or press ++tab++ to cycle through the scopes (++shift+tab++ cycles
backward). The active scope shows on the left of the status line; the match
counter and `commando` label stay on the right.

| Prefix | Scope |
|---|---|
| `:c` | Authored cheats |
| `:p` | Project-detected / provider commands |
| `:h` | Shell history |
| `:b` | Bookmarks |
| *(none)* | Everything (the default) |

The rest of the query still searches within the scope: `:h git push` shows only
history entries matching "git push", while a bare `:h` lists all history. A
colon that isn't a scope token — `:z`, `:cd`, or a mid-string `ubuntu:latest` —
is searched literally. Pressing ++tab++ simply rewrites this prefix in your
query, so the two mechanisms are always in sync.

### List keys

| Key | Action |
|---|---|
| ++up++ / ++ctrl+p++ | Move the highlight up |
| ++down++ / ++ctrl+n++ | Move the highlight down |
| ++enter++ | Select the highlighted entry (opens the form, or returns it if it has no variables) |
| ++tab++ / ++shift+tab++ | Cycle the source scope forward / backward |
| ++ctrl+b++ | Bookmark a literal command, or resolve and bookmark a parameterized entry. A secret-looking command is refused |
| ++ctrl+d++ | Delete the highlighted bookmark (bookmark rows only). The first press asks for confirmation; press ++ctrl+d++ again to delete, or any other key to cancel |
| ++left++ / ++right++ | Move the cursor within the query |
| ++backspace++ | Delete before the query cursor |
| ++esc++ / ++ctrl+c++ | Quit without selecting |

!!! note "Why is the list upside down?"

    The list is drawn with the best match nearest your input at the bottom, so
    your eyes and the cursor stay in the same place. Moving "up" the screen
    walks toward less-relevant results.

## Stage 2 — the fill-in form

Selecting an entry with variables opens a compact, bottom-anchored workspace.
Each variable is a **field**; the command summary assembles the command as you
go and stays near the active input even in a tall terminal.

```text
╭────────────────────────────────────────────────────────╮
│ Search git log                                         │
│ command:                                               │
│   git log -n 20 -- <file>                              │
│                                                        │
│ params:                                                │
│   author (optional) = (skipped)                        │
│   count (optional) = 20                                │
│ ❯ file (required, multi) = ‹unset›                     │
╰────────────────────────────────────────────────────────╯
    helper.go
❯ ◯ main.go
❯ file (multi) = ▊
↑↓ candidates  space toggle  ⏎ run  ^p/^n params  ^d clear
```

### Form keys

| Key | Action |
|---|---|
| ++enter++ | Commit this field (adopting the highlighted candidate, or your typed text) and move to the next; on the last field, assemble and finish |
| ++left++ / ++right++ | Move the cursor within the active input |
| ++ctrl+p++ / ++ctrl+n++ | Move to the previous / next field |
| ++tab++ | Jump to the next **required** field (skips optionals) |
| ++shift+tab++ | Jump to the previous required field |
| ++up++ | Move up through the candidate list. When nothing is highlighted, the first press highlights the candidate nearest the input |
| ++down++ | Move down through the candidate list (the first press also just highlights the nearest candidate) |
| ++space++ | On a multi-select field: toggle the highlighted candidate. Otherwise: type a space |
| ++ctrl+s++ | Skip this field (only allowed on optional/skippable fields) |
| ++ctrl+d++ | Clear this field's entire value |
| ++ctrl+r++ | Re-run the field's `src` command: retries a failed source or refreshes its candidates, bypassing any [`cache`d](variables.md#cache-a-slow-lister-cache) result |
| ++backspace++ | Delete before the input cursor |
| ++esc++ | Return to the search list |
| ++ctrl+c++ | Quit entirely |

### Recent fills

When a cheat's form opens and you have run it before (or your shell history has
lines matching it), a **Recent** block appears above the first input with the
value sets you used. While it is shown, it takes these keys; see
[Recent fills](argument-memory.md#recent-fills) for details.

| Key | Action |
|---|---|
| ++up++ / ++down++ | Move within the block; ++down++ on the bottom row hides it |
| ++enter++ | Fill every field, then focus the first field still empty or invalid, or the last field when all are set (one more ++enter++ runs it) |
| ++alt+enter++ / ++ctrl+j++ | Fill every field and run immediately (after full validation) |
| ++ctrl+d++ | Delete the highlighted stored fill (press twice). History rows can't be deleted |
| Any other key | Hides the block and acts as usual (typing goes to the first field) |
| ++esc++ | Return to the search list |

### Field types at a glance

- **Plain field** — free text. A `default` prefills it. Type to overwrite.
- **List field** (`src` set) — offers candidate values. Type to filter, or pick
  with the arrows. Your typed text also appears as the highlighted row, so
  ++enter++ commits exactly what you see. With `strict = true`, only listed
  values are accepted. On a **required** list field, ++enter++ with an empty
  input adopts the top candidate as a quick pick; on an **optional** one it
  leaves the field empty instead, so nothing is preselected.
- Candidate sources load only when their field is focused. A slow source shows
  a loading state without blocking input; failures remain visible and can be
  retried with ++ctrl+r++, which also refreshes a loaded list.
- **Optional field** (inside a `[...]` template segment) — shows `[skip]`.
  Nothing is highlighted until you engage (type, or move the arrows), so
  pressing ++enter++ leaves it empty and drops the whole bracketed fragment.
- **Multi-select field** (`multi = true`) — toggle several values with ++space++;
  they're formatted with `each` and joined with `sep`. When optional, the toggle
  cursor stays hidden until you navigate or select, so it can be left empty.

Candidate previews are loaded asynchronously after a candidate is engaged.
Moving away cancels stale work, and preview failures are shown in the workspace
instead of being mistaken for empty output.

See [Template syntax](templates.md) and [Variables](variables.md) for how these
are declared.

## Finishing

On the last field, ++enter++ first validates every field, not just the
current one: `strict` values must be in the candidate list, values must match
their `pattern`, and required fields must be filled. A field whose candidates
may be stale (a variable it depends on changed) is re-fetched first. If any
field fails, focus jumps to the first invalid field and shows its error.
Otherwise it assembles the command and:

- **Default:** places it on your shell prompt for review (via the Zsh, Bash, or
  Fish widget) or prints it to stdout (when run directly).
- **With `--exec`:** runs it immediately.

Either way, the values you entered are [remembered](argument-memory.md), the
whole form is saved as a [recent fill](argument-memory.md#recent-fills), and the
entry's usage count is bumped for future ranking.
