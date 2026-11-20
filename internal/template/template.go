// Package template parses and assembles command templates. A template is a
// command string that may contain:
//
//   - <name>            a variable placeholder
//   - [ ... <name> ... ] an optional segment: if the variable(s) inside it are
//     not provided, the entire bracketed fragment (including its literal text)
//     is dropped from the assembled command
//   - \<  \>  \[  \]  \\ escaped literals for the metacharacters above
//
// Optionality deliberately attaches to the *fragment*, not the variable, which
// avoids the dangling-flag problem (e.g. a leftover "--author=" when no author
// is chosen).
//
// The package is intentionally free of dependencies on the rest of the code
// base so it can be reasoned about and tested in isolation.
package template

import (
	"errors"
	"fmt"
	"strings"
)

// NodeKind distinguishes the three kinds of parsed node.
type NodeKind int

const (
	// KindText is a run of literal characters.
	KindText NodeKind = iota
	// KindVar is a <name> placeholder.
	KindVar
	// KindOpt is a [ ... ] optional segment containing child nodes.
	KindOpt
)

// Node is a single element of a parsed template.
type Node struct {
	Kind     NodeKind
	Text     string // set for KindText
	Var      string // set for KindVar
	Children []Node // set for KindOpt
}

// Template is a parsed command template.
type Template struct {
	Raw   string
	Nodes []Node
}

// VarRef describes a variable referenced by a template.
type VarRef struct {
	// Name is the placeholder name.
	Name string
	// Optional is true when every occurrence of the variable is inside an
	// optional segment. If the variable appears at least once outside any
	// optional segment it is required and Optional is false.
	Optional bool
}

// Parse converts a raw template string into a Template. It returns an error for
// a malformed "<name" placeholder (so typos are reported, not emitted).
//
// Shell syntax that merely looks like template syntax is kept verbatim:
//   - "<" not directly followed by a name character ("sort < f", "<<EOF",
//     "<(cmd)", "2<&1") is a literal;
//   - "[ ... ]" that contains no variable ("grep '[0-9]+'"), an unclosed "[",
//     and "[[ ... ]]" are literals, brackets included;
//   - a backslash escapes only < > [ ] and itself; any other backslash
//     ("\t", "\*.go", a line continuation) is kept as written.
func Parse(raw string) (*Template, error) {
	rs := []rune(raw)
	nodes, next, err := parseSeq(rs, 0, false)
	if err != nil {
		return nil, err
	}
	if next != len(rs) {
		return nil, fmt.Errorf("template: unexpected trailing input at %d", next)
	}
	return &Template{Raw: raw, Nodes: nodes}, nil
}

// errUnterminatedGroup signals a '[' with no matching ']', which the caller
// keeps as a literal '['.
type errUnterminatedGroup struct{}

func (e errUnterminatedGroup) Error() string { return "template: unterminated '['" }

func isNameRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.'
}

// scanVar reads a placeholder starting at rs[i] == '<'. ok is false when the
// '<' is a shell literal: it is not directly followed by a name character
// ("sort < f", "<<EOF", "<(cmd)", "2<&1"). err is set when it starts like a
// placeholder ("<name") but is malformed, so typos are reported, not emitted.
func scanVar(rs []rune, i int) (name string, next int, ok bool, err error) {
	j := i + 1
	if j < len(rs) && rs[j] == '>' {
		return "", 0, false, fmt.Errorf("template: empty variable name at %d", i)
	}
	if j >= len(rs) || !isNameRune(rs[j]) {
		return "", 0, false, nil
	}
	k := j
	for k < len(rs) && isNameRune(rs[k]) {
		k++
	}
	if k >= len(rs) {
		return "", 0, false, fmt.Errorf("template: unterminated '<' at %d (write a literal as \\<)", i)
	}
	if rs[k] != '>' {
		return "", 0, false, fmt.Errorf("template: invalid character %q in variable at %d (names use letters, digits, _ - .; write a literal as \\<)", rs[k], i)
	}
	return string(rs[j:k]), k + 1, true, nil
}

// parseSeq parses a sequence of nodes starting at index i. When inGroup is
// true, parsing stops after the matching ']' and returns the index just past
// it; otherwise it consumes to the end of input.
func parseSeq(rs []rune, i int, inGroup bool) (nodes []Node, next int, err error) {
	var text []rune
	flush := func() {
		if len(text) > 0 {
			nodes = append(nodes, Node{Kind: KindText, Text: string(text)})
			text = nil
		}
	}

	for i < len(rs) {
		c := rs[i]
		switch c {
		case '\\':
			if i+1 < len(rs) {
				switch rs[i+1] {
				case '<', '>', '[', ']', '\\':
					text = append(text, rs[i+1])
					i += 2
					continue
				}
			}
			text = append(text, c)
			i++
		case '<':
			if i > 0 && rs[i-1] == '<' {
				// "<<EOF", "<<<word": heredoc and here-string operators.
				text = append(text, c)
				i++
				continue
			}
			name, ni, ok, verr := scanVar(rs, i)
			if verr != nil {
				return nil, 0, verr
			}
			if !ok {
				text = append(text, c)
				i++
				continue
			}
			flush()
			nodes = append(nodes, Node{Kind: KindVar, Var: name})
			i = ni
		case '[':
			if i+1 < len(rs) && rs[i+1] == '[' {
				// "[[ ... ]]" is a shell test, never an optional segment.
				text = append(text, '[', '[')
				i += 2
				continue
			}
			child, ni, gerr := parseSeq(rs, i+1, true)
			var ug errUnterminatedGroup
			if gerr != nil && !errors.As(gerr, &ug) {
				return nil, 0, gerr
			}
			if gerr != nil {
				// No closing ']': a literal '[' (e.g. "grep '['").
				text = append(text, c)
				i++
				continue
			}
			if !containsVar(child) {
				// "[...]" without a variable is shell syntax, kept verbatim.
				text = append(text, []rune(string(rs[i:ni]))...)
				i = ni
				continue
			}
			flush()
			nodes = append(nodes, Node{Kind: KindOpt, Children: child})
			i = ni
		case ']':
			if inGroup {
				flush()
				return nodes, i + 1, nil
			}
			text = append(text, c)
			i++
		default:
			text = append(text, c)
			i++
		}
	}

	if inGroup {
		return nil, 0, errUnterminatedGroup{}
	}
	flush()
	return nodes, i, nil
}

func containsVar(nodes []Node) bool {
	for _, n := range nodes {
		if n.Kind == KindVar || (n.Kind == KindOpt && containsVar(n.Children)) {
			return true
		}
	}
	return false
}

// Vars returns the variables referenced by the template in order of first
// appearance, each annotated with whether it is optional. A variable that
// appears both inside and outside an optional segment is reported as required.
func (t *Template) Vars() []VarRef {
	order := make([]string, 0)
	optional := make(map[string]bool)
	seen := make(map[string]bool)

	var walk func(nodes []Node, inOpt bool)
	walk = func(nodes []Node, inOpt bool) {
		for _, n := range nodes {
			switch n.Kind {
			case KindVar:
				if !seen[n.Var] {
					seen[n.Var] = true
					order = append(order, n.Var)
					optional[n.Var] = inOpt
				} else if !inOpt {
					// A required occurrence overrides a prior optional one.
					optional[n.Var] = false
				}
			case KindOpt:
				walk(n.Children, true)
			}
		}
	}
	walk(t.Nodes, false)

	refs := make([]VarRef, len(order))
	for i, name := range order {
		refs[i] = VarRef{Name: name, Optional: optional[name]}
	}
	return refs
}

// Assemble renders the final command. values maps a variable name to its
// substituted string; a variable absent from the map is treated as "not
// provided" (skipped). Optional segments whose direct variables are all
// provided are emitted; segments with any missing direct variable are dropped
// entirely. It is an error for a required variable to be missing.
//
// Whitespace introduced by dropped segments is collapsed so the result never
// contains doubled spaces from concatenation, while whitespace *inside*
// substituted values is preserved verbatim (so quoted multi-word arguments are
// safe).
func (t *Template) Assemble(values map[string]string) (string, error) {
	for _, ref := range t.Vars() {
		if !ref.Optional {
			if _, ok := values[ref.Name]; !ok {
				return "", fmt.Errorf("template: required variable %q not provided", ref.Name)
			}
		}
	}
	var s spacer
	renderNodes(&s, t.Nodes, values)
	return strings.TrimSpace(s.String()), nil
}

// renderNodes appends the rendering of nodes to s.
func renderNodes(s *spacer, nodes []Node, values map[string]string) {
	for _, n := range nodes {
		switch n.Kind {
		case KindText:
			s.literal(n.Text)
		case KindVar:
			if v, ok := values[n.Var]; ok {
				s.value(v)
			}
		case KindOpt:
			if segmentIncluded(n, values) {
				renderNodes(s, n.Children, values)
			} else {
				s.dropped = true
			}
		}
	}
}

// segmentIncluded reports whether an optional segment should be emitted: it is
// included only when every one of its *direct* variable children has a provided
// value. Nested optional segments are evaluated independently during rendering
// and do not affect the inclusion of their parent.
func segmentIncluded(opt Node, values map[string]string) bool {
	for _, c := range opt.Children {
		if c.Kind == KindVar {
			if _, ok := values[c.Var]; !ok {
				return false
			}
		}
	}
	return true
}

// FormatMulti formats a list of selected values into a single substitution
// string. Each item is passed through the each format (where "{}" is replaced
// by the item; an empty each leaves the item unchanged) and the results are
// joined with sep (defaulting to a single space when empty).
func FormatMulti(items []string, sep, each string) string {
	if sep == "" {
		sep = " "
	}
	out := make([]string, len(items))
	for i, it := range items {
		if each == "" {
			out[i] = it
		} else {
			out[i] = strings.ReplaceAll(each, "{}", it)
		}
	}
	return strings.Join(out, sep)
}

// spacer accumulates rendered output. Literal template text and values are
// appended verbatim (so quoting, tabs, and newlines inside the template are
// preserved); the only whitespace it removes is the doubled gap a dropped
// optional segment leaves behind.
type spacer struct {
	buf     []byte
	dropped bool // an optional segment was just dropped
}

func isSpace(b byte) bool { return b == ' ' || b == '\t' }

// literal appends literal template text. Right after a dropped segment, leading
// spaces are trimmed if the output already ends in a space (or is empty), so
// "cmd [<a>] tail" renders as "cmd tail".
func (s *spacer) literal(txt string) {
	if s.dropped {
		if len(s.buf) == 0 || isSpace(s.buf[len(s.buf)-1]) {
			txt = strings.TrimLeft(txt, " \t")
		}
		if txt != "" {
			s.dropped = false
		}
	}
	s.buf = append(s.buf, txt...)
}

// value appends a substituted value verbatim, preserving any internal spacing.
func (s *spacer) value(v string) {
	s.dropped = false
	s.buf = append(s.buf, v...)
}

func (s *spacer) String() string {
	return string(s.buf)
}
