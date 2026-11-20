---
title: Concepts
---

# Concepts

Commando is built on one small idea, applied consistently. Understanding it
makes every feature feel obvious.

## Entries and providers

Everything in the fuzzy list is an **entry** — a searchable command with an
optional set of variables to fill in. Every source of commands is a
**provider** that emits entries in that same shape:

```text
        ┌─────────────────── providers ───────────────────┐
   bookmarks    cheats    project(make/npm/…)    history
        └────────────────────┬─────────────────────────────┘
                             ▼
                merge + dedup + frecency rank
                             ▼
                       fuzzy TUI
                             ▼
       resolution engine  ◄──►  plain-file state (memory + usage)
        (optional [] · multi · memory · command preview)
                             ▼
                    assembled command
                             ▼
              shell prompt (review)   │   --exec (run now)
```

Because the four sources all produce the same kind of entry, **the hard part —
resolving a template into a command — is written once** and works for all of
them. Adding a capability is a matter of a new provider or a new resolver
feature, not a new special case.

The four shipped sources:

| Source | What it contributes |
|---|---|
| **Cheat sheets** | Hand-authored, parameterized command templates. |
| **Project** | Commands detected from the folder you're in (`make`, `npm`, `cargo`, …). |
| **History** | Commands read live from your shell's history file. |
| **Bookmarks** | Commands you explicitly saved, with tags and notes. |

## Design principles

These are the tie-breakers behind Commando's behavior:

1. **One resolution engine, many sources.** Cheats, project commands, history,
   and bookmarks are all just providers emitting the same entry shape.
2. **The shell owns execution.** Commando assembles a command and hands it to
   your shell's line editor. It does **not** `fork/exec` by default — otherwise
   `cd`, environment variables, and aliases wouldn't stick, and you couldn't
   edit before running.
3. **Everything is a plain file.** No database. Every piece of state is
   human-readable TOML you can `cat`, `grep`, and edit.
4. **Zero-config is useful; config is powerful.** In any folder with no setup,
   Commando already surfaces your project's `make`/`npm`/`gradle` targets and
   frequent commands. Configuration unlocks custom providers and cheat sheets.

## The resolution flow

When you pick an entry with variables, Commando walks its template and, for each
placeholder, decides a value — from a candidate list, a default, your typed
input, or a remembered value. Optional `[...]` segments are dropped wholesale if
you skip their variable. The assembled command is what lands on your prompt.

The engine is deliberately **UI-agnostic**: the same resolution logic could
drive a different front-end in the future without change.

## Where things live

| Path | Holds |
|---|---|
| `~/.config/commando/config.toml` | Key binding, redaction denylist, custom providers |
| `~/.config/commando/cheats/*.toml` | Cheat sheets |
| `~/.config/commando/bookmarks.toml` | Saved commands + tags/notes |
| `~/.local/share/commando/memory.toml` | Remembered variable values (frecency) |
| `~/.local/share/commando/usage.toml` | Per-entry launch counts (ranking) |
| `~/.local/share/commando/data/` | Auxiliary data files cheats can read (`$COMMANDO_DATA_DIR`) |
| your `$HISTFILE` | Shell history — read live, never copied |

See [Files & storage](../reference/files.md) for the full reference.
