# commando

A context-aware command launcher for the terminal. `commando` unifies four
sources of commands behind a single fuzzy TUI, then places the assembled command
on your shell prompt for review before you run it.

**Website and docs: [commando.liliumapps.com](https://commando.liliumapps.com/)** · Made by
[Lilium Apps](https://liliumapps.com)

![Commando: search across sources, fill in an optional and multi-select command, and it lands on your prompt](docs/assets/gifs/tour.gif)

- **Cheat sheets** — parameterized, reusable command templates.
- **Project-detected commands** — `make` targets, `npm`/`pnpm`/`yarn` scripts,
  `cargo`, Gradle tasks, `just` recipes, docker-compose services, and git
  actions, discovered from whatever folder you're in.
- **Bookmarks** — commands you explicitly saved, with searchable tags and notes.
- **Shell history** — read live, searchable, and offered back to you.

On top of that it adds the things existing tools don't combine:

- **Argument memory** with **frecency** ranking — it remembers what you last
  entered per variable and floats hot values to the top.
- **First-class optional and multi-select parameters** — no dangling flags, no
  awkward escaping.
- **A live command preview** that assembles as you fill in variables.

See [PROPOSAL.md](./PROPOSAL.md) for the full design rationale.

## Documentation

The full documentation lives at **[commando.liliumapps.com](https://commando.liliumapps.com/)**:

- [Installation](https://commando.liliumapps.com/getting-started/installation/) and
  [quickstart](https://commando.liliumapps.com/getting-started/quickstart/)
- [Cheat sheets](https://commando.liliumapps.com/guide/cheat-sheets/),
  [templates](https://commando.liliumapps.com/guide/templates/), and
  [variables](https://commando.liliumapps.com/guide/variables/)
- [Project commands](https://commando.liliumapps.com/guide/project-commands/),
  [shell history](https://commando.liliumapps.com/guide/shell-history/), and
  [bookmarks](https://commando.liliumapps.com/guide/bookmarks/)
- [Argument memory](https://commando.liliumapps.com/guide/argument-memory/) and the
  [TUI and keys](https://commando.liliumapps.com/guide/tui/)
- Reference: [CLI](https://commando.liliumapps.com/reference/cli/),
  [configuration](https://commando.liliumapps.com/reference/configuration/), and
  [files](https://commando.liliumapps.com/reference/files/)
- [FAQ](https://commando.liliumapps.com/faq/)

## Install

With Homebrew on macOS:

```sh
brew install --cask liliumapps/tap/commando
```

Or use the release installer on macOS, Linux, or Windows from Git Bash:

```sh
curl -fsSL https://raw.githubusercontent.com/liliumapps/commando/main/install.sh | sh
```

The installer verifies the release checksum and writes to `~/.local/bin` by
default. To build from source instead, install Go 1.25+ and run:

```sh
make install
```

Then add the integration for your shell:

```sh
eval "$(commando init zsh)"        # ~/.zshrc
eval "$(commando init bash)"       # ~/.bashrc
commando init fish | source        # ~/.config/fish/config.fish
```

Reload your shell. On first run, `commando` seeds a set of sample cheat sheets
into `~/.config/commando/cheats/`.

## Usage

Press **Ctrl-G** (the default binding) at any prompt to summon the launcher, or
run `commando` directly.

1. **Search** — type to fuzzy-filter across cheats, project commands, bookmarks,
   and history. Each whitespace-separated term is matched independently against
   an entry's title, description, command, and tags, so `deploy cluster` finds a
   command tagged `deploy` whose description mentions `cluster`. Narrow to one
   source with a `:` prefix (`:c` cheats, `:p` project commands, `:h` history,
   `:b` bookmarks) or press `Tab` to cycle scopes. Title and prefix matches rank
   first, with frecency (how often/recently you run a command) breaking close
   matches.
2. **Fill in** — for a parameterized command, a form walks each variable:
   - type a value, or pick from candidates with the arrow keys
   - optional fields can be skipped (`Ctrl-S`) — the whole `[...]` fragment
     disappears from the command
   - multi-select fields let you choose several values (`Space` toggles)
   - a command preview updates as the values are filled
3. **Run** — the finished command is placed on your prompt. Review, edit if you
   like, and press Enter.

### Commands

```
commando                    Open the interactive search TUI
commando search --print     Open the TUI; write the chosen command to stdout
commando search --exec      Open the TUI; run the chosen command directly
commando init <shell>       Print integration for zsh, bash, or fish
commando add                Interactively author a new cheat entry
commando promote [flags] <template>  Promote a command into a managed cheat
commando history [query]    Search your shell history (read live from the file)
commando doctor             Diagnose setup and provider issues
commando state clear memory|usage|all  Reset remembered state
commando bookmark add <cmd> Save a bookmark (--tags "...", --note "...", --force)
commando bookmark list      List bookmarks
commando bookmark remove <cmd>  Remove a bookmark
commando version            Print the version
```

In the TUI, press **Ctrl-B** on any entry to bookmark it. Literal commands are
saved immediately; parameterized cheats open their form and save the fully
assembled command. A command that looks like it contains a secret is refused,
since bookmarks are stored in plain text. Highlight a bookmark and press
**Ctrl-D** twice to delete it (the first press asks for confirmation).

### Bookmarks

A bookmark is a saved command with optional tags and a note — all
fuzzy-searchable, so you can find a hard-to-remember command by describing it:

```sh
commando bookmark add --tags "k8s deploy" --note "prod rollout" \
  "kubectl rollout restart deployment/web -n prod"
```

Bookmarks are stored in `~/.config/commando/bookmarks.toml` (plain, editable)
and sort to the top of the list. `bookmark add` refuses a command that looks
like it contains a secret (see [Configuration](#configuration)); pass `--force`
to save it anyway.

## Cheat sheets

Cheat files are TOML, stored in `~/.config/commando/cheats/*.toml`. Example:

```toml
[[cmd]]
id    = "git/log-author"   # optional stable id: memory survives edits to title/tmpl
title = "Search git log"
desc = "Search git log by author for a file."
tmpl = "git log [--author=<author>] [-n <count>] -- <file>"
tags = ["git", "log"]

  [[cmd.var]]
  name = "author"
  src  = "git log --all --format='%an' | sort -u"   # candidate values

  [[cmd.var]]
  name    = "count"
  default = "20"

  [[cmd.var]]
  name     = "file"
  multi    = true          # allow selecting several
  each     = '"{}"'        # wrap each value: "a.go" "b.go"
  remember = "project"     # scope memory per git project (default is global)
```

The optional `id` gives a cheat a stable identity. Remembered values and usage
counts are keyed by it, so you can retitle the cheat or edit its template
without losing them. It may contain letters, digits, `.`, `_`, `/`, and `-`
(starting with a letter or digit), is at most 128 characters, and must be
unique across all cheat files. Without an `id`, the identity comes from the
title and template, so editing either starts the cheat with fresh memory (as
does adding an `id` to an existing cheat).

### Template syntax

| Token | Meaning |
|---|---|
| `<var>` | A required placeholder, prompted in the form. |
| `[ ... <var> ... ]` | An **optional segment** — if you skip the variable, the entire bracketed fragment is removed. |
| `\<` `\>` `\[` `\]` `\\` | Escaped literals. |

Shell syntax is kept verbatim. `<` starts a placeholder only when a name
character (letter, digit, `_`, `-`, `.`) directly follows it, so `sort < f`,
`<<EOF`, `<<<word`, `<(cmd)`, and `2<&1` are literal. `[...]` is an optional
segment only when it contains a variable, so `grep '[0-9]+'`, an unclosed `[`,
and `[[ ... ]]` (always literal) stay as written. A backslash escapes only
`< > [ ] \`; any other backslash (`\t`, `\*.go`, a line continuation) is kept.
Whitespace in the template, including newlines and spaces inside quotes, is
preserved; only the gap a skipped segment leaves behind is collapsed. A
malformed placeholder such as `<bad name>` or an unterminated `<file` is an
error, so typos are reported rather than emitted.

### Variable options

| Field | Meaning |
|---|---|
| `src` | Shell command whose stdout lines become candidate values. |
| `values` | Inline candidate list (e.g. `["dev","staging","prod"]`) — no shell needed. Offered before `src` values; extraction fields apply to these too. |
| `default` | Pre-filled initial value. |
| `multi` | Allow selecting multiple values. |
| `sep` | Separator joining multiple values (default: space). |
| `each` | Per-item format, `{}` = the value (e.g. `'"{}"'`). |
| `remember` | Memory scope: unset (default) = `global`; or `project`, `dir`, or `none` to opt out. See [Remembering values](#remembering-values). |
| `header_lines` | Drop this many leading lines from `src` output (e.g. a header row). Not applied to `values`. |
| `delimiter` | Regex used to split each line into columns for `columns` (default: whitespace runs). |
| `columns` | Names for the columns of each line after splitting on `delimiter`, so `label`/`value` can reference them as `<name>`. Mutually exclusive with `match`. |
| `match` | Regex applied to each line; its capture groups (named `(?P<x>…)` or positional) feed `label`/`value`. A line that doesn't match is dropped. Mutually exclusive with `columns`. |
| `label` | Template for the **displayed & searched** text (e.g. `"<comm> (<pid>)"`). Defaults to the raw line. See [Display vs. inserted value](#display-vs-inserted-value). |
| `value` | Template for the **inserted & remembered** value (e.g. `"<pid>"`). Defaults to the raw line. |
| `strict` | Restrict the field to its candidate *values*; reject free-typed values. |
| `pattern` | Regex the resolved value must match in full (e.g. `'^[0-9]+$'`); a non-matching value is rejected. Validates typed *or* selected values. |
| `preview` | Command run for the highlighted candidate (`{}` = the candidate's value); output shown in a preview pane. |
| `cache` | Memoize a slow `src` command's output for this Go duration (e.g. `"5m"`) instead of re-running it on every re-fetch. The output is kept in the [disk cache](#how-it-works), so it is reused across launches until it expires. |

Inside `label`, `value`, `match`, and `columns`, a placeholder is a **column
reference**, not another variable: `<0>` is the whole line/match, `<1>`, `<2>`, …
are columns/groups by position, and a name from `columns` or a `(?P<name>…)`
group in `match` is addressable as `<name>`. (This is distinct from a `<var>` in
a `src` command, which references *another variable* — see [Shared variable
groups](#shared-variable-groups).)

### Display vs. inserted value

A candidate has two projections: what you **see and fuzzy-search** (its label)
and what actually gets **inserted into the command** (its value). By default
they're identical — the whole `src` line. `label` and `value` let them differ,
which solves the classic "the list is readable but I only want one field out of
it" problem.

The motivating example — kill a process you find *by name*, but insert only its
PID:

```toml
[[cmd]]
title = "Kill process"
desc = "Pick a process by name and send SIGKILL to its PID."
tmpl = "kill -9 <pid>"

  [[cmd.var]]
  name         = "pid"
  src          = "ps -eo pid,comm"
  header_lines = 1                              # drop the "PID COMMAND" header
  match        = '^\s*(?P<pid>\d+)\s+(?P<comm>.+)$'
  label        = "<comm>  (<pid>)"              # shown & searched: "chrome  (1234)"
  value        = "<pid>"                        # inserted: "1234"
  pattern      = '^[0-9]+$'                      # value must be all digits
  remember     = "none"                          # PIDs are ephemeral
```

You fuzzy-search the list by process name, and commando places `kill -9 1234` on
your prompt. The same pattern with `columns` (for cleanly-aligned output) instead
of `match`:

```toml
  [[cmd.var]]
  name    = "pid"
  src     = "ps -eo pid,comm"
  header_lines = 1
  columns = ["pid", "comm"]
  label   = "<comm>  (<pid>)"
  value   = "<pid>"
```

**A note on memory:** what's remembered is the *value* (e.g. `1234`), not the
label — a recalled value with no matching line in the current list shows as its
bare value. For ephemeral data (PIDs, container ids) set `remember = "none"`; for
stable data (branches, accounts) the value is already readable, so recall reads
naturally.

### Remembering values

commando remembers the values you enter for a variable and offers them the next
time as ranked candidates (most-recently-used first) above the input. **Memory
is on by default** — you don't need to configure anything.

The `remember` field controls the *scope* of that memory:

| `remember` | Behavior |
|---|---|
| *(unset)* | **Default.** Remember globally — the value is offered everywhere. |
| `global` | Same as unset: remember across every directory. |
| `project` | Remember per git project (keyed by the repository root). |
| `dir` | Remember per working directory. |
| `none` | **Opt out** — never store values for this variable (use for secrets). |

How remembered values behave in the form:

- They appear as candidates in the list above the input, ordered most-recent
  first — never typed into the input automatically.
- For a **list field** (one with a `src` command) pressing Enter on an empty
  input picks the top candidate. For a **plain field** (no `src`) Enter never
  auto-fills a remembered value; select one deliberately with ↑ then Enter.
- An author-defined `default` still pre-fills the input regardless of memory.

Values are stored in `~/.local/share/commando/memory.toml` (plain, editable).
Set `remember = "none"` on any variable holding sensitive input so its values
are never written there. Values that look like secrets (by themselves, or as
`<var>=<value>` against the redaction patterns) are never remembered either.

### Shared variable groups

Define a variable group once and reuse it across commands with `use`, instead of
repeating specs. A command-local `[[cmd.var]]` overrides a shared var of the
same name:

```toml
[[vars.aws]]
name = "profile"
src  = "aws configure list-profiles"

[[vars.aws]]
name    = "region"
values  = ["us-east-1", "us-west-2", "eu-west-1"]
default = "us-east-1"

[[cmd]]
title = "List S3 buckets"
desc = "List S3 buckets with shared profile and region variables."
tmpl = "aws --profile <profile> --region <region> s3 ls"
use  = ["aws"]        # pulls in profile + region
```

A variable's `src` may reference other variables with `<name>`. commando
resolves referenced variables first (adding them as fields if they don't appear
in the template) and substitutes their chosen values before running the
command, so one field's candidates can depend on another:

```toml
[[cmd]]
title = "Show container logs"
desc = "Show a container's logs; candidates narrow from namespace to pod to container."
tmpl = "kubectl logs <pod> -c <container> -n <namespace>"

  [[cmd.var]]
  name = "container"
  src  = "kubectl get pod <pod> -n <namespace> -o jsonpath='{.spec.containers[*].name}' | tr ' ' '\\n'"  # depends on <pod>

  [[cmd.var]]
  name = "pod"
  src  = "kubectl get pods -n <namespace> -o name | cut -d/ -f2"  # depends on <namespace>

  [[cmd.var]]
  name    = "namespace"
  src     = "kubectl get namespaces -o name | cut -d/ -f2"
  default = "default"
```

Here `<namespace>` is resolved first, then `<pod>`, and each chosen value is
substituted before the next field's candidate list is computed. Changing
`namespace` refreshes `pod`'s candidates, and changing `pod` refreshes
`container`'s. A variable referenced only in a `src` (not the command template)
is still added to the form as a field.

Add entries interactively with `commando add`, which discovers the variables in
your template and prompts for their options.

## Data files for cheats

A cheat variable's `src` command runs with `COMMANDO_DATA_DIR` set to
`~/.local/share/commando/data/`. Drop candidate lists there and reference them,
e.g.:

```toml
  [[cmd.var]]
  name = "host"
  src  = 'cat "$COMMANDO_DATA_DIR/hosts.txt"'
```

The `examples/` directory in this repo holds sample cheat files demonstrating
the format, including optional `[ ]` segments, multi-select, shared variable
groups, and cross-variable `src` lookups.

## Configuration

Optional `~/.config/commando/config.toml` — see
[config.example.toml](./config.example.toml). You can change the key binding,
tune the history redaction denylist, and define your own project providers.

The `history_ignore` patterns are globs matched case-insensitively against the
whole command and each word; `*` also matches `/`, and a pattern without a
wildcard matches as a substring. Setting the list replaces the defaults, but
built-in detectors always apply on top of it: cloud access keys, GitHub, Slack,
and OpenAI-style tokens, JWTs, URLs with `user:pass@`, `mysql -p<password>`,
and `Authorization`/`Bearer` headers. The same checks keep secret-looking
values out of argument memory and bookmarks.

## How it works

```
providers (bookmarks · cheats · project · history)
        └─ merge + dedup + frecency rank
              └─ fuzzy TUI  ◄──►  plain-file state (memory + usage)
                    └─ fill-in form (optional/multi/memory + command preview)
                          └─ assembled command → shell prompt
```

**Everything is a plain, human-readable file — there is no database.** You can
`cat`, `grep`, and hand-edit every piece of commando's data:

| File | Contents |
|---|---|
| `~/.config/commando/config.toml` | key binding, redaction denylist, providers |
| `~/.config/commando/cheats/*.toml` | cheat sheets |
| `~/.config/commando/bookmarks.toml` | saved commands + tags/notes |
| `~/.local/share/commando/memory.toml` | remembered variable values (frecency) |
| `~/.local/share/commando/usage.toml` | per-entry launch counts (ranking) |
| `<user cache dir>/commando/` | disposable output of `cache`-enabled `src` and provider commands |
| your `$HISTFILE` (e.g. `~/.zsh_history`) | shell history — read live, never copied |

- **History is read live** from your shell's history file (`$HISTFILE`, then
  the active shell's zsh, Bash, or Fish history path) each time commando runs.
  No capture hook is installed; the shell already maintains that file.
  Secret-looking commands are hidden via the configurable denylist and the
  built-in secret detectors.
- Argument memory is on by default: values are stored per
  `(command, variable, scope)` and offered as most-recently-used-first
  candidates. Set `remember = "none"` on a variable to opt out. See
  [Remembering values](#remembering-values).
- The cache directory is `os.UserCacheDir()/commando` (`~/Library/Caches` on
  macOS, `$XDG_CACHE_HOME` or `~/.cache` on Linux, `%LocalAppData%` on
  Windows), or `$COMMANDO_HOME/cache/commando`. It is safe to delete.
- A memory, usage, or bookmarks file that fails to parse is moved aside to
  `<name>.corrupt-<unix time>` and commando starts with empty state;
  `commando doctor` reports the moved file.
- The binary is pure Go with no CGO, so it builds to a single static binary.

## Development

```sh
make test        # run the full test suite (incl. binary integration test)
make test-short  # skip the slow binary build test
make test-race   # run tests with the race detector
make test-install # test the release installer against local fixtures
make vet
make fmt         # gofmt -w .
make lint        # golangci-lint (see .golangci.yml)
make check       # full local gate: fmt-check + vet + lint + test-race
```

Linting uses [golangci-lint](https://golangci-lint.run) with the config in
`.golangci.yml`. Install it with:

```sh
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

## Status

Zsh, Bash, and Fish summon widgets are supported. Community cheat repositories
remain on the roadmap (see [PROPOSAL.md](./PROPOSAL.md) §14).

## More from Lilium Apps

Commando is made by [Lilium Apps](https://liliumapps.com), an independent
studio. Other things we make:

- [Lumen](https://lumen.liliumapps.com): a privacy-first Android file manager
  with fast search and storage insights.
  [Get it on Google Play](https://play.google.com/store/apps/details?id=com.liliumapps.lumenfiles).
- [VLC Stream and Remote](https://vlc.liliumapps.com): control VLC on your
  computer from your phone.
  [Get it on Google Play](https://play.google.com/store/apps/details?id=tuple.me.vlcremote).

See everything at [liliumapps.com](https://liliumapps.com).

## Credits

Commando is inspired by [navi](https://github.com/denisidoro/navi) by Denis
Isidoro, the interactive cheat sheet tool for the command line. Its cheat
sheets with variables and suggestion lists shaped the core idea here.

## License

[MIT](./LICENSE) © [Lilium Apps](https://liliumapps.com). Commando's home
page is [commando.liliumapps.com](https://commando.liliumapps.com/).
