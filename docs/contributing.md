---
title: Contributing
---

# Contributing

Commando is a pure-Go project with no CGO — it builds to a single static binary.
Development and release builds use Go 1.25 or newer. This page covers the
developer workflow and how to work on these docs.

## Building & testing

```sh
make build       # build ./commando
make test        # run the full test suite (incl. binary integration test)
make test-short  # skip the slow binary build test
make test-race   # run tests with the race detector
make test-install # test the release installer against local fixtures
make vet         # go vet
make fmt         # gofmt -w .
make lint        # golangci-lint (see .golangci.yml)
make check       # full local gate: fmt-check + vet + lint + test-race
```

Run `make check` before sending a change — it's the same gate contributors are
expected to pass.

CI runs formatting, vet, lint, and race-enabled tests on Linux and macOS. It
also parses generated Zsh, Bash, and Fish snippets with the real shells. Strict
documentation builds run for documentation pull requests.

### Linting

Linting uses [golangci-lint](https://golangci-lint.run) with the config in
`.golangci.yml`. Install it with:

```sh
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

## Project layout

Commando follows the entry/provider architecture described in
[Concepts](guide/concepts.md):

```text
main.go                    thin entrypoint → internal/cli
cheats/                    built-in cheat packs (embedded; contribute new ones here)
internal/
├── cli/                   command dispatch: search, init, add, history, bookmark, cheats
├── app/                   wiring: aggregate providers, post-process, backend
├── entry/                 the Entry + VarSpec contract
├── provider/              providers that emit entries
│   ├── cheatfile/         TOML cheat sheets
│   ├── project/           make, npm, cargo, gradle, just, compose, git
│   └── history/           live shell-history reader
├── resolve/               template resolution, cross-var refs
├── template/              template parsing (<var>, [optional])
├── rank/                  frecency ranking + fuzzy matching
├── state/                 plain-file persistence (memory, usage, fills)
├── bookmark/              bookmark store
├── config/                config load + validate, seeded cheats
├── sources/               cheat sources: git repos, review, loading
├── shell/                 zsh, Bash, and Fish integration snippets
└── tui/                   Bubble Tea: list view + fill-in form + preview
```

The resolver is kept **UI-agnostic** so a future non-TUI front-end stays
possible. When adding a new command source, prefer writing a new **provider**
over special-casing the resolver.

## Roadmap

From the design proposal, still open:

- **Community cheat repos** — `commando repo add <src>` to pull shared cheats.
- **Auto-cheats** — promote frequently-run history commands into draft cheat
  entries.

See [`PROPOSAL.md`](https://github.com/liliumapps/commando/blob/main/PROPOSAL.md)
in the repo for the full design rationale.

## Releasing

Push a semantic-version tag such as `v0.2.0`. The release workflow runs tests,
builds static archives for Linux, macOS, and Windows on amd64 and arm64, injects
the tag into `commando version`, publishes checksums, and creates the GitHub
release through GoReleaser. It also updates the `commando` cask in
`liliumapps/homebrew-tap`.

Before the first public release:

1. Make `liliumapps/commando` public so release archives and `install.sh` are
   available without authentication.
2. Create a public `liliumapps/homebrew-tap` repository.
3. Generate an SSH key pair, add the public key to the tap as a deploy key with
   write access, and store the private key as the `HOMEBREW_TAP_DEPLOY_KEY`
   Actions secret in `liliumapps/commando`. The key can push only to the tap.

Local source builds report `commando dev`. `make build VERSION=0.2.0` exercises
the same link-time version injection without creating a release.

Test the packaging locally without publishing:

```sh
goreleaser release --snapshot --clean
```
