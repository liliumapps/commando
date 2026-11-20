# Cheat packs

The cheat packs that ship with commando. Each `*.toml` file here is embedded
in the binary and seeded into `~/.config/commando/cheats/` on first run (a
pack you delete is not recreated). The same files work as a cheat source
straight from this repository, so you can follow one pack without copying it:

```sh
commando cheats add https://github.com/liliumapps/commando cheats/docker.toml
```

## Format

A pack is a TOML file of `[[cmd]]` entries, each with a `title`, a `tmpl`
command template, optional `desc` and `tags`, and one `[[cmd.var]]` block per
variable. Shared variables go in `[[vars.<group>]]` and are pulled in with
`use = ["<group>"]`. The full format is in the
[cheat sheet guide](https://commando.liliumapps.com/guide/cheat-sheets/) and
the [variables reference](https://commando.liliumapps.com/guide/variables/).

```toml
[[cmd]]
title = "Tail container logs"
tmpl  = "docker logs -f [--tail <lines>] <container>"
tags  = ["docker", "logs"]

  [[cmd.var]]
  name = "container"
  src  = "docker ps --format '{{.Names}}'"
```

## Contributing a pack

- One tool or topic per file, named after it (`terraform.toml`).
- Keep commands generic: no hostnames, accounts, or paths from your setup.
- `src` and `preview` commands run on the user's machine whenever the form
  opens. Keep them read-only and quick (listing branches, containers,
  profiles), and never let them change anything.
- Set `remember = "none"` on variables that hold one-off or sensitive values.
- Run `go test ./internal/config/` before opening a pull request; it loads
  and assembles every pack here.
