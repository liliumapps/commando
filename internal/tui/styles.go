package tui

import "github.com/charmbracelet/lipgloss"

// Colour palette (256-colour codes): a blue command preview with distinct
// parameter names, cyan tags, a magenta accent, and a highlighted selection bar.
const (
	colAccent    = "212" // magenta — titles, cursor
	colVar       = "213" // pink — parameter names inside <vars>
	colCmd       = "75"  // blue — command literal text
	colOptText   = "109" // muted blue-grey — optional inner text
	colOptBucket = "214" // amber — optional [ ] brackets
	colTag       = "80"  // cyan — tag column
	colDesc      = "252" // near-white — descriptions
	colDim       = "244" // grey — secondary text
	colDimmer    = "240" // darker grey — hints
	colSelFg     = "231" // white — selected row foreground
	colSelBg     = "54"  // deep purple — selected row background
	colGreen     = "114" // cheat source
	colOrange    = "215" // project source
	colBlue      = "111" // history source
	colErr       = "203" // red — errors
	colCheckOn   = "114" // green — checked multi item
	colValue     = "150" // soft green — resolved parameter values
	colParam     = "213" // pink — parameter names in assignments
	colEquals    = "244" // grey — the "=" between name and value
)

// styles groups the lipgloss styles used across the TUI.
type styles struct {
	counter lipgloss.Style
	prompt  lipgloss.Style
	query   lipgloss.Style
	caret   lipgloss.Style

	// list rows
	rowTag      lipgloss.Style
	rowDesc     lipgloss.Style
	rowDescSel  lipgloss.Style
	cursor      lipgloss.Style
	sourceCheat lipgloss.Style
	sourceProj  lipgloss.Style
	sourceHist  lipgloss.Style

	// candidate preview pane
	previewBox lipgloss.Style
	// full command box (list preview + form stage)
	commandBox lipgloss.Style

	// command highlighting
	cmdLiteral lipgloss.Style
	varStyle   lipgloss.Style
	varBracket lipgloss.Style
	optBracket lipgloss.Style
	optText    lipgloss.Style

	// form
	fieldActive  lipgloss.Style
	paramName    lipgloss.Style // parameter name in an assignment
	paramNameCur lipgloss.Style // parameter name of the active param
	equals       lipgloss.Style // the grey "=" between name and value
	valueText    lipgloss.Style // a resolved value in an assignment
	placeholder  lipgloss.Style // an unfilled value slot
	skipTag      lipgloss.Style
	checkOn      lipgloss.Style
	checkOff     lipgloss.Style

	dim     lipgloss.Style
	errText lipgloss.Style
	help    lipgloss.Style
	helpKey lipgloss.Style
}

func defaultStyles() styles {
	sel := lipgloss.NewStyle().Background(lipgloss.Color(colSelBg))
	return styles{
		counter: lipgloss.NewStyle().Foreground(lipgloss.Color(colDim)),
		prompt:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colAccent)),
		query:   lipgloss.NewStyle().Foreground(lipgloss.Color(colDesc)),
		caret:   lipgloss.NewStyle().Foreground(lipgloss.Color(colAccent)),

		rowTag:     lipgloss.NewStyle().Foreground(lipgloss.Color(colTag)),
		rowDesc:    lipgloss.NewStyle().Foreground(lipgloss.Color(colDesc)),
		rowDescSel: sel.Foreground(lipgloss.Color(colSelFg)).Bold(true),
		cursor:     lipgloss.NewStyle().Foreground(lipgloss.Color(colAccent)).Bold(true),

		sourceCheat: lipgloss.NewStyle().Foreground(lipgloss.Color(colGreen)),
		sourceProj:  lipgloss.NewStyle().Foreground(lipgloss.Color(colOrange)),
		sourceHist:  lipgloss.NewStyle().Foreground(lipgloss.Color(colBlue)),

		previewBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colTag)).
			Padding(0, 1),

		commandBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colAccent)).
			Padding(0, 1),

		cmdLiteral: lipgloss.NewStyle().Foreground(lipgloss.Color(colCmd)),
		varStyle:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colVar)),
		varBracket: lipgloss.NewStyle().Foreground(lipgloss.Color(colDim)),
		optBracket: lipgloss.NewStyle().Foreground(lipgloss.Color(colOptBucket)),
		optText:    lipgloss.NewStyle().Foreground(lipgloss.Color(colOptText)),

		fieldActive:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colAccent)),
		paramName:    lipgloss.NewStyle().Foreground(lipgloss.Color(colParam)),
		paramNameCur: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colAccent)),
		equals:       lipgloss.NewStyle().Foreground(lipgloss.Color(colEquals)),
		valueText:    lipgloss.NewStyle().Foreground(lipgloss.Color(colValue)),
		placeholder:  lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color(colDimmer)),
		skipTag:      lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color(colDimmer)),
		checkOn:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colCheckOn)),
		checkOff:     lipgloss.NewStyle().Foreground(lipgloss.Color(colDimmer)),

		dim:     lipgloss.NewStyle().Foreground(lipgloss.Color(colDim)),
		errText: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colErr)),
		help:    lipgloss.NewStyle().Foreground(lipgloss.Color(colDimmer)),
		helpKey: lipgloss.NewStyle().Foreground(lipgloss.Color(colTag)),
	}
}

// sourceStyle returns the colour style for a source label.
func (s styles) sourceStyle(label string) lipgloss.Style {
	switch label {
	case "cheat":
		return s.sourceCheat
	case "project":
		return s.sourceProj
	case "history":
		return s.sourceHist
	case "bookmark":
		return s.paramName
	default:
		return s.dim
	}
}
