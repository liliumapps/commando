package cli

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/liliumapps/commando/internal/config"
	"github.com/liliumapps/commando/internal/template"
)

// cmdPromote turns an existing literal command or a user-supplied parameterized
// template into a managed cheat without an interactive prompt.
func cmdPromote(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("promote", flag.ContinueOnError)
	fs.SetOutput(stderr)
	title := fs.String("title", "", "short command title")
	desc := fs.String("desc", "", "searchable description")
	tags := fs.String("tags", "", "comma-separated searchable tags")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	tmpl := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if tmpl == "" {
		fmt.Fprintln(stderr, "commando promote: a command or template is required")
		return 2
	}
	parsed, err := template.Parse(tmpl)
	if err != nil {
		fmt.Fprintf(stderr, "commando promote: invalid template: %v\n", err)
		return 2
	}
	if strings.TrimSpace(*title) == "" {
		*title = tmpl
	}

	var fragment strings.Builder
	fragment.WriteString("\n[[cmd]]\n")
	fmt.Fprintf(&fragment, "title = %s\n", tomlString(strings.TrimSpace(*title)))
	if value := strings.TrimSpace(*desc); value != "" {
		fmt.Fprintf(&fragment, "desc = %s\n", tomlString(value))
	}
	fmt.Fprintf(&fragment, "tmpl = %s\n", tomlString(tmpl))
	if values := splitCSV(*tags); len(values) > 0 {
		fmt.Fprintf(&fragment, "tags = [%s]\n", tomlStringList(values))
	}
	for _, ref := range parsed.Vars() {
		fragment.WriteString("\n  [[cmd.var]]\n")
		fmt.Fprintf(&fragment, "  name = %s\n", tomlString(ref.Name))
		fragment.WriteString("  remember = \"none\"\n")
	}

	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintf(stderr, "commando promote: %v\n", err)
		return 1
	}
	if err := paths.EnsureDirs(); err != nil {
		fmt.Fprintf(stderr, "commando promote: %v\n", err)
		return 1
	}
	dst := filepath.Join(paths.CheatsDir, "user.toml")
	if err := appendCheat(dst, []byte(fragment.String())); err != nil {
		fmt.Fprintf(stderr, "commando promote: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "promoted to %s\n", dst)
	return 0
}
