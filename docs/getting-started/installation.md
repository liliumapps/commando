---
title: Installation & setup
---

# Installation & setup

Commando builds to a **single static binary** — pure Go, no CGO, no runtime
dependencies. Install it, add one line to your shell config, and you're ready.

## Prerequisites

- **Zsh, Bash, or Fish** — for the summon widget. The CLI itself works in any
  shell.
- **Go 1.25 or newer** is required only when building from source.

## Install the binary

=== "Homebrew"

    On macOS:

    ```sh
    brew install --cask liliumapps/tap/commando
    ```

    Homebrew installs the latest release and keeps it updated with
    `brew upgrade --cask commando`.

=== "Install script"

    ```sh
    curl -fsSL https://raw.githubusercontent.com/liliumapps/commando/main/install.sh | sh
    ```

    The script detects macOS, Linux, or Windows under Git Bash and selects
    amd64 or arm64 automatically. It verifies the archive against the release
    checksum before installing to `~/.local/bin`.

    Pin a release or choose another destination:

    ```sh
    curl -fsSL https://raw.githubusercontent.com/liliumapps/commando/main/install.sh \
      | sh -s -- --version v0.1.0 --install-dir /usr/local/bin
    ```

=== "Build from source"

    From the repository root with Go 1.25 or newer:

    ```sh
    make install
    ```

    This builds `commando` and installs it to `~/.local/bin/commando`. Install
    elsewhere with `make install PREFIX=/usr/local`.

    To build without installing:

    ```sh
    make build      # produces ./commando
    make run        # builds, then launches the TUI
    ```

!!! note "Is `~/.local/bin` on your `PATH`?"

    If `commando` isn't found after installing, add to your shell rc file:

    ```sh
    export PATH="$HOME/.local/bin:$PATH"
    ```

Verify the install:

```console
$ commando version
commando 0.1.0
```

## Set up shell integration

The shell integration gives you the **summon widget** — press a key at any
prompt and Commando pops open, then places your chosen command back on the line.
Choose your shell:

=== "Zsh"

    ```sh
    eval "$(commando init zsh)"   # add to ~/.zshrc
    exec zsh
    ```

=== "Bash"

    ```sh
    eval "$(commando init bash)"  # add to ~/.bashrc
    exec bash
    ```

=== "Fish"

    ```fish
    commando init fish | source   # add to ~/.config/fish/config.fish
    exec fish
    ```

Press ++ctrl+g++ at any prompt to summon Commando. On first launch, Commando
seeds starter cheat sheets (git, docker, kubectl, files) so you have something
to search immediately.

### What `commando init` emits

Run `commando init zsh`, `commando init bash`, or `commando init fish` to
preview the snippet. Each uses the shell's native line editor:

| Shell | Line editor API |
|---|---|
| Zsh | ZLE widget + `bindkey` |
| Bash | Readline `bind -x` + `READLINE_LINE`, bound in the emacs, vi-insert, and vi-command keymaps, so the key works with `set -o vi` too |
| Fish | `bind` + `commandline --replace` |

Every snippet installs **no** history capture hook (Commando reads the shell's
history file directly), and it **places** the command on the line rather than
running it — you review, edit, and press ++enter++ yourself.

### Change the summon key

The default is ++ctrl+g++. To rebind, set `key_binding` in your
[config file](../reference/configuration.md#key_binding) using portable `^X`
notation (e.g. `^F` for Ctrl-F), then reload the integration:

```toml
# ~/.config/commando/config.toml
key_binding = "^F"
```

## Next steps

[Take the quickstart tour :material-arrow-right:](quickstart.md){ .md-button .md-button--primary }
