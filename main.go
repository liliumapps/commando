// Command commando is a context-aware command launcher: an interactive TUI over
// parameterized cheat sheets, project-detected commands, and live shell
// history, with argument memory (frecency), optional/multi parameters, and a
// live command preview.
//
// This file is the thin entrypoint; all behaviour lives in internal/cli.
package main

import (
	"os"

	"github.com/liliumapps/commando/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
