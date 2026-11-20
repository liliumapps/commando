---
title: Shell history
---

# Shell history

Your shell already keeps a record of everything you run. Commando reads it
**live** and folds it into the same fuzzy list as your cheats and project
commands — so a command you ran last week is one search away.

## Read live, never copied

Commando does **not** install a capture hook and does **not** keep its own copy
of your history. Each time it runs, it reads your shell's history file directly.
This means:

- No `preexec`/`precmd` hook slowing down your prompt.
- No second, drifting copy of your history to manage.
- Nothing to migrate or back up — your shell already owns the file.

### Which file is read

Commando looks for your history file in this order:

1. `$HISTFILE` (whatever your shell has set)
2. The active shell's standard path:
   `~/.zsh_history`, `~/.bash_history`, or
   `$XDG_DATA_HOME/fish/fish_history`
3. The remaining standard paths as fallbacks

The first one that exists is used.

### How the file is parsed

Commando understands zsh's extended format (`: <time>:<elapsed>;<command>`),
including non-ASCII text, which zsh stores in an internal escaped form. For
Bash, `HISTTIMEFORMAT` timestamp lines (`#<unix seconds>`) are read, and the
lines between two timestamps form one command, so multi-line commands saved
with `shopt -s lithist` stay whole. A line ending in an unescaped backslash
continues onto the next line in either shell. When the file carries
timestamps, commands are ordered by when they ran rather than by file
position, which matters for Bash with `histappend`, where each session's
commands are appended at exit.

## Searching history

Inside the TUI, history entries appear alongside everything else — just type to
filter. From the command line, use the `history` subcommand:

```sh
commando history              # recent commands
commando history docker       # commands containing "docker"
commando history --limit 100 kubectl
```

The query is a case-insensitive substring match. Results print newest-first.

!!! note "List limit vs. search"

    When you're **not** searching, the TUI caps how many history commands appear
    (default 10) so the list stays focused on curated cheats and project
    commands. As soon as you type a query, all matching history in the loaded
    window is shown. Tune this with `history.list_limit` in
    [config](../reference/configuration.md).

## Redaction

Commands that look like they contain secrets are **hidden** from the history
list via a configurable, `HISTIGNORE`-style denylist. The defaults cover common
cases:

```toml
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
```

Matching rules:

- Patterns are matched **case-insensitively**, against the whole command and
  against each whitespace-separated word.
- A pattern with no wildcard is a **substring** match.
- `*` matches any run of characters, including `/`, so `*password*` catches
  `mysql --password=a/b`. `?` matches one character and `[ ]` is a character
  class (`[!...]` negates).

On top of the list, built-in detectors always hide commands containing
secret-shaped values, even when you replace the list: AWS access key ids,
GitHub, Slack, and OpenAI-style (`sk-`) tokens, JWTs, URLs with embedded
`user:password@`, an inline `mysql -p<password>`, and `Authorization:` or
`Bearer` headers.

The same checks (patterns and detectors) protect Commando's own files. A value
that looks like a secret, by itself or as `<variable>=<value>`, is not added to
[argument memory](argument-memory.md). The TUI refuses to bookmark a
secret-looking command, and `commando bookmark add` refuses it unless you pass
`--force`.

Add your own patterns in [`config.toml`](../reference/configuration.md#history-redaction)
to hide anything project- or company-specific.

!!! warning "Redaction hides; it doesn't delete"

    The denylist stops secret-looking commands from appearing in Commando's
    list. It does **not** edit your shell history file. To keep secrets out of
    history entirely, use your shell's own `HISTIGNORE`/`setopt HIST_IGNORE_SPACE`
    features. And to keep secret *values* out of Commando's memory, set
    [`remember = "none"`](argument-memory.md#secrets-always-use-remember-none).

## Performance on large histories

Commando reads only the newest 8 MiB of the history file and retains up to 5,000
unique commands by default. This keeps startup predictable even when a shell
history has grown for years. Tune either bound:

```toml
[history]
max_load = 10000
max_bytes = 16777216   # newest 16 MiB
```

Set either value to a negative number for unlimited behavior. A zero value keeps
the default.

## Next

<div class="grid cards" markdown>

-   :material-bookmark: __[Bookmarks](bookmarks.md)__

    Promote a hard-to-remember command into a saved, tagged entry.

-   :material-cog: __[Configuration](../reference/configuration.md)__

    Tune list limits, load caps, and the redaction denylist.

</div>
