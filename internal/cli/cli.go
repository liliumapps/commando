// Package cli implements commando's command-line dispatch. It parses the first
// argument as a subcommand and routes to the appropriate handler.
package cli

import (
	"fmt"
	"io"
	"os"
)

// version is the build version reported by `commando version`. It is a var so
// it can be overridden at link time (-ldflags "-X ...").
var version = "dev"

// Main runs the CLI with the given arguments (excluding the program name) and
// returns a process exit code.
func Main(args []string) int {
	return run(args, os.Stdout, os.Stderr)
}

// run is the testable core of Main, with output streams injected.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		// Default action: open the interactive search TUI and print the chosen
		// command to stdout.
		return cmdSearch(nil, stdout, stderr)
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "search":
		return cmdSearch(rest, stdout, stderr)
	case "init":
		return cmdInit(rest, stdout, stderr)
	case "run":
		return cmdRun(rest, stdout, stderr)
	case "add":
		return cmdAdd(rest, stdout, stderr)
	case "history":
		return cmdHistory(rest, stdout, stderr)
	case "doctor":
		return cmdDoctor(rest, stdout, stderr)
	case "state":
		return cmdState(rest, stdout, stderr)
	case "promote":
		return cmdPromote(rest, stdout, stderr)
	case "bookmark", "bm":
		return cmdBookmark(rest, stdout, stderr)
	case "cheats":
		return cmdCheats(rest, stdout, stderr)
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "commando %s\n", version)
		return 0
	case "help", "--help", "-h":
		usage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "commando: unknown command %q\n\n", cmd)
		usage(stderr)
		return 2
	}
}

func usage(w io.Writer) {
	fmt.Fprint(w, `commando — context-aware command launcher

Usage:
  commando [command]

Commands:
  (no args)          Open the interactive search TUI
  search [--print]   Open the TUI; --print writes the chosen command to stdout
  init <shell>       Print integration for zsh, bash, or fish
  run                Alias for search
  add                Interactively add a cheat entry
  history [query]    Search your shell history (read live from the history file)
  doctor             Diagnose config, state, history, providers, and shell setup
  state clear ...    Clear remembered arguments or usage ranking
  promote [flags]    Promote a command or history entry into a managed cheat
  bookmark ...       Manage bookmarks: add/list/remove (also: bm)
  cheats ...         Manage cheat sources (git repos): add/list/update/remove
  version            Print the version
  help               Show this help

Shell setup:
  eval "$(commando init zsh)"   # or bash
  commando init fish | source
`)
}
