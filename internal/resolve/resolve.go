// Package resolve bridges an entry.Entry and the template engine. It produces
// an ordered plan of the variables a user must fill in (respecting optionality
// from the template and the VarSpecs), and assembles the final command from the
// values collected. The TUI drives resolution field by field; this package
// keeps the logic testable and independent of any UI.
package resolve

import (
	"fmt"

	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/template"
)

// Field is one variable the user must resolve, combining the template's view of
// the variable (name, optionality) with its VarSpec (candidates, formatting,
// memory scope).
type Field struct {
	Name     string
	Optional bool
	Spec     entry.VarSpec
}

// Plan is the ordered set of fields to resolve for an entry, plus the parsed
// template used to assemble the result.
type Plan struct {
	Entry    entry.Entry
	Template *template.Template
	Fields   []Field
}

// NewPlan parses the entry's template and builds the resolution plan. Variables
// referenced in the template but lacking a VarSpec get a default (empty) spec;
// specs that reference no template variable are ignored. It returns an error if
// the template is malformed.
func NewPlan(e entry.Entry) (*Plan, error) {
	tpl, err := template.Parse(e.Template)
	if err != nil {
		return nil, err
	}

	spec := func(name string) entry.VarSpec {
		if s, ok := e.Var(name); ok {
			return s
		}
		return entry.VarSpec{Name: name}
	}

	// Seed with the template's variables (in appearance order), recording their
	// optionality. Then expand transitively: any variable referenced by a
	// field's Src command becomes a field too, even if it never appears in the
	// template (e.g. a "namespace" used only to compute another field's
	// candidates). Such helper variables are required, since something depends
	// on them.
	optional := make(map[string]bool)
	discovered := make(map[string]bool)
	var order []string // discovery order (template vars first, then helpers)

	for _, ref := range tpl.Vars() {
		if !discovered[ref.Name] {
			discovered[ref.Name] = true
			order = append(order, ref.Name)
			optional[ref.Name] = ref.Optional
		}
	}

	deps := make(map[string][]string)
	for i := 0; i < len(order); i++ {
		name := order[i]
		refs := SrcRefs(spec(name).Src)
		deps[name] = refs
		for _, r := range refs {
			if !discovered[r] {
				discovered[r] = true
				order = append(order, r)
				optional[r] = false // helper variables are required
			}
		}
	}

	// Topologically sort so a field is resolved after every field its Src
	// references (dependencies first). A cycle is broken gracefully, preserving
	// discovery order for the entries involved.
	sorted := topoSort(order, deps)

	fields := make([]Field, len(sorted))
	for i, name := range sorted {
		fields[i] = Field{Name: name, Optional: optional[name], Spec: spec(name)}
	}
	return &Plan{Entry: e, Template: tpl, Fields: fields}, nil
}

// topoSort returns names ordered so that each name appears after all names it
// depends on. It uses a stable post-order DFS over the given discovery order;
// dependency edges point from a name to the names its Src references. Cycles are
// tolerated (the back-edge is ignored), so the function always terminates.
func topoSort(order []string, deps map[string][]string) []string {
	const (
		unvisited = 0
		active    = 1
		done      = 2
	)
	state := make(map[string]int, len(order))
	sorted := make([]string, 0, len(order))

	var visit func(string)
	visit = func(name string) {
		switch state[name] {
		case done, active:
			return // already placed, or a cycle back-edge — skip
		}
		state[name] = active
		for _, d := range deps[name] {
			visit(d)
		}
		state[name] = done
		sorted = append(sorted, name)
	}
	for _, name := range order {
		visit(name)
	}
	return sorted
}

// HasVars reports whether the entry requires any user input at all. Entries with
// no variables (e.g. plain project commands) can be emitted immediately.
func (p *Plan) HasVars() bool {
	return len(p.Fields) > 0
}

// Assemble renders the final command from collected values. A field absent from
// values is treated as skipped; skipping a required field is an error.
func (p *Plan) Assemble(values map[string]string) (string, error) {
	return p.Template.Assemble(values)
}

// Preview renders a best-effort command string for display while the user is
// still filling fields. Unlike Assemble it never errors on missing required
// values: a not-yet-provided required variable is shown as a bracketed
// placeholder so the user can see where it will land.
func (p *Plan) Preview(values map[string]string) string {
	// Build an augmented value map so that required-but-empty variables render
	// as visible placeholders instead of being treated as skipped.
	aug := make(map[string]string, len(values))
	for k, v := range values {
		aug[k] = v
	}
	for _, f := range p.Fields {
		if _, ok := aug[f.Name]; ok {
			continue
		}
		if !f.Optional {
			aug[f.Name] = fmt.Sprintf("<%s>", f.Name)
		}
	}
	out, err := p.Template.Assemble(aug)
	if err != nil {
		// Should not happen because every required var now has a value, but
		// fall back to the raw template rather than panicking in the UI.
		return p.Entry.Template
	}
	return out
}
