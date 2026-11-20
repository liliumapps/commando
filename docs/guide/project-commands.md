---
title: Project commands
---

# Project commands

Commando inspects the folder you're in and surfaces the commands that folder
already knows how to run — no configuration required. Open the launcher inside a
Node project and you'll see its `npm` scripts; inside a Rust crate, its `cargo`
commands.

![Inside a repo, commando surfaces make targets and other project commands with no configuration](../assets/gifs/project.gif)

<p class="gif-caption">A repo with a <code>Makefile</code> and <code>package.json</code>: searching “make build” surfaces <code>make build</code> — detected automatically, zero config.</p>

## How detection works

When you summon Commando, it walks **up** from your current directory looking for
marker files. Each marker it finds contributes a set of entries. The walk stops
at the git root, so it doesn't climb all the way to your home directory. When a
tool has several marker names (`GNUmakefile`, `makefile`, `Makefile`), only the
nearest one is used.

A built-in command found in a parent directory carries a path flag, so it runs
correctly from where you are without a `cd`:

| Tool | Command from a subdirectory |
|---|---|
| make | `make -C .. build` |
| npm / pnpm | `npm --prefix .. run test`, `pnpm --dir .. run test` |
| yarn / bun | `yarn --cwd .. run test`, `bun --cwd .. run test` |
| cargo | `cargo build --manifest-path ../Cargo.toml` |
| gradle | `../gradlew -p .. build` (or `gradle -p .. build`) |
| just | `just --justfile ../justfile test` |
| docker compose | `docker compose -f ../compose.yaml up -d` |

The path is relative, or absolute when that is shorter. User-defined providers
run their `list` command in the marker's directory, but their `template` is
used as written.

## Built-in detectors

| Marker file | Provider | Contributes |
|---|---|---|
| `GNUmakefile` / `makefile` / `Makefile` | make | One entry per target (`make <target>`) |
| `package.json` | npm / yarn / pnpm / bun | One entry per `scripts` key |
| `Cargo.toml` | cargo | Common subcommands: `run`, `test`, `build`, `bench`, … |
| `build.gradle` / `build.gradle.kts` | gradle | Common tasks (uses the nearest `gradlew` up to the git root when present) |
| `justfile` / `Justfile` / `.justfile` | just | One entry per recipe |
| `compose.yaml` / `compose.yml` / `docker-compose.yaml` / `docker-compose.yml` | compose | `up -d`, `down`, plus per-service `logs -f`/`restart` |
| `.git` | git | Contextual actions: `status`, `pull --rebase`, `push`, `fetch --all --prune` |

These entries flow into the same fuzzy list as your cheats, history, and
bookmarks — search `build` and you'll see `make build`, `npm run build`, and
`cargo build` side by side, ranked by how often you run each.

!!! note "Node package manager"

    For `package.json` scripts, Commando picks the runner from the package's
    `packageManager` field, then from an ancestor `package.json`'s
    `packageManager` or a lockfile (`pnpm-lock.yaml`, `yarn.lock`, `bun.lock`,
    `bun.lockb`, `package-lock.json`, `npm-shrinkwrap.json`), searching from the
    package directory up to the git root. A workspace package in a monorepo
    therefore uses the root's package manager. The default is npm.

## Defining your own provider

Any tool with a marker file and a listing command can become a provider. Add a
`[[provider]]` block to your [config file](../reference/configuration.md):

```toml
# ~/.config/commando/config.toml

[[provider]]
name     = "task"                                   # (1)!
detect   = "Taskfile.yml"                           # (2)!
list     = "task --list-all --silent | awk '{print $2}'"  # (3)!
template = "task {}"                                # (4)!
cache    = "5m"                                     # (5)!
```

1. **`name`** — label shown for these entries.
2. **`detect`** — marker filename that must exist (found by walking up from cwd).
3. **`list`** — shell command producing candidate items, one per line.
4. **`template`** — `{}` is replaced by each item as one safely quoted shell
   argument.
5. **`cache`** — optional non-negative duration to cache the `list` output, for
   slow listers. The cache persists across launches.

### Provider fields

| Field | Meaning |
|---|---|
| `name` | Display label for the provider's entries. |
| `detect` | Relative marker path that triggers the provider when found. |
| `list` | Shell command whose stdout lines each become a candidate item. |
| `template` | Command template containing `{}`; each occurrence receives the quoted item. |
| `cache` | Optional cache duration (e.g. `5m`, `1h`) for slow `list` commands. |

Each output line is data, not a shell fragment. A line such as `release prod`
becomes `task 'release prod'`, and shell metacharacters remain inside that
argument. Keep flags and shell syntax in the authored `template`, not in lister
output.

### Example — re-declaring the built-ins

The built-in `make` and `npm` providers are equivalent to:

```toml
[[provider]]
name     = "make"
detect   = "Makefile"
list     = "grep -E '^[a-zA-Z0-9_-]+:' Makefile | cut -d: -f1"
template = "make {}"

[[provider]]
name     = "npm"
detect   = "package.json"
list     = "jq -r '.scripts | keys[]' package.json"
template = "npm run {}"
```

!!! tip "Cache slow listers"

    Tools like Gradle can take a second or two to enumerate tasks. Add
    `cache = "5m"` so Commando reuses the last listing instead of re-running the
    command every time you open the launcher. The listing is stored in the
    [cache directory](../reference/files.md#the-cache-directory), keyed by the
    `list` command and the marker's directory, so it survives across launches
    until it expires.

## Next

<div class="grid cards" markdown>

-   :material-history: __[Shell history](shell-history.md)__

    The other zero-config source — your own past commands.

-   :material-cog: __[Configuration](../reference/configuration.md)__

    The complete `config.toml` reference.

</div>
