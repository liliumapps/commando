# Sourced (hidden) at the top of every .tape, so none of this shows in the GIF.
# It pins commando entirely inside the sandbox — real config/history can't leak.
# Assumes the current directory is the repo root (where `vhs` is invoked).

SANDBOX="$PWD/docs/recordings/.sandbox"

# Redirect every path commando consults into the sandbox.
export HOME="$SANDBOX"
export XDG_CONFIG_HOME="$SANDBOX/.config"
export XDG_DATA_HOME="$SANDBOX/.local/share"
export HISTFILE="$SANDBOX/.zsh_history"

# VHS can make color auto-detection look like a non-color terminal. Force ANSI
# colors only for recordings so GIFs match the real TUI.
unset NO_COLOR
export TERM="xterm-256color"
export COLORTERM="truecolor"
export CLICOLOR_FORCE=1
export COMMANDO_FORCE_COLOR=1

# Use the sandbox's commando build; keep real system tools on PATH too.
export PATH="$SANDBOX/bin:$PATH"

# Commando normally renders the TUI on stderr so stdout stays clean for shell
# integration. VHS records stdout as the terminal stream, so route stderr into
# stdout only inside the docs recording shell.
commando() {
  command commando "$@" 2>&1
}

# A clean, branded prompt for the recording.
export PS1='%F{045}demo%f %F{078}❯%f '
export PS2=''

# Land in the demo project so project-detection + git cheats have real context.
cd "$SANDBOX/demo"

clear
