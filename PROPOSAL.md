# commando - Context-Aware Command Launcher

> **Status:** Implemented as v0.1.0 and updated for the title/description TUI
> model. This file is the project context and architecture note; see
> [README.md](./README.md) for user-facing usage.

---

## 1. Summary

`commando` is an interactive, context-aware command launcher for the terminal.
It unifies four sources of commands behind one fuzzy TUI:

1. **Cheat sheets** - parameterized, reusable command templates.
2. **Project commands** - `make`, `npm`/`pnpm`/`yarn`, `cargo`, Gradle, `just`,
   docker-compose, and git actions detected from the current project.
3. **Shell history** - read live from the user's own history file.
4. **Bookmarks** - saved commands with searchable tags and notes.

The selected command is assembled and placed on the shell prompt for review by
default. `--exec` exists for direct execution, but the normal path keeps the
shell in control so `cd`, aliases, exports, and last-minute edits work as users
expect.

Key differentiators:

- **One resolver for every source.** Cheats, project commands, history, and
  bookmarks all become `entry.Entry` values.
- **Optional segments and multi-select values.** `[ ... <var> ... ]` drops as a
  whole when skipped; multi values are formatted and joined by the resolver.
- **Argument memory with frecency.** Values are remembered by
  `(command, variable, scope)` unless a variable opts out.
- **Display/value split for candidates.** A list can show readable labels while
  inserting only the value needed by the command.
- **Plain files, no database.** State lives in TOML files under XDG config/data
  directories; shell history is read live and never copied.

---

## 2. Design Principles

1. **One resolution engine, many sources.** Providers emit a shared entry shape;
   the resolver does not care where an entry came from.
2. **The shell owns execution.** The default action is prompt injection, not
   execution.
3. **Everything is inspectable.** Cheats, bookmarks, argument memory, usage, and
   config are plain text.
4. **Zero-config is useful; config is powerful.** Built-in providers and seeded
   cheats give immediate value; custom providers and cheats add depth.
5. **The UI should reduce ambiguity.** The list shows a compact title and the
   command in separate columns; selected details appear once in the footer.

---

## 3. Core Model

`entry.Entry` is the central contract.

```go
type Entry struct {
    Title    string    // short name shown in the result list
    Desc     string    // optional selected-entry detail
    Tags     []string
    Template string    // template text or literal command text
    Kind     CommandKind
    Vars     []VarSpec
    Source   Source
    LastUsed int64
    UseCount int
}
```

Important behavior:

- `TitleText()` returns `Title`, falling back to legacy `Desc`, then
  `Template`.
- `DescText()` returns `Desc` when it is distinct; otherwise it returns the
  title so the footer always has useful text.
- Search text is `title + desc + command + tags`.
- Entry IDs include source, template, and title. `desc` is intentionally
  excluded so adding explanatory detail does not reset memory.
- `Kind` distinguishes parsed command templates from literal commands.
  History, bookmarks, and project-detected commands are literals.

Sources:

```go
const (
    SourceCheat Source = iota
    SourceProject
    SourceHistory
    SourceBookmark
)
```

---

## 4. Cheat File Format

Cheat files live in `~/.config/commando/cheats/*.toml`.

```toml
[[cmd]]
title = "Search git log"
desc = "Search git log by author for a file."
tmpl = "git log [--author=<author>] [-n <count>] -- <file>"
tags = ["git", "log"]

  [[cmd.var]]
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

Entry fields:

| Field | Required | Meaning |
|---|---|---|
| `title` | yes | Short searchable name shown in the list. |
| `desc` | no | Longer searchable detail shown in the selected footer. |
| `tmpl` | yes | Command template with `<var>` and optional `[ ... ]` segments. |
| `tags` | no | Extra searchable keywords. |
| `use` | no | Shared variable groups to merge into this command. |
| `[[cmd.var]]` | no | Variable specs. |

Backward compatibility: old `desc`-only cheats still load. When `title` is
missing, `desc` becomes the title and the separate description is empty.

`commando add` writes the new schema by prompting for:

1. title,
2. optional description,
3. command template,
4. optional tags,
5. each discovered variable's options.

---

## 5. Variable Specs

`entry.VarSpec` controls how a `<name>` placeholder is resolved.

```go
type VarSpec struct {
    Name     string
    Src      string
    Values   []string
    Default  string
    Multi    bool
    Sep      string
    Each     string
    Remember Scope

    HeaderLines int
    Delimiter   string
    Columns     []string
    Match       string
    Label       string
    Value       string

    Strict  bool
    Pattern string
    Preview string
    Cache   string
}
```

Candidate extraction:

- `values` provides inline candidates.
- `src` runs in the user's shell and returns one candidate per stdout line.
- `header_lines` drops table headers.
- `columns` or `match` names fields in each candidate line.
- `label` is shown and searched.
- `value` is inserted, remembered, and substituted into dependent `src`
  commands.

Example: show `chrome (1234)` but insert `1234`.

```toml
[[cmd.var]]
name         = "pid"
src          = "ps -eo pid,comm"
header_lines = 1
match        = '^\s*(?P<pid>\d+)\s+(?P<comm>.+)$'
label        = "<comm>  (<pid>)"
value        = "<pid>"
pattern      = '^[0-9]+$'
remember     = "none"
```

Memory defaults to `global`; set `remember = "none"` for secrets or ephemeral
values.

---

## 6. Resolver Behavior

The resolver parses the template and walks it left to right:

1. Required `<var>` placeholders must resolve to a value.
2. Optional `[ ... <var> ... ]` segments disappear as a whole when skipped.
3. Multi-select values are formatted through `each` and joined by `sep`.
4. Cross-variable references in a variable's `src` resolve their dependencies
   first, even if those dependencies are not in the command template.
5. `strict` rejects values outside the candidate list.
6. `pattern` validates the final inserted value.

Template example:

```text
git log [--author=<author>] [-n <count>] -- <file>
```

If `author` is skipped, `--author=<author>` is removed entirely. This is the
core reason optional flags do not dangle.

---

## 7. TUI Interaction Model

### Search List

The list is bottom-anchored: the best match sits nearest the input. Rows use a
compact source code, then a title column and a command column.

```text
  h  git status                     git status
  p  npm build                      npm run build
❯ c  Search git log                 git log [--author=<author>] [-n <count>] -- <file>
scope: all────────────────────────────────────────────3/42 commando
Search git log by author for a file.
git log [--author=<author>] [-n <count>] -- <file>
────────────────────────────────────────────────────────
❯ log auth▊
↑↓ move  ←→ edit  ⏎ select  tab scope  ^b bookmark  esc quit
```

Search matches title, description, command, and tags. Scope can be changed with
`:c`, `:h`, `:b`, or by cycling with Tab / Shift-Tab.

### Fill-In Form

The form separates overview from editing:

- Top box: title, command preview, parameter assignments, optional preview
  pane.
- Middle: candidate list for the active parameter.
- Bottom: active parameter input and key hints.

```text
╭────────────────────────────────────────────────────────╮
│ Search git log                                         │
│ command:                                               │
│   git log -n 20 -- <file>                              │
│                                                        │
│ params:                                                │
│   author (optional) = (skipped)                        │
│   count (optional) = 20                                │
│ ❯ file (required, multi) = ‹unset›                     │
╰────────────────────────────────────────────────────────╯
    helper.go
❯ ◯ main.go
❯ file (multi) = ▊
↑↓ candidates  space toggle  ⏎ run  ^p/^n params  ^d clear
```

Key decisions:

- Left/right edit the text cursor.
- Ctrl-P/Ctrl-N move between parameters.
- Tab/Shift-Tab jump between required parameters.
- Space toggles multi-select candidates and types a literal space otherwise.
- Ctrl-S skips optional fields.

---

## 8. State and Storage

Commando follows XDG paths:

| Path | Holds |
|---|---|
| `~/.config/commando/config.toml` | Key binding, redaction denylist, custom providers. |
| `~/.config/commando/cheats/*.toml` | Cheat sheets. |
| `~/.config/commando/bookmarks.toml` | Saved commands with tags and notes. |
| `~/.local/share/commando/memory.toml` | Remembered variable values. |
| `~/.local/share/commando/usage.toml` | Per-entry use counts and last-used times. |
| `~/.local/share/commando/data/` | Auxiliary data files for cheat `src` commands. |
| `$HISTFILE` | Shell history, read live and never copied. |

There is no SQLite store. The shell integration installs a ZLE widget only; it
does not install `preexec` or `precmd` capture hooks.

---

## 9. Providers

Built-in providers:

| Provider | Detection / source | Entries |
|---|---|---|
| Cheats | `~/.config/commando/cheats/*.toml` | Authored templates. |
| Project | Marker files while walking up from cwd | `make`, package scripts, cargo, Gradle, just, compose, git actions. |
| History | `$HISTFILE` and fallbacks | Literal past shell commands. |
| Bookmarks | `bookmarks.toml` | Literal saved commands. |

Custom project provider example:

```toml
[[provider]]
name     = "task"
detect   = "Taskfile.yml"
list     = "task --list-all --silent | awk '{print $2}'"
template = "task {}"
cache    = "5m"
```

---

## 10. CLI Surface

```text
commando                    open the interactive search TUI
commando search --print     print the chosen command to stdout
commando search --exec      run the chosen command directly
commando init zsh           print zsh widget setup
commando add                author a cheat entry
commando history [query]    search shell history read live from the file
commando bookmark ...       add/list/remove bookmarks
commando version            print version
```

---

## 11. Package Layout

```text
internal/app/        provider aggregation, usage merge, runtime context
internal/bookmark/   bookmark storage and conversion into entries
internal/cli/        command dispatch and shell execution path
internal/config/     config, cheat TOML, seeded cheats, paths
internal/entry/      Entry, VarSpec, source/kind/scope contracts
internal/provider/   history and project providers
internal/rank/       search, scope parsing, frecency ordering
internal/resolve/    template parsing and command assembly
internal/tui/        Bubble Tea list and fill-in form
```

---

## 12. Deferred Work

- Bash/fish shell widgets.
- Community cheat repositories.
- Optional alternate front-ends using the same resolver.
- Auto-suggesting cheat drafts from repeated history patterns.
