---
title: CLI reference
---

# CLI reference

```text
commando — context-aware command launcher

Usage:
  commando [command]

Commands:
  (no args)          Open the interactive search TUI
  search [--print]   Open the TUI; --print writes the chosen command to stdout
  init <shell>       Print integration for zsh, bash, or fish
  run                Alias for search
  add                Interactively add a cheat entry
  history [query]    Search your shell history (read live from the history file)
  doctor             Diagnose config, state, history, providers, and shell setup
  state clear ...    Clear remembered arguments or usage ranking
  promote [flags]    Promote a command or history entry into a managed cheat
  bookmark ...       Manage bookmarks: add/list/remove (also: bm)
  version            Print the version
  help               Show this help
```

Run with no arguments to open the search TUI. `commando --help` prints the usage
above.

---

## `commando` / `commando search`

Open the interactive search TUI. Running `commando` with no arguments is
identical to `commando search`.

```sh
commando
commando search [--print] [--exec]
```

| Flag | Effect |
|---|---|
| `--print` | Write the chosen command to stdout. Used by the shell widget. (The command is written to stdout regardless; the flag exists for explicitness.) |
| `--exec` | Run the chosen command directly instead of printing it. |

**Behavior:** you search, optionally fill in a form, and Commando emits the
assembled command. By default it prints to stdout (the zsh widget puts it on
your prompt). With `--exec` it runs the command via your shell.

In the list, type to fuzzy-filter (each term matches independently across an
entry's title, description, command, and tags) and lead the query with a
`:c`/`:p`/`:h`/`:b` prefix — or press ++tab++ — to scope to cheats, project
commands, history, or bookmarks. See [The TUI](../guide/tui.md#scoping-to-a-source)
for the full key reference.

**Exit codes:**

| Code | Meaning |
|---|---|
| `0` | A command was chosen and emitted (or run). |
| `130` | You cancelled (Esc / Ctrl-C) — nothing chosen. |
| `1` | An error occurred. |

---

## `commando init`

Print shell integration to stdout for Zsh, Bash, or Fish.

```sh
commando init zsh
commando init bash
commando init fish
```

Add the matching command to your shell config:

```sh
eval "$(commando init zsh)"    # ~/.zshrc
eval "$(commando init bash)"   # ~/.bashrc
commando init fish | source    # ~/.config/fish/config.fish
```

The emitted snippet defines the summon widget and binds it to your configured
`key_binding` (default ++ctrl+g++). See
[Installation & setup](../getting-started/installation.md#set-up-shell-integration).

---

## `commando run`

An alias for [`commando search`](#commando-commando-search). Accepts the same
flags.

```sh
commando run [--print] [--exec]
```

---

## `commando add`

Interactively author a cheat entry. Commando prompts for a title, optional
description, template, and tags, then parses the template to discover each
variable and prompts for its options. The validated entry is appended to
`~/.config/commando/cheats/user.toml`.

```sh
commando add
```

Prompts, in order:

1. **Title** — required; shown in the result list.
2. **Description** — optional; shown for the selected command.
3. **Command template** — required; use `<var>` and `[optional <var>]`.
4. **Tags** — comma-separated, optional.
5. For each discovered variable: `src`, `default`, multi-select (y/N), and
   remember scope (`none`/`global`/`project`/`dir`, default `none`).

See [Cheat sheets](../guide/cheat-sheets.md#authoring-interactively-with-commando-add).

---

## `commando history`

Search your shell history, read live from the history file. Commando never
stores a copy.

```sh
commando history [query...] [--limit N]
```

| Flag | Default | Effect |
|---|---|---|
| `--limit N` | `50` | Maximum results to print. |

- The query is one or more words, matched case-insensitively as a substring.
- Results print **newest-first**.
- Flags may appear before or after the query — both
  `commando history --limit 3 docker` and `commando history docker --limit 3`
  work.

```sh
commando history docker
commando history --limit 100 kubectl rollout
```

See [Shell history](../guide/shell-history.md).

---

## `commando doctor`

Inspect the active paths, config and cheat files, state store, shell history,
project providers, shell support, and binary discovery.

```sh
commando doctor
```

Each line is marked `ok`, `warn`, or `error` and includes the path or an
actionable detail. Warnings describe reduced functionality and still exit `0`;
an error exits `1`.

---

## `commando state clear`

Reset Commando's learned state without deleting configuration or bookmarks.

```sh
commando state clear memory
commando state clear usage
commando state clear all
```

`memory` forgets parameter values, `usage` resets command and candidate
frecency, and `all` clears both.

---

## `commando promote`

Append a literal command or parameterized template to
`~/.config/commando/cheats/user.toml`.

```sh
commando promote [--title "..."] [--desc "..."] [--tags "a,b"] <template>
commando promote --title "Pod logs" --tags "k8s,logs" \
  'kubectl logs <pod> [-n <namespace>]'
```

| Flag | Effect |
|---|---|
| `--title` | Short result title. Defaults to the template. |
| `--desc` | Searchable description. |
| `--tags` | Comma-separated searchable tags. |

Discovered variables are written with `remember = "none"` so promoting an
existing command cannot start persisting parameter values unexpectedly. Edit
the generated cheat when a variable is safe to remember.

---

## `commando bookmark`

Manage saved commands. Also available as `commando bm`. With no subcommand, it
lists bookmarks.

```sh
commando bookmark add  [--tags "a,b"] [--note "..."] [--force] <command>
commando bookmark list                     # alias: ls
commando bookmark remove <command>         # alias: rm
```

### `bookmark add`

| Flag | Effect |
|---|---|
| `--tags` | Comma-separated searchable tags. |
| `--note` | Free-form searchable note. |
| `--force` | Save even if the command looks like it contains a secret. |

The command is everything after the flags. A command that matches the
[redaction patterns or built-in secret detectors](../guide/shell-history.md#redaction)
is refused (exit `1`) unless `--force` is given, because bookmarks are stored
in plain text.

### `bookmark list` / `ls`

Prints each bookmark as `command  [tags]  — note`.

### `bookmark remove` / `rm`

Removes the bookmark whose command **exactly** matches the argument.
In the TUI, highlight a bookmark and press ++ctrl+d++ twice (the first press
asks for confirmation) for the same operation.

See [Bookmarks](../guide/bookmarks.md).

---

## `commando version`

Print the version and exit.

```console
$ commando version
commando 0.1.0
```

Also available as `commando --version` and `commando -v`.

---

## `commando help`

Print the usage summary. Also `commando --help` and `commando -h`.
