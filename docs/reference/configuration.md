---
title: Configuration
---

# Configuration

Configuration is **optional** — Commando works with zero setup. When you want to
customize it, create `~/.config/commando/config.toml`. Every field is optional;
omitted fields keep their defaults.

Configuration is decoded strictly. A misspelled or unknown field is an error
instead of being ignored; `commando doctor` reports the exact key.

!!! info "Config location"

    Commando follows the XDG spec. The config file is at
    `$XDG_CONFIG_HOME/commando/config.toml`, which is
    `~/.config/commando/config.toml` unless you've set `XDG_CONFIG_HOME`.

## The complete example

```toml
# ~/.config/commando/config.toml

# Portable control-key sequence bound by `commando init`.
key_binding = "^G"

# Commands matching any of these patterns are hidden from history, and
# matching values and bookmarks are not stored. Case-insensitive, matched
# against the whole line and each word. No-wildcard patterns are substring
# matches; `*` matches any run of characters, including `/`.
# Built-in secret detectors apply in addition to this list.
history_ignore = [
  "*password*",
  "*passwd*",
  "*secret*",
  "*token*",
  "*apikey*",
  "*api_key*",
  "export *KEY*",
  "*credential*",
  "*authorization:*",
  "*bearer *",
  "aws_*=*",
  "*_token=*",
  "*_secret=*",
  "*_pat=*",
]

# Shell-history tuning.
[history]
# How many history commands appear when you are NOT searching.
# A query shows all matches in the loaded window. Negative = unlimited.
list_limit = 10
# How many unique commands to retain from the newest history.
max_load = 5000
# Read only this many bytes from the newest end of the history file.
max_bytes = 8388608

# User-defined project providers (repeatable).
[[provider]]
name     = "task"
detect   = "Taskfile.yml"
list     = "task --list-all --silent | awk '{print $2}'"
template = "task {}"
cache    = "5m"
```

## Top-level fields

| Field | Type | Default | Meaning |
|---|---|---|---|
| `key_binding` | string | `"^G"` | Portable control-key sequence for the summon widget. Read by `commando init`. |
| `history_ignore` | string list | see below | Patterns for commands to hide from history and values to keep out of memory and bookmarks. |

### `key_binding`

The key sequence bound to the summon widget. Use `^X` control-key notation, for
example `^G` (Ctrl-G) or `^F` (Ctrl-F). Commando translates it to Zsh, Bash, or
Fish syntax. After changing it, reload your shell integration.

### History redaction

`history_ignore` hides secret-looking commands from the history list (defaults
shown in the example above). The same patterns keep secret-looking values out
of argument memory and bookmarks. Matching rules:

- **Case-insensitive**, against the whole command and each whitespace-separated
  word.
- A pattern with **no** wildcard is a **substring** match.
- `*` matches any run of characters, **including `/`**; `?` matches one
  character; `[ ]` is a character class (`[!...]` negates).

Built-in detectors for secret-shaped values (cloud access keys, GitHub, Slack,
and OpenAI-style tokens, JWTs, URLs with `user:password@`, `mysql -p<password>`,
`Authorization`/`Bearer` headers) always apply in addition to the list.

!!! warning "Setting your own list replaces the defaults"

    Providing `history_ignore` overrides the built-in pattern list rather than
    adding to it. Include the defaults you still want, plus your own patterns.
    The built-in detectors still apply.

See [Shell history → Redaction](../guide/shell-history.md#redaction).

## `[history]` table

| Field | Type | Default | Meaning |
|---|---|---|---|
| `list_limit` | int | `10` | Max history commands shown when **not** searching. A query shows all matches in the loaded window. Negative = unlimited. |
| `max_load` | int | `5000` | Max unique commands loaded from the newest history. `0` uses the default; negative = unlimited. |
| `max_bytes` | int | `8388608` | Max bytes read from the newest end of the history file (8 MiB). `0` uses the default; negative reads the whole file. |

## `[[provider]]` blocks

Each `[[provider]]` block defines a custom project provider, detected by a marker
file found while walking up from the current directory.

| Field | Type | Meaning |
|---|---|---|
| `name` | string | Display label for the provider's entries. |
| `detect` | string | Relative marker path that must exist beneath the project walk. Absolute paths and `..` escapes are rejected. |
| `list` | string | Shell command producing candidate items, one per line. |
| `template` | string | Command template containing `{}`; each placeholder is replaced by one shell-quoted item. |
| `cache` | string | Optional non-negative cache duration (e.g. `5m`, `1h`) for slow `list` commands. Cached output persists across launches in the [cache directory](files.md#the-cache-directory). |

Provider names must be unique, case-insensitively. A lister line is treated as
one argument rather than executable shell syntax, so spaces and metacharacters
are quoted automatically. You can define as many `[[provider]]` blocks as you
like. See
[Project commands → Defining your own provider](../guide/project-commands.md#defining-your-own-provider).

## Environment variables

| Variable | Effect |
|---|---|
| `XDG_CONFIG_HOME` | Overrides the config root (default `~/.config`). |
| `XDG_DATA_HOME` | Overrides the data root (default `~/.local/share`). |
| `COMMANDO_HOME` | Relocates everything: config and data under `$COMMANDO_HOME/commando`, the cache under `$COMMANDO_HOME/cache/commando`. |
| `HISTFILE` | The history file Commando reads first (before the fallbacks). |
| `COMMANDO_DATA_DIR` | **Set by Commando** when running a cheat's `src`/`preview`; points at `~/.local/share/commando/data/`. Read it in your cheat commands. |
