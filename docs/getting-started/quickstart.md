---
title: Quickstart
---

# Quickstart

This tour assumes you've [installed Commando and wired up the shell
integration](installation.md). We'll go from an empty prompt to a fully
assembled command in under a minute.

## 1. Summon the launcher

Press ++ctrl+g++ at any prompt (or just run `commando`). You'll see a fuzzy
search list, ranked by frecency. Type to filter across **cheats, project
commands, bookmarks, and history** at once — each word you type is matched
independently, so you can mix a tag, title, description word, or command token
in any order. Then press ++enter++ to put the command on your prompt:

![Summon commando, fuzzy-search for a command, and it lands on the prompt](../assets/gifs/search.gif)

<p class="gif-caption">Searching “status”, selecting <code>git status</code>, and it lands on the prompt — nothing runs until you press Enter.</p>

The list, annotated:

```text
  p  docker compose up -d            docker compose up -d
  p  npm build                       npm run build
❯ c  Search git log                  git log [--author=<author>] [-n <count>] -- <file>
scope: all────────────────────────────────────────────3/42 commando
Search git log by author for a file.
git log [--author=<author>] [-n <count>] -- <file>
────────────────────────────────────────────────────────
❯ log auth▊
↑↓ move  ←→ edit  ⏎ select  tab scope  ^b bookmark  esc quit
```

Want just one source? Lead the query with a `:` prefix — `:c` for cheats, `:p`
for project commands, `:h` for history, `:b` for bookmarks — or press ++tab++
to cycle through the scopes. See
[The TUI](../guide/tui.md#scoping-to-a-source) for the details.

## 2. Fill in the variables

Select an entry with ++enter++. If it has variables, Commando opens a fill-in
form with a command preview that updates as you type:

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

Working the form:

- **Type** a value, or use ++up++/++down++ to pick from candidates.
- **Optional fields** show `[skip]` — press ++ctrl+s++ to skip, and the whole
  `[...]` fragment vanishes from the command (no dangling flags).
- **Multi-select fields** let you toggle several values with ++space++.
- Move within text with ++left++/++right++; move between fields with
  ++ctrl+p++/++ctrl+n++. ++tab++ jumps to the next required field.

## 3. Review and run

On the last field, ++enter++ finishes assembly. The command is placed on your
shell prompt:

```console
$ git log -n 20 -- "a.go" "b.go"▊
```

Nothing has run yet. **Review, edit if you like, then press ++enter++ yourself.**
This is the core of Commando's design — the shell owns execution, so your `cd`,
environment variables, and aliases all keep working.

!!! tip "Run it directly instead"

    If you want Commando to execute the command for you (handy in scripts):

    ```sh
    commando search --exec
    ```

## Try these next

<div class="grid cards" markdown>

-   :material-plus-box: __Author a cheat__

    ---

    ```sh
    commando add
    ```

    Commando parses your template, discovers the variables, and prompts for each
    one's options.

    [:octicons-arrow-right-24: Cheat sheets](../guide/cheat-sheets.md)

-   :material-bookmark-plus: __Bookmark a command__

    ---

    Press ++ctrl+b++ on any list entry (parameterized entries are resolved
    before saving), or:

    ```sh
    commando bookmark add --tags "k8s" \
      "kubectl rollout restart deploy/web"
    ```

    [:octicons-arrow-right-24: Bookmarks](../guide/bookmarks.md)

-   :material-history: __Search your history__

    ---

    ```sh
    commando history docker
    ```

    Read live from your shell's history file.

    [:octicons-arrow-right-24: Shell history](../guide/shell-history.md)

</div>
