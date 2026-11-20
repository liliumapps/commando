package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/liliumapps/commando/internal/app"
	"github.com/liliumapps/commando/internal/bookmark"
	"github.com/liliumapps/commando/internal/config"
)

// cmdBookmark dispatches the bookmark subcommands: add, list, remove.
func cmdBookmark(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return cmdBookmarkList(nil, stdout, stderr)
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "add":
		return cmdBookmarkAdd(rest, stdout, stderr)
	case "list", "ls":
		return cmdBookmarkList(rest, stdout, stderr)
	case "remove", "rm":
		return cmdBookmarkRemove(rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "commando bookmark: unknown subcommand %q (add|list|remove)\n", sub)
		return 2
	}
}

func cmdBookmarkAdd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("bookmark add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	tags := fs.String("tags", "", "comma-separated searchable tags")
	note := fs.String("note", "", "optional searchable note")
	force := fs.Bool("force", false, "save even if the command looks like it contains a secret")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cmd := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if cmd == "" {
		fmt.Fprintln(stderr, "commando bookmark add: a command is required")
		return 2
	}

	a, err := app.Open()
	if err != nil {
		fmt.Fprintf(stderr, "commando: %v\n", err)
		return 1
	}
	defer a.Close()

	if !*force && config.ShouldIgnore(a.Config.HistoryIgnore, cmd) {
		fmt.Fprintln(stderr, "commando bookmark add: the command looks like it contains a secret; bookmarks are stored in plain text (use --force to save anyway)")
		return 1
	}

	bm := bookmark.Bookmark{
		Cmd:     cmd,
		Tags:    splitCSV(*tags),
		Note:    strings.TrimSpace(*note),
		Created: time.Now().Unix(),
	}
	if err := a.Bookmarks.Add(bm); err != nil {
		fmt.Fprintf(stderr, "commando: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "bookmarked: %s\n", cmd)
	return 0
}

func cmdBookmarkList(args []string, stdout, stderr io.Writer) int {
	a, err := app.Open()
	if err != nil {
		fmt.Fprintf(stderr, "commando: %v\n", err)
		return 1
	}
	defer a.Close()

	bs, err := a.Bookmarks.List()
	if err != nil {
		fmt.Fprintf(stderr, "commando: %v\n", err)
		return 1
	}
	for _, b := range bs {
		line := b.Cmd
		if len(b.Tags) > 0 {
			line += "  [" + strings.Join(b.Tags, " ") + "]"
		}
		if b.Note != "" {
			line += "  — " + b.Note
		}
		fmt.Fprintln(stdout, line)
	}
	return 0
}

func cmdBookmarkRemove(args []string, stdout, stderr io.Writer) int {
	cmd := strings.TrimSpace(strings.Join(args, " "))
	if cmd == "" {
		fmt.Fprintln(stderr, "commando bookmark remove: specify the exact command to remove")
		return 2
	}
	a, err := app.Open()
	if err != nil {
		fmt.Fprintf(stderr, "commando: %v\n", err)
		return 1
	}
	defer a.Close()

	removed, err := a.Bookmarks.Remove(cmd)
	if err != nil {
		fmt.Fprintf(stderr, "commando: %v\n", err)
		return 1
	}
	if !removed {
		fmt.Fprintf(stderr, "no bookmark matched: %s\n", cmd)
		return 1
	}
	fmt.Fprintf(stdout, "removed: %s\n", cmd)
	return 0
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
