---
title: Cheat sheets
---

# Cheat sheets

Cheat sheets are the heart of Commando: parameterized, reusable command
templates you author once and fill in interactively forever after.

## The format

Cheat files are **TOML**, stored in `~/.config/commando/cheats/*.toml`. Each
file holds one or more `[[cmd]]` entries. Here's a complete example:

```toml
[[cmd]]
title = "Search git log"                           # (1)!
desc = "Search git log by author for a file."      # (2)!
tmpl = "git log [--author=<author>] [-n <count>] -- <file>"  # (3)!
tags = ["git", "log"]                              # (4)!

  [[cmd.var]]                                       # (5)!
  name = "author"
  src  = "git log --all --format='%an' | sort -u"

  [[cmd.var]]
  name    = "count"
  default = "20"

  [[cmd.var]]
  name     = "file"
  multi    = true
  each     = '"{}"'
  remember = "project"
```

1. **`title`** — the short name shown in the result list and searched.
2. **`desc`** — optional detail shown for the selected command and searched.
   If omitted, Commando uses the title as the selected detail too. Older
   `desc`-only cheats still load; their `desc` is treated as the title.
3. **`tmpl`** — the command template. See [Template syntax](templates.md) for
   `<var>` and `[...]`.
4. **`tags`** — extra searchable keywords (also fuzzy-matched).
5. **`[[cmd.var]]`** — one block per variable, declaring how to resolve it. See
   [Variables](variables.md).

## Entry fields

| Field | Required | Meaning |
|---|---|---|
| `id` | | Optional stable identity. See [Stable ids](#stable-ids). |
| `title` | ✅ | Short searchable name shown in the list. |
| `desc` | | Optional searchable detail shown for the selected command. |
| `tmpl` | ✅ | The command template with `<var>` placeholders and `[...]` optional segments. |
| `tags` | | List of extra searchable keywords. |
| `use` | | Names of [shared variable groups](variables.md#shared-variable-groups) to pull in. |
| `[[cmd.var]]` | | Zero or more variable specs. See [Variables](variables.md). |

## Stable ids

Commando keys a cheat's remembered values and usage count by its identity. By
default that identity is derived from the title and template, so retitling a
cheat or editing its template starts it with fresh memory and ranking. Give the
cheat an `id` to keep them across edits:

```toml
[[cmd]]
id    = "k8s/restart-deploy"
title = "Restart deployment"
tmpl  = "kubectl rollout restart deploy/<name> [-n <ns>]"
```

An `id` may contain letters, digits, `.`, `_`, `/`, and `-`, must start with a
letter or digit, and is at most 128 characters. It must be unique across all
cheat files; a duplicate is reported as a load error. Adding an `id` to an
existing cheat changes its identity once, so its earlier memory is not carried
over.

## Where cheat files live

```text
~/.config/commando/cheats/
├── git.toml         ← seeded on first run (also docker, kubectl, files,
│                      process, npm, ssh, network)
└── user.toml        ← where `commando add` appends
```

Every `.toml` file in this directory is loaded. Group commands however you
like — by tool, by project, by team convention. On first run, Commando seeds a
starter set (git, docker, kubectl, files, process, npm, ssh, network) so the
list isn't empty. To share cheats or pull in someone else's, add a git repo or
a single file from one with `commando cheats add`; see
[Cheat packs & repos](cheat-packs.md).

!!! tip "Data files for candidate lists"

    A variable's `src` command runs with `COMMANDO_DATA_DIR` set to
    `~/.local/share/commando/data/`. Drop a plain text file there and read it:

    ```toml
    [[cmd.var]]
    name = "host"
    src  = 'cat "$COMMANDO_DATA_DIR/hosts.txt"'
    ```

## Authoring interactively with `commando add`

Rather than hand-writing TOML, let Commando do it:

```console
$ commando add
Title: Restart deployment
Description (optional): Restart a Kubernetes deployment.
Command template (use <var> and [optional <var>]): kubectl rollout restart deploy/<name> [-n <ns>]
Tags (comma-separated, optional): k8s, deploy

Variable <name>:
  Candidate command (src, optional): kubectl get deploy -o name
  Default value (optional):
  Allow multiple selections? [y/N]: n
  Remember scope [none/global/project/dir] (default none): project

Variable <ns> (optional):
  Candidate command (src, optional): kubectl get ns -o name
  Default value (optional):
  Allow multiple selections? [y/N]: n
  Remember scope [none/global/project/dir] (default none): global

Added to ~/.config/commando/cheats/user.toml
```

`commando add`:

- Parses your template and **discovers each variable**, including whether it's
  optional (inside `[...]`).
- Prompts for each variable's options.
- Validates the generated TOML before writing.
- Appends the entry to `cheats/user.toml`.

## Editing by hand

Cheat files are plain TOML — open them in any editor. Commando validates every
cheat file on load; a malformed file surfaces an error rather than silently
disappearing. Use the [examples in the repo](https://github.com/liliumapps/commando/tree/main/examples)
as templates:

- `examples/git.toml` — optional segments, multi-select, candidate `src`,
  memory.
- `examples/shared-and-crossvar.toml` — shared variable groups, candidate
  post-processing, previews, and cross-variable lookups.

## Next

<div class="grid cards" markdown>

-   :material-code-brackets: __[Template syntax](templates.md)__

    `<var>`, optional `[...]` segments, and escaping.

-   :material-tune-variant: __[Variables](variables.md)__

    Every field on a `[[cmd.var]]` block.

</div>
