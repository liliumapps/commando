---
title: Files & storage
---

# Files & storage

**Commando has no database.** Every piece of state is a plain, human-readable
file you can `cat`, `grep`, edit, back up, or delete. This page is the complete
map of what lives where.

## The layout

```text
~/.config/commando/                 ← config root  ($XDG_CONFIG_HOME/commando)
├── config.toml                     ← key binding, redaction, custom providers
├── bookmarks.toml                  ← saved commands + tags/notes
├── sources.toml                    ← cheat sources and their reviewed commits
└── cheats/
    ├── git.toml                    ← seeded on first run
    ├── docker.toml                 ← seeded on first run
    ├── kubectl.toml                ← seeded on first run
    ├── files.toml                  ← seeded on first run
    └── user.toml                   ← where `commando add` appends

~/.local/share/commando/            ← data root  ($XDG_DATA_HOME/commando)
├── memory.toml                     ← remembered variable values (frecency)
├── usage.toml                      ← per-entry launch counts (ranking)
├── fills.toml                      ← recent complete form fills per cheat
├── repos/                          ← shallow clones of cheat sources
│   └── <name>-<hash>/
└── data/                           ← auxiliary files for cheat `src` commands
                                       (exposed as $COMMANDO_DATA_DIR)

<user cache dir>/commando/          ← cache root  ($COMMANDO_HOME/cache/commando)
└── <sha256>.json                   ← cached output of `cache`-enabled commands

$HISTFILE (e.g. ~/.zsh_history)     ← shell history — read live, never copied
```

Commando may also create `*.lock` sidecars next to mutable TOML files. They are
empty advisory-lock files used to merge updates safely when several shells run
Commando at once; leave them in place.

## Every file, explained

| File | Read/Written | Contents |
|---|---|---|
| `config.toml` | you write | Key binding, history redaction denylist, custom providers. See [Configuration](configuration.md). |
| `cheats/*.toml` | you + `commando add` | Cheat sheet entries. Every `.toml` here is loaded. See [Cheat sheets](../guide/cheat-sheets.md). |
| `sources.toml` | `commando cheats` + you | Cheat sources: `url`, optional `file`, the reviewed `commit`, and `updated`. See [Cheat packs & repos](../guide/cheat-packs.md#how-sources-are-stored). |
| `repos/<name>-<hash>/` | Commando | One shallow clone per source URL, shared by its sources. Only the reviewed commit is loaded. |
| `bookmarks.toml` | Commando + you | Saved commands with tags and notes. See [Bookmarks](../guide/bookmarks.md). |
| `memory.toml` | Commando + you | Remembered variable values, keyed by `(command, variable, scope)`. A cheat's `id`, when set, identifies the command. See [Argument memory](../guide/argument-memory.md). |
| `usage.toml` | Commando | Per-entry launch counts and last-used times, used to rank the list by frecency. |
| `fills.toml` | Commando + you | Recent complete sets of values per cheat, offered in the form's Recent block. See [Recent fills](../guide/argument-memory.md#recent-fills). |
| cache `*.json` | Commando | Output of `src` and provider `list` commands that set `cache`. Disposable. See [The cache directory](#the-cache-directory). |
| `data/*` | you | Arbitrary candidate-list files your cheats read via `$COMMANDO_DATA_DIR`. |
| `$HISTFILE` | Commando reads | Your shell's own history file. Commando reads it live and never modifies or copies it. |

## Why plain files?

- **Inspectable.** `cat ~/.local/share/commando/memory.toml` and see exactly what
  Commando remembers — no opaque binary blob.
- **Editable.** Fix a typo in a remembered value, prune a bookmark, or hand-write
  a cheat, all with your normal editor.
- **Portable.** Commit your cheats to a dotfiles repo, sync them across machines,
  or share them with your team.
- **Debuggable.** When something looks wrong, the state is right there in text.

Writes are assembled in a temporary file, synced, and atomically renamed.
Concurrent processes lock and merge the latest file before writing. Remembered
values are capped at 25 per command-variable-scope bucket, and usage ranking is
capped at the 5,000 most recent entries, so these files remain bounded.

If `memory.toml`, `usage.toml`, `fills.toml`, or `bookmarks.toml` exists but cannot be
parsed (for example after a bad hand edit), Commando moves it aside to
`<name>.corrupt-<unix time>`, such as `memory.toml.corrupt-1751500000`, and
starts with that file empty instead of failing. `commando doctor` warns about
any moved-aside files. Fix the old file and move it back, or delete it.

## The cache directory

A variable's `src` or a custom provider's `list` command that sets `cache`
stores its output here, so the result is reused across launches until the
duration expires. The directory is `commando/` under the platform's user cache
directory:

| Platform | Location |
|---|---|
| macOS | `~/Library/Caches/commando` |
| Linux | `$XDG_CACHE_HOME/commando`, or `~/.cache/commando` |
| Windows | `%LocalAppData%\commando` |

With `COMMANDO_HOME` set, it is `$COMMANDO_HOME/cache/commando`. Each record is
a JSON file named by a hash of the command and directory, readable only by you
(command output can be sensitive). Expired or unreadable records are treated as
misses. Deleting the directory is always safe.

## The data directory (`COMMANDO_DATA_DIR`)

`~/.local/share/commando/data/` is a scratch space for **candidate-list data
files** your cheats read. When Commando runs a variable's `src` or `preview`
command, it sets the `COMMANDO_DATA_DIR` environment variable to this path:

```toml
[[cmd.var]]
name = "host"
src  = 'cat "$COMMANDO_DATA_DIR/hosts.txt"'
```

Drop a `hosts.txt` in `data/` (one host per line) and use it as `ssh <host>`, and the variable's candidates come from
it. Great for lists that are expensive to compute or come from an external
source.

## XDG overrides

Both roots honor the XDG environment variables:

| Root | Env var | Default |
|---|---|---|
| Config | `XDG_CONFIG_HOME` | `~/.config` |
| Data | `XDG_DATA_HOME` | `~/.local/share` |

Set these to relocate Commando's files — useful for testing or for keeping
everything under a single dotfiles tree.

## Resetting

| To reset… | Do this |
|---|---|
| Remembered values | `commando state clear memory` |
| Ranking / usage | `commando state clear usage` |
| Both state files | `commando state clear all` |
| Bookmarks | Delete `bookmarks.toml` or edit it |
| A cheat | Edit or delete the relevant `cheats/*.toml` |
| A cheat source | `commando cheats remove <git-url> [file]` |
| Cached command output | Delete the [cache directory](#the-cache-directory) |
| Everything | Remove `~/.config/commando`, `~/.local/share/commando`, and the cache directory |

Commando recreates whatever it needs on the next run (including re-seeding the
starter cheats).
