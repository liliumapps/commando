package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/liliumapps/commando/internal/config"
	"github.com/liliumapps/commando/internal/sources"
)

const cheatsUsage = `usage:
  commando cheats add [--yes] <git-url> [file]   add a repository, or one file in it
  commando cheats list                           list sources
  commando cheats update [--yes]                 update every source
  commando cheats remove <git-url> [file]        remove a source`

// cmdCheats dispatches the cheat source subcommands: add, list, update,
// remove.
func cmdCheats(args []string, stdout, stderr io.Writer) int {
	return runCheats(args, stdout, stderr, os.Stdin, stdinIsTerminal())
}

func runCheats(args []string, stdout, stderr io.Writer, stdin io.Reader, tty bool) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, cheatsUsage)
		return 2
	}
	sub, rest := args[0], args[1:]
	assumeYes := false
	var pos []string
	for _, a := range rest {
		switch {
		case a == "--yes" || a == "-yes" || a == "-y":
			assumeYes = true
		case strings.HasPrefix(a, "-") && a != "-":
			fmt.Fprintf(stderr, "commando cheats %s: unknown flag %s\n%s\n", sub, a, cheatsUsage)
			return 2
		default:
			pos = append(pos, a)
		}
	}

	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintf(stderr, "commando: %v\n", err)
		return 1
	}
	m := &sources.Manager{Paths: paths, Out: stdout, Confirm: confirmer(stdout, stderr, stdin, tty, assumeYes)}

	switch sub {
	case "add":
		if len(pos) < 1 || len(pos) > 2 {
			fmt.Fprintln(stderr, cheatsUsage)
			return 2
		}
		file := ""
		if len(pos) == 2 {
			file = pos[1]
		}
		err = m.Add(pos[0], file)
	case "list", "ls":
		if len(pos) != 0 || assumeYes {
			fmt.Fprintln(stderr, cheatsUsage)
			return 2
		}
		err = m.List()
	case "update":
		if len(pos) != 0 {
			fmt.Fprintln(stderr, cheatsUsage)
			return 2
		}
		err = m.Update()
	case "remove", "rm":
		if len(pos) < 1 || len(pos) > 2 || assumeYes {
			fmt.Fprintln(stderr, cheatsUsage)
			return 2
		}
		file := ""
		if len(pos) == 2 {
			file = pos[1]
		}
		err = m.Remove(pos[0], file)
	default:
		fmt.Fprintf(stderr, "commando cheats: unknown subcommand %q\n%s\n", sub, cheatsUsage)
		return 2
	}
	if err != nil {
		if sources.IsDeclined(err) {
			fmt.Fprintln(stderr, "commando cheats: not trusted; nothing changed")
		} else {
			fmt.Fprintf(stderr, "commando cheats %s: %v\n", sub, err)
		}
		return 1
	}
	return 0
}

// confirmer asks "Trust these commands? [y/N]" on the terminal. --yes trusts
// without asking; without a terminal (and without --yes) nothing is trusted.
func confirmer(stdout, stderr io.Writer, stdin io.Reader, tty, assumeYes bool) func() bool {
	in := bufio.NewReader(stdin)
	return func() bool {
		if assumeYes {
			fmt.Fprintln(stdout, "Trusted (--yes).")
			return true
		}
		if !tty {
			fmt.Fprintln(stderr, "commando cheats: stdin is not a terminal; review the commands above and re-run with --yes to trust them")
			return false
		}
		fmt.Fprint(stdout, "Trust these commands? [y/N] ")
		line, _ := in.ReadString('\n')
		return yes(strings.TrimSpace(line))
	}
}

func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
