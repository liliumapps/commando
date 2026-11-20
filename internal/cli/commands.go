package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/liliumapps/commando/internal/app"
	"github.com/liliumapps/commando/internal/config"
	"github.com/liliumapps/commando/internal/shell"
)

// cmdSearch opens the interactive TUI. With --print (used by the shell widget)
// the chosen command is written to stdout; otherwise it is echoed for the user.
func cmdSearch(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(stderr)
	// --print is accepted for explicitness in the shell widget; the command is
	// always written to stdout regardless, so it needs no separate handling.
	_ = fs.Bool("print", false, "write the chosen command to stdout (for shell widgets)")
	execIt := fs.Bool("exec", false, "run the chosen command directly instead of printing")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	a, err := app.Open()
	if err != nil {
		fmt.Fprintf(stderr, "commando: %v\n", err)
		return 1
	}
	defer a.Close()

	res, err := a.Search()
	if err != nil {
		fmt.Fprintf(stderr, "commando: %v\n", err)
		return 1
	}
	if res.Aborted || res.Command == "" {
		return 130 // conventional exit code for user cancel
	}

	if *execIt {
		return runShellCommand(res.Command, stdout, stderr)
	}

	// Print the assembled command to stdout. The shell widget captures stdout
	// and places the command on the prompt line; a user running `search`
	// directly sees it printed so they can copy or pipe it.
	fmt.Fprintln(stdout, res.Command)
	return 0
}

// cmdInit prints shell integration for the requested shell.
func cmdInit(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "commando init: specify one shell (zsh|bash|fish)")
		return 2
	}
	shellName := args[0]

	// Load config only to read the configured key binding; fall back to default.
	keyBinding := "^G"
	if paths, err := config.DefaultPaths(); err == nil {
		if cfg, err := config.Load(paths); err == nil {
			keyBinding = cfg.KeyBinding
		}
	}

	switch shellName {
	case "zsh":
		fmt.Fprint(stdout, shell.ZshInit(keyBinding))
		return 0
	case "bash":
		fmt.Fprint(stdout, shell.BashInit(keyBinding))
		return 0
	case "fish":
		fmt.Fprint(stdout, shell.FishInit(keyBinding))
		return 0
	default:
		fmt.Fprintf(stderr, "commando init: unsupported shell %q (supported: zsh, bash, fish)\n", shellName)
		return 2
	}
}

// cmdRun is an alias for search.
func cmdRun(args []string, stdout, stderr io.Writer) int {
	return cmdSearch(args, stdout, stderr)
}

func cmdDoctor(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		fmt.Fprintln(stderr, "commando doctor: no arguments expected")
		return 2
	}
	failed := false
	for _, check := range app.Doctor() {
		fmt.Fprintf(stdout, "%-5s %-12s %s\n", check.Status, check.Name, check.Detail)
		if check.Status == app.CheckError {
			failed = true
		}
	}
	if failed {
		return 1
	}
	return 0
}

func cmdState(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 || args[0] != "clear" {
		fmt.Fprintln(stderr, "usage: commando state clear memory|usage|all")
		return 2
	}
	a, err := app.Open()
	if err != nil {
		fmt.Fprintf(stderr, "commando: %v\n", err)
		return 1
	}
	defer a.Close()

	switch args[1] {
	case "memory":
		err = a.State.ClearMemory()
	case "usage":
		err = a.State.ClearUsage()
	case "all":
		if err = a.State.ClearMemory(); err == nil {
			err = a.State.ClearUsage()
		}
	default:
		fmt.Fprintln(stderr, "usage: commando state clear memory|usage|all")
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "commando: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "cleared %s state\n", args[1])
	return 0
}

// cmdHistory searches the shell history file. Positional args form a substring
// query; matching commands are printed newest-first. History is read live from
// the shell history file — commando never stores a copy.
func cmdHistory(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("history", flag.ContinueOnError)
	fs.SetOutput(stderr)
	limit := fs.Int("limit", 50, "maximum results")
	// Allow the query words to appear before or after flags, so both
	// `history --limit 3 deploy` and `history deploy --limit 3` work.
	if err := fs.Parse(reorderFlagsFirst(args)); err != nil {
		return 2
	}

	a, err := app.Open()
	if err != nil {
		fmt.Fprintf(stderr, "commando: %v\n", err)
		return 1
	}
	defer a.Close()

	entries, err := a.HistoryEntries()
	if err != nil {
		fmt.Fprintf(stderr, "commando: %v\n", err)
		return 1
	}

	query := strings.ToLower(strings.Join(fs.Args(), " "))
	printed := 0
	for _, e := range entries {
		if *limit > 0 && printed >= *limit {
			break
		}
		if query != "" && !strings.Contains(strings.ToLower(e.SearchText()), query) {
			continue
		}
		fmt.Fprintln(stdout, e.TitleText())
		printed++
	}
	return 0
}

// reorderFlagsFirst moves flag arguments (tokens starting with "-", plus the
// value that follows a "--limit N"-style flag) ahead of positional arguments,
// so a query can be typed before flags. Go's flag package otherwise stops at the
// first positional, silently treating trailing flags as query text.
//
// It recognises the history command's flags. "--flag=value" and bare boolean
// flags need no special handling; only space-separated values are pulled along.
func reorderFlagsFirst(args []string) []string {
	valueFlags := map[string]bool{"-limit": true, "--limit": true}
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			// Pull along a separate value token for value-taking flags that did
			// not use "=".
			if valueFlags[a] && !strings.Contains(a, "=") && i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		positional = append(positional, a)
	}
	return append(flags, positional...)
}

// runShellCommand executes cmd via the user's shell. Its usage is recorded by
// the TUI flow (via the entry ID); the shell itself records it in the shell
// history file, so commando does not separately persist it.
func runShellCommand(cmd string, stdout, stderr io.Writer) int {
	return execViaShell(cmd, stdout, stderr)
}
