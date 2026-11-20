package app

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/liliumapps/commando/internal/template"
)

// fillMatcher recognises shell-history lines that are instances of a cheat
// template and extracts the variable values they used.
//
// Literal template text matches exactly, except that any run of whitespace
// matches any run of whitespace. Each <var> matches one shell word: a single-
// or double-quoted string, or a run of non-space characters. An optional
// [ ... ] segment may be absent. A variable that appears more than once must
// capture the same value everywhere.
type fillMatcher struct {
	re *regexp.Regexp
	// groups maps capture group index (1-based) to the variable name; "" for
	// a group that is matched but not captured (multi or never-remembered).
	groups []string
	// anchor is the longest required literal word, checked with
	// strings.Contains before running the regex on a line.
	anchor string
}

// wordPattern matches one shell word.
const wordPattern = `"(?:[^"\\]|\\.)*"|'[^']*'|\S+`

// newFillMatcher compiles a matcher for tmpl. capture reports whether a
// variable's value should be extracted; multi reports variables whose value
// is a list of words. It returns nil when the template cannot be matched
// safely: it fails to parse, has no variables, or has no required literal
// word (a bare "<cmd>" would match every line).
func newFillMatcher(tmpl string, capture, multi func(name string) bool) *fillMatcher {
	t, err := template.Parse(tmpl)
	if err != nil || len(t.Vars()) == 0 {
		return nil
	}
	fm := &fillMatcher{}
	var b strings.Builder
	b.WriteString(`^\s*`)
	fm.writeNodes(&b, t.Nodes, false, capture, multi)
	b.WriteString(`\s*$`)
	if fm.anchor == "" {
		return nil
	}
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil
	}
	fm.re = re
	return fm
}

func (fm *fillMatcher) writeNodes(b *strings.Builder, nodes []template.Node, inOpt bool, capture, multi func(string) bool) {
	for i, n := range nodes {
		switch n.Kind {
		case template.KindText:
			text := n.Text
			// Whitespace before an optional segment belongs to the segment,
			// so "cmd [-n <ns>] tail" also matches "cmd tail".
			if i+1 < len(nodes) && nodes[i+1].Kind == template.KindOpt {
				text = strings.TrimRightFunc(text, unicode.IsSpace)
			}
			fm.writeText(b, text, inOpt)
		case template.KindVar:
			word := `(?:` + wordPattern + `)`
			switch {
			case multi(n.Var):
				b.WriteString(word + `(?:\s+` + word + `)*`)
			case capture(n.Var):
				b.WriteString(`(` + wordPattern + `)`)
				fm.groups = append(fm.groups, n.Var)
			default:
				b.WriteString(word)
			}
		case template.KindOpt:
			b.WriteString(`(?:`)
			if i > 0 && nodes[i-1].Kind == template.KindText &&
				strings.TrimRightFunc(nodes[i-1].Text, unicode.IsSpace) != nodes[i-1].Text {
				b.WriteString(`\s+`)
			}
			fm.writeNodes(b, n.Children, true, capture, multi)
			b.WriteString(`)?`)
		}
	}
}

// writeText writes literal text, turning whitespace runs into \s+.
func (fm *fillMatcher) writeText(b *strings.Builder, text string, inOpt bool) {
	fields := strings.FieldsFunc(text, unicode.IsSpace)
	if len(fields) == 0 {
		if text != "" {
			b.WriteString(`\s+`)
		}
		return
	}
	if strings.IndexFunc(text, unicode.IsSpace) == 0 {
		b.WriteString(`\s+`)
	}
	for i, f := range fields {
		if i > 0 {
			b.WriteString(`\s+`)
		}
		b.WriteString(regexp.QuoteMeta(f))
		if !inOpt && len(f) > len(fm.anchor) {
			fm.anchor = f
		}
	}
	if strings.TrimRightFunc(text, unicode.IsSpace) != text {
		b.WriteString(`\s+`)
	}
}

// match returns the captured values of line, or ok false when the line is
// not an instance of the template (including when a repeated variable
// captured two different values).
func (fm *fillMatcher) match(line string) (map[string]string, bool) {
	if !strings.Contains(line, fm.anchor) {
		return nil, false
	}
	sub := fm.re.FindStringSubmatchIndex(line)
	if sub == nil {
		return nil, false
	}
	vals := make(map[string]string)
	for gi, name := range fm.groups {
		start, end := sub[2*(gi+1)], sub[2*(gi+1)+1]
		if start < 0 {
			continue // inside an absent optional segment
		}
		v := unquoteWord(line[start:end])
		if prev, seen := vals[name]; seen && prev != v {
			return nil, false
		}
		vals[name] = v
	}
	return vals, true
}

// unquoteWord strips the quotes from a quoted word when the content is a
// plain shell word (so 'payments' and payments are the same value). A value
// that needs its quotes, such as "fix login bug", keeps them, since a form
// value is inserted verbatim and would otherwise split into several words.
func unquoteWord(w string) string {
	if len(w) < 2 {
		return w
	}
	q := w[0]
	if (q != '"' && q != '\'') || w[len(w)-1] != q {
		return w
	}
	inner := w[1 : len(w)-1]
	if inner == "" {
		return w
	}
	for _, r := range inner {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_@%+=:,./-", r)) {
			return w
		}
	}
	return inner
}
