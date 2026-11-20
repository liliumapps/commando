package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/liliumapps/commando/internal/config"
	"github.com/liliumapps/commando/internal/persist"
	"github.com/liliumapps/commando/internal/template"
)

// cmdAdd interactively authors a cheat entry and appends it to a user cheat
// file (cheats/user.toml). It parses the template to discover variables and
// prompts for each one's options, then writes valid TOML.
func cmdAdd(args []string, stdout, stderr io.Writer) int {
	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintf(stderr, "commando: %v\n", err)
		return 1
	}
	if err := paths.EnsureDirs(); err != nil {
		fmt.Fprintf(stderr, "commando: %v\n", err)
		return 1
	}

	in := bufio.NewReader(os.Stdin)
	ask := func(prompt string) string {
		fmt.Fprint(stdout, prompt)
		line, _ := in.ReadString('\n')
		return strings.TrimSpace(line)
	}

	title := ask("Title: ")
	if title == "" {
		fmt.Fprintln(stderr, "add: title is required")
		return 2
	}
	desc := ask("Description (optional): ")
	tmpl := ask("Command template (use <var> and [optional <var>]): ")
	if tmpl == "" {
		fmt.Fprintln(stderr, "add: template is required")
		return 2
	}
	parsed, err := template.Parse(tmpl)
	if err != nil {
		fmt.Fprintf(stderr, "add: invalid template: %v\n", err)
		return 2
	}
	tags := splitTags(ask("Tags (comma-separated, optional): "))

	var b strings.Builder
	b.WriteString("\n[[cmd]]\n")
	fmt.Fprintf(&b, "title = %s\n", tomlString(title))
	if desc != "" {
		fmt.Fprintf(&b, "desc = %s\n", tomlString(desc))
	}
	fmt.Fprintf(&b, "tmpl = %s\n", tomlString(tmpl))
	if len(tags) > 0 {
		fmt.Fprintf(&b, "tags = [%s]\n", tomlStringList(tags))
	}

	for _, ref := range parsed.Vars() {
		fmt.Fprintf(stdout, "\nVariable <%s>%s:\n", ref.Name, optionalLabel(ref.Optional))
		src := ask("  Candidate command (src, optional): ")
		def := ask("  Default value (optional): ")
		multi := yes(ask("  Allow multiple selections? [y/N]: "))
		remember := ask("  Remember scope [none/global/project/dir] (default none): ")
		if remember == "" {
			remember = "none"
		}

		b.WriteString("\n  [[cmd.var]]\n")
		fmt.Fprintf(&b, "  name = %s\n", tomlString(ref.Name))
		if src != "" {
			fmt.Fprintf(&b, "  src = %s\n", tomlString(src))
		}
		if def != "" {
			fmt.Fprintf(&b, "  default = %s\n", tomlString(def))
		}
		if multi {
			b.WriteString("  multi = true\n")
			b.WriteString("  each = \"\\\"{}\\\"\"\n")
		}
		// Write "none" explicitly. An omitted remember field means global
		// memory, which would contradict the privacy-preserving prompt default.
		fmt.Fprintf(&b, "  remember = %s\n", tomlString(remember))
	}

	dst := filepath.Join(paths.CheatsDir, "user.toml")
	if err := appendCheat(dst, []byte(b.String())); err != nil {
		fmt.Fprintf(stderr, "add: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "\nAdded to %s\n", dst)
	return 0
}

// appendCheat validates the complete user cheat file and replaces it
// atomically. A malformed existing file is left untouched and reported.
func appendCheat(path string, fragment []byte) error {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	combined := make([]byte, 0, len(existing)+len(fragment))
	combined = append(combined, existing...)
	combined = append(combined, fragment...)
	if _, err := config.LoadCheatData(filepath.Base(path), combined); err != nil {
		return fmt.Errorf("entry is invalid: %w", err)
	}
	return persist.WriteFile(path, combined, 0o644)
}

func optionalLabel(opt bool) string {
	if opt {
		return " (optional)"
	}
	return ""
}

func splitTags(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func yes(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "y" || s == "yes"
}

// tomlString renders a Go string as a TOML basic string with proper escaping.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString("\\\"")
		case '\\':
			b.WriteString("\\\\")
		case '\n':
			b.WriteString("\\n")
		case '\t':
			b.WriteString("\\t")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func tomlStringList(items []string) string {
	quoted := make([]string, len(items))
	for i, it := range items {
		quoted[i] = tomlString(it)
	}
	return strings.Join(quoted, ", ")
}
