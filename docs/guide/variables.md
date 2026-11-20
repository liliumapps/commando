---
title: Variables
---

# Variables

Each `<var>` in a template is backed by a `[[cmd.var]]` block that controls how
its value is resolved — where candidates come from, defaults, multi-select,
previews, and more.

## The full field reference

| Field | Type | Meaning |
|---|---|---|
| `name` | string | **Required.** Matches `<name>` in the template. |
| `src` | string | Shell command whose stdout lines become candidate values. |
| `values` | list | Inline candidate list, e.g. `["dev","prod"]` — no shell needed. Offered before `src` values. |
| `default` | string | Pre-filled initial value. |
| `multi` | bool | Allow selecting multiple values. |
| `sep` | string | Separator joining multiple values (default: a space). |
| `each` | string | Per-item format, `{}` = the value (e.g. `'"{}"'`). |
| `remember` | string | Memory scope: unset (default) = global; or `project`, `dir`, `none`. See [Argument memory](argument-memory.md). |
| `header_lines` | int | Drop this many leading lines from `src` output (e.g. a header row). Not applied to `values`. |
| `delimiter` | string | Regex to split each line into columns for `columns` (default: whitespace runs). |
| `columns` | list | Names for the split columns, referenced as `<name>` in `label`/`value`. Mutually exclusive with `match`. |
| `match` | string | Regex whose capture groups feed `label`/`value`; a non-matching line is dropped. Mutually exclusive with `columns`. |
| `label` | string | Template for the **displayed & searched** text. Defaults to the raw line. |
| `value` | string | Template for the **inserted & remembered** value. Defaults to the raw line. |
| `strict` | bool | Restrict the field to its candidate values; reject free-typed values. |
| `pattern` | string | Regex the resolved value must fully match; a non-matching value is rejected. |
| `preview` | string | Command run for the highlighted candidate (`{}` = the candidate's value); output shown in a preview pane. |
| `cache` | string | Memoize a slow `src` command's output for this Go duration (e.g. `"5m"`), across launches. |

The sections below cover the ones that need more than a line.

## Candidate lists — `src`

A `src` command produces the list of values offered in the form. Each line of
its stdout is one candidate:

```toml
[[cmd.var]]
name = "branch"
src  = "git branch --format='%(refname:short)'"
```

`src` runs in your shell, so pipes, `jq`, `awk`, and friends all work. It also
runs with `COMMANDO_DATA_DIR` set, so you can read from data files:

```toml
[[cmd.var]]
name = "host"
src  = 'cat "$COMMANDO_DATA_DIR/hosts.txt"'
```

## Inline candidates — `values`

For a small fixed choice list, skip `src` and its shell entirely:

```toml
[[cmd.var]]
name   = "env"
values = ["dev", "staging", "prod"]
strict = true
```

`values` entries are treated like `src` lines, so the extraction fields below
apply to them too (but `header_lines` doesn't — inline lists are already clean).
When both are set, `values` are offered first, then `src` output.

## Display vs. inserted value — `columns` / `match`, `label`, `value`

A candidate has **two projections**: what you *see and fuzzy-search* (the label)
and what actually gets *inserted into the command* (the value). By default they
are identical — the whole `src` line. When you need them to differ — a readable
list but only one field inserted — name the line's parts, then template a `label`
and a `value` from them.

The motivating example — kill a process you find *by name*, insert only its PID:

```toml
[[cmd.var]]
name         = "pid"
src          = "ps -eo pid,comm"
header_lines = 1                              # drop the "PID COMMAND" header
match        = '^\s*(?P<pid>\d+)\s+(?P<comm>.+)$'
label        = "<comm>  (<pid>)"              # shown & searched: "chrome  (1234)"
value        = "<pid>"                        # inserted: "1234"
```

Two ways to name a line's parts:

- **`columns`** — split each line on `delimiter` (default: runs of whitespace)
  and name the resulting fields: `columns = ["pid", "comm"]`. Best for
  cleanly-aligned tabular output.
- **`match`** — a regex whose capture groups become the names: named groups
  `(?P<pid>…)` are addressable as `<pid>`. A line that doesn't match is dropped.
  Best for irregular output.

Inside `label`, `value`, and the templates above, a `<name>` is a **column
reference**: `<0>` is the whole line/match, `<1>`, `<2>`, … are fields by
position, and any named column or group is addressable by name. (This differs
from a `<var>` inside a `src` command, which references *another variable* — see
[Cross-variable references](#cross-variable-references).)

The `columns` form of the same example swaps the `match` line for
`columns = ["pid", "comm"]` and is otherwise identical.

!!! note "What gets remembered"
    Memory stores the **value** (e.g. `1234`), not the label. A recalled value
    with no matching line in the current list shows as its bare value. For
    ephemeral data (PIDs, image ids) set `remember = "none"`; for stable data
    (branches, accounts) the value is already readable.

## Validate input — `pattern`

`pattern` is a regex the resolved value must match **in full**, whether typed or
selected. Unlike `strict` (which limits input to the candidate list), `pattern`
constrains the *shape* of any value:

```toml
[[cmd.var]]
name    = "pid"
pattern = '^[0-9]+$'    # only digits
```

## Cache a slow lister — `cache`

When a `src` command is expensive (a network call, a slow API), memoize its
output for a [Go duration](https://pkg.go.dev/time#ParseDuration) so it isn't
re-run every time the field is fetched:

```toml
[[cmd.var]]
name  = "pod"
src   = "kubectl get pods"
cache = "10s"
```

The output is stored in Commando's
[cache directory](../reference/files.md#the-cache-directory), so it is reused
across launches until it expires, not only within one session. Entries are
keyed by the fully resolved command (after `<var>` substitution) and the
working directory, so different dependency values or folders never share a
result. Shortening `cache` takes effect immediately for existing entries.
++ctrl+r++ always re-runs the command and replaces the cached result.

## Multi-select — `multi`, `each`, `sep`

Set `multi = true` to let the user toggle several candidates with ++space++.
Each selected value is formatted through `each` (where `{}` is the value), then
all are joined with `sep`:

```toml
[[cmd.var]]
name  = "file"
multi = true
each  = '"{}"'      # wrap each value in quotes
sep   = " "         # join with a space (the default)
```

Selecting `a.go` and `b.go` yields:

```text
"a.go" "b.go"
```

## Restrict input — `strict`

By default a field accepts free-typed text even when it has candidates. Set
`strict = true` to reject anything not in the candidate list:

```toml
[[cmd.var]]
name   = "region"
src    = "printf 'us-east-1\\nus-west-2\\neu-west-1\\n'"
strict = true       # only these three are accepted
```

If you type a value that isn't a candidate, Commando refuses to finish and
explains why.

## Preview a candidate — `preview`

Show extra context for the highlighted candidate in a preview pane. The
`preview` command runs with `{}` replaced by the candidate's **value** (the
inserted value, not the display label):

```toml
[[cmd.var]]
name         = "image"
src          = "docker images"
header_lines = 1
match        = '^(?P<repo>\S+)\s+(?P<tag>\S+)\s+(?P<id>\S+)'
label        = "<repo>:<tag>"
value        = "<id>"
preview      = "docker image inspect {}"   # {} is the image id
```

As you move through candidates, the pane updates with each one's inspected
output — great for picking the right container, pod, or commit.

## Shared variable groups

Define a set of variables once and reuse them across commands with `use`,
instead of repeating specs. A group is declared with `[[vars.<group>]]`:

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

A command-local `[[cmd.var]]` **overrides** a shared var of the same name, so
you can specialize one field while inheriting the rest.

## Cross-variable references

A variable's `src` may reference **another** variable with `<name>`. Commando
resolves the referenced variable first — even if it only appears in a `src`, not
in the template — and substitutes its value before computing candidates:

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

Here `<namespace>` is resolved first, then `<pod>`. Each chosen value is
substituted before the next field's candidate list is computed — so changing
`namespace` **refreshes** `pod`'s candidates, and changing `pod` refreshes
`container`'s. Referenced variables that aren't already in the template are
added to the form as extra fields automatically.

## Next

<div class="grid cards" markdown>

-   :material-brain: __[Argument memory](argument-memory.md)__

    How `remember` scopes work and how frecency ranks your values.

-   :material-application: __[The TUI](tui.md)__

    How these field types behave key-by-key in the form.

</div>
