---
title: FAQ
---

# FAQ

## Does Commando run my commands for me?

**No, not by default.** It assembles the command, places it on your shell prompt,
and lets you review, edit, and press ++enter++ yourself — so `cd`, environment
variables, and aliases keep working. Use `commando search --exec` if you *do*
want it to run the command directly.

## Which shells are supported?

The interactive **summon widget** supports Zsh, Bash, and Fish. The CLI itself
works from any shell — it just prints the assembled command to stdout.

## Is there a database?

No. Every piece of state is a plain TOML file you can read and edit, and shell
history is read live and never copied. See [Files & storage](reference/files.md).

## Does Commando slow down my shell?

No. It installs **no** `preexec`/`precmd` capture hook — only a summon widget
bound to a key. History is read on demand when you open the launcher.

## How is the list ordered? / How does memory rank values?

The list uses **frecency**: how often you run an entry, weighted by how
recently. It orders the list before you type and breaks ties among close
matches after. Remembered values are shown most-recently-used first, and
frecency decides which 25 values per `(command, variable, scope)` are kept. See
[Argument memory](guide/argument-memory.md#frecency-ranking).

## How does the search box match my query?

Each whitespace-separated term is matched **independently** against an entry's
title, description, command text, and tags, and every term must match
*somewhere* — so `deploy cluster` finds a command tagged `deploy` whose
description contains `cluster`, in any word order. Exact and prefix matches rank
above word-prefix, contains, and subsequence ("fuzzy") matches. Matches in the
title are weighted above tags, command text, and description. To restrict the
list to one source, lead the query with a scope prefix (`:c`, `:p`, `:h`, `:b`)
or press ++tab++ to cycle; see
[Scoping to a source](guide/tui.md#scoping-to-a-source).

## How do I keep secrets out of Commando?

Two independent controls:
[`remember = "none"`](guide/argument-memory.md#secrets-always-use-remember-none)
on a variable keeps its *values* out of `memory.toml`, and the
[redaction denylist](guide/shell-history.md#redaction) hides secret-looking
*commands* from history. The denylist and built-in secret detectors also keep
secret-looking values out of memory and refuse secret-looking bookmarks
(`commando bookmark add --force` overrides).

## What's the difference between history and bookmarks?

History is **everything** you've run, read live and shown newest-first (capped
until you search). Bookmarks are the **handful** you explicitly saved, with
searchable tags and notes, sorted to the top.

## Can I share my cheat sheets?

Yes — they're plain TOML files in `~/.config/commando/cheats/`. Commit them to a
dotfiles repo, sync across machines, or share with your team. Every `.toml` in
that directory is loaded.

## How do I change the summon key?

Set [`key_binding`](reference/configuration.md#key_binding) in `config.toml`,
then reload `commando init zsh`, `bash`, or `fish` integration.

## Where does `commando add` write?

It appends the new cheat entry to `~/.config/commando/cheats/user.toml` after
validating the generated TOML.

## A command has an optional flag — how do I avoid a dangling flag?

Put the flag *inside* the optional bracket with its variable: `[-n <count>]`. If
you skip `count`, the whole `-n <count>` fragment disappears. See
[Template syntax](guide/templates.md#optional-segments).

## Something looks wrong — how do I reset it?

Run `commando state clear memory` to forget parameter values,
`commando state clear usage` to reset ranking, or `commando state clear all`
for both. See the [reset table](reference/files.md#resetting) for file-level
recovery.

If a state or bookmarks file was corrupt, Commando moves it aside to
`<name>.corrupt-<unix time>` and starts with it empty; `commando doctor`
reports the moved file. See [Files & storage](reference/files.md#why-plain-files).
