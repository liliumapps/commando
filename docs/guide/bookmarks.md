---
title: Bookmarks
---

# Bookmarks

A bookmark is a command you explicitly saved — with optional **tags** and a
**note** — all fuzzy-searchable. It's the answer to "I know I figured this out
once, but I can never remember the exact incantation."

## Why bookmarks?

History is everything you've run; bookmarks are the handful of commands you want
to *keep*. Because the tags and note are searchable, you can find a command by
**describing** it rather than recalling its syntax:

```sh
commando bookmark add --tags "k8s deploy" --note "prod rollout" \
  "kubectl rollout restart deployment/web -n prod"
```

Later, searching `prod rollout` or `k8s` surfaces it — even though neither word
appears in the command itself. Bookmarks also sort to the **top** of the list.

## From the TUI

Press ++ctrl+b++ on any highlighted list entry. Literal commands are bookmarked
immediately. A parameterized entry opens its normal fill-in form and bookmarks
the assembled command, so the saved bookmark is directly runnable. You can
refine its tags and note afterward with the CLI. Highlight a bookmark and press
++ctrl+d++ twice to remove it without leaving the TUI: the first press asks for
confirmation, and any other key cancels.

Bookmarks are stored in plain text, so a command that looks like it contains a
secret is refused. The check uses the
[redaction patterns and built-in detectors](shell-history.md#redaction).

## From the command line

### Add

```sh
commando bookmark add [--tags "a,b,c"] [--note "..."] [--force] <command>
```

- `--tags` — comma-separated, searchable keywords.
- `--note` — a free-form searchable note.
- `--force` — save the command even if it looks like it contains a secret.
  Without it, such a command is refused with an error.
- The command itself is everything after the flags.

```sh
commando bookmark add --tags "docker,cleanup" \
  --note "reclaim disk" \
  "docker system prune -af --volumes"
```

### List

```sh
commando bookmark list        # also: commando bookmark ls
```

Each line shows the command, its `[tags]`, and its `— note`.

### Remove

```sh
commando bookmark remove "docker system prune -af --volumes"   # also: rm
```

Removal matches the **exact** command string.

!!! tip "`bm` is shorthand for `bookmark`"

    All the subcommands work under the shorter alias too:

    ```sh
    commando bm add --tags git "git commit --amend --no-edit"
    commando bm ls
    ```

## Where bookmarks live

Bookmarks are stored in `~/.config/commando/bookmarks.toml` — plain, editable
TOML:

```toml
[[bookmark]]
cmd     = "kubectl rollout restart deployment/web -n prod"
tags    = ["k8s", "deploy"]
note    = "prod rollout"
created = 1751500000
```

Edit it by hand if you prefer; Commando reads it on every run.

## Next

<div class="grid cards" markdown>

-   :material-console-line: __[CLI reference](../reference/cli.md)__

    Every command and flag, including `bookmark`.

-   :material-file-cog: __[Files & storage](../reference/files.md)__

    Where bookmarks sit among Commando's other files.

</div>
