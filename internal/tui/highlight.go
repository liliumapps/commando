package tui

import (
	"strings"

	"github.com/liliumapps/commando/internal/template"
)

// highlightTemplate renders a command template with syntax colouring: variable
// names inside <vars> are tinted separately from their brackets, [optional]
// segments have their brackets accented and their inner text dimmed, and
// literal command text uses the base command colour. On a
// parse error the raw string is returned in the literal style so the UI never
// breaks.
func (s styles) highlightTemplate(raw string) string {
	tpl, err := template.Parse(raw)
	if err != nil {
		return s.cmdLiteral.Render(raw)
	}
	var b strings.Builder
	s.renderHighlightNodes(&b, tpl.Nodes, false)
	return b.String()
}

func (s styles) renderHighlightNodes(b *strings.Builder, nodes []template.Node, inOpt bool) {
	for _, n := range nodes {
		switch n.Kind {
		case template.KindText:
			if inOpt {
				b.WriteString(s.optText.Render(n.Text))
			} else {
				b.WriteString(s.cmdLiteral.Render(n.Text))
			}
		case template.KindVar:
			b.WriteString(s.highlightVar(n.Var))
		case template.KindOpt:
			b.WriteString(s.optBracket.Render("["))
			s.renderHighlightNodes(b, n.Children, true)
			b.WriteString(s.optBracket.Render("]"))
		}
	}
}

func (s styles) highlightVar(name string) string {
	return s.varBracket.Render("<") + s.varStyle.Render(name) + s.varBracket.Render(">")
}
