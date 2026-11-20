---
title: Cheat packs & repos
---

# Cheat packs & repos

A **cheat pack** is an ordinary [cheat file](cheat-sheets.md): one `.toml`
file of `[[cmd]]` entries about one tool or topic. Commando ships a set of
packs, and you can follow packs that other people maintain by adding their
git repository as a **cheat source**.

## Built-in packs

The packs in the repository's
[`cheats/`](https://github.com/liliumapps/commando/tree/main/cheats) folder
are built into the binary: `git`, `docker`, `kubectl`, `files`, `process`,
`npm`, `ssh`, and `network`. On first run each one is copied into
`~/.config/commando/cheats/`, where it is yours to edit. A pack you delete is
not copied again, and packs added in a later release are offered once on
upgrade.

## Adding a source

A source is a git repository, either all of it or one file in it:

```sh
# Every *.toml file in the repository
commando cheats add https://github.com/acme/team-cheats

# One file, by its path from the repository root
commando cheats add https://github.com/liliumapps/commando cheats/docker.toml
```

- **Whole repository:** every `*.toml` file is loaded, in any folder.
  Folders whose name starts with `.` (such as `.git` and `.github`) are
  skipped.
- **One file:** the path is relative to the repository root, must end in
  `.toml`, and cannot contain `..` or start with `/`.

Repository URLs can be `https://...`, `ssh://...`, or `git@host:path`. Local
folders, `file://`, `http://`, and git transport helpers such as `ext::` are
refused. Private repositories work with your normal git authentication (ssh
keys or a credential helper); never put a password or token in the URL.

Cheats from a source appear in the list next to your own. Each keeps its own
remembered values and ranking, separate from your cheats and from other
sources.

## Reviewing what a source runs

Most of a cheat is inert text: the template is only placed on your prompt,
and you choose whether to run it. Two fields are different. A variable's
`src` runs a shell command to list candidates whenever its form opens, and
`preview` runs one for the highlighted candidate. A cheat file from someone
else can therefore run commands on your machine without you pressing Enter.

So commando asks before it trusts a source:

```console
$ commando cheats add https://github.com/liliumapps/commando cheats/docker.toml
Cloning https://github.com/liliumapps/commando ...

Source https://github.com/liliumapps/commando (cheats/docker.toml), 5 command(s)
commit 3f9c2e1d4b...
These cheats run the following commands on your machine (src fills candidate lists, preview shows the highlighted value):

  Tail container logs
    src      docker ps --format '{{.Names}}'

  Remove docker image
    src      docker images
    preview  docker image inspect {} --format '{{json .Config}}'

Trust these commands? [y/N] y
Added https://github.com/liliumapps/commando#cheats/docker.toml at 3f9c2e1: 5 command(s).
```

- `add` prints the URL, the commit, and every distinct `src` and `preview`
  command, grouped by cheat title. Answering anything but `y` leaves nothing
  behind: no clone and no entry in `sources.toml`.
- `update` asks again when any `src` or `preview` command in the source is
  new or changed, marking those with `*`. Changes to titles, templates, and
  other fields update without asking. Declining keeps the source at the
  commit you reviewed before.
- A source with no `src` or `preview` commands is added without a question.
- `--yes` trusts without asking (the commands are still printed). Without a
  terminal and without `--yes`, nothing is trusted.

## Updating, listing, and removing

```sh
commando cheats update      # every source
commando cheats list
commando cheats remove https://github.com/acme/team-cheats
commando cheats remove https://github.com/liliumapps/commando cheats/docker.toml
```

`update` fetches the newest commit of each repository's default branch. `list`
shows each source's URL, its file (or `all`), the reviewed commit, how many
commands it loads, and when it was last added or updated. `remove` takes the
same URL and file you added; the clone is deleted once no source uses it.

## Overriding a source cheat

To change a cheat from a source, copy it into one of your own files under
`~/.config/commando/cheats/` and keep its [`id`](cheat-sheets.md#stable-ids).
When one of your cheats has the same `id` as a source cheat, yours is used
and the source's is dropped. Your own cheats' identities, and so their
remembered values, never change when you add sources.

## How sources are stored

`~/.config/commando/sources.toml` lists the sources. Commando writes it, and
you can edit it by hand:

```toml
[[source]]
url = "https://github.com/acme/team-cheats"

[[source]]
url     = "https://github.com/liliumapps/commando"
file    = "cheats/docker.toml"
commit  = "3f9c2e1d4b..."               # the reviewed commit
updated = "2026-11-20T10:00:00Z"
```

A source you add by hand has no `commit` yet, so it is not loaded until you
run `commando cheats update`, which clones it and asks you to review it.

Each repository is cloned once, shallowly (`--depth 1`, default branch only),
into `~/.local/share/commando/repos/<name>-<hash>/`, and shared by every
source that uses it.

## The security model

- **Only reviewed code is loaded.** Commando loads a source only when its
  clone is checked out at the `commit` recorded in `sources.toml`. If the
  clone is at another commit (for example, after a manual `git pull`), the
  source is skipped, `commando doctor` and the list's status line report it,
  and `commando cheats update` reviews the difference.
- **Git runs locked down.** Hooks are disabled (`core.hooksPath=/dev/null`),
  only the `https` and `ssh` transports are allowed, submodules are never
  fetched, and symlinks are checked out as plain files.
- **Files stay inside the clone.** A cheat file that is a symlink, or that
  resolves outside the clone, is refused.
- **Problems are contained.** A missing clone, a file that does not parse, or
  an `id` used twice in a source is reported for that source or file only;
  everything else still loads. Run `commando doctor` to see the details.

Reviewing the commands is the protection. Commando cannot tell a harmless
`src` from a harmful one, so read them before you answer `y`, and only add
repositories whose owners you trust to keep them safe.

## Sharing your own pack

Any git repository of cheat files works as a source; there is nothing else to
set up. For a pack in this repository, add a file to
[`cheats/`](https://github.com/liliumapps/commando/tree/main/cheats) and open
a pull request. Its
[README](https://github.com/liliumapps/commando/blob/main/cheats/README.md)
has the guidelines: one tool per file, generic commands, and quick, read-only
`src` and `preview` commands.
