package app

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/rank"
	"github.com/liliumapps/commando/internal/resolve"
)

// wsRun matches a run of whitespace, the default column delimiter.
var wsRun = regexp.MustCompile(`\s+`)

// ansiRE matches terminal escape sequences: CSI (colours, cursor moves), OSC
// (titles, hyperlinks; BEL- or ST-terminated), and two-byte escapes.
var ansiRE = regexp.MustCompile(`\x1b(?:\[[\x30-\x3f]*[\x20-\x2f]*[\x40-\x7e]|\][^\x07\x1b]*(?:\x07|\x1b\\)|[\x40-\x5f])`)

// stripANSI removes terminal escape sequences from s.
func stripANSI(s string) string {
	if !strings.Contains(s, "\x1b") {
		return s
	}
	return ansiRE.ReplaceAllString(s, "")
}

// extractCandidates turns the raw stdout of a variable's Src command into a
// candidate list. It splits the output into lines, drops the first HeaderLines
// (so a header row is removed even if it would become a candidate), then
// projects each remaining line into a Display/Value pair via the spec's
// extraction fields. See projectLines for the per-line logic.
func extractCandidates(raw string, spec entry.VarSpec) []rank.Candidate {
	// Tools that colourise even when piped (or with --color=always) would
	// otherwise leak escape codes into both Display and the inserted Value.
	raw = stripANSI(raw)
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSuffix(line, "\r")
	}

	// Drop a trailing empty line produced by a final newline so it does not
	// count against HeaderLines math or appear as a blank candidate.
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}

	if spec.HeaderLines > 0 {
		if spec.HeaderLines >= len(lines) {
			return nil
		}
		lines = lines[spec.HeaderLines:]
	}

	return projectLines(lines, spec)
}

// projector renders each source line into a candidate's Display and Value,
// resolving the Columns names or Match groups the Label/Value templates
// reference. It is built once per spec so the Match/Delimiter regexes compile a
// single time regardless of how many lines are processed.
type projector struct {
	match      *regexp.Regexp // non-nil in Match mode
	groupNames []string       // Match mode: re.SubexpNames()
	columns    []string       // Columns mode: field names by position
	splitter   *regexp.Regexp // Columns mode: line splitter
	label      string         // Label template ("" => raw line)
	value      string         // Value template ("" => raw line)
}

// newProjector compiles a projector from a spec. A malformed Delimiter regex
// falls back to whitespace splitting so a bad cheat degrades gracefully; a
// malformed Match regex is validated at config load, so here it simply disables
// Match mode if compilation somehow fails.
func newProjector(spec entry.VarSpec) projector {
	p := projector{label: spec.Label, value: spec.Value}
	if spec.Match != "" {
		if re, err := regexp.Compile(spec.Match); err == nil {
			p.match = re
			p.groupNames = re.SubexpNames()
		}
		return p
	}
	if len(spec.Columns) > 0 {
		p.columns = spec.Columns
		p.splitter = wsRun
		if spec.Delimiter != "" {
			if re, err := regexp.Compile(spec.Delimiter); err == nil {
				p.splitter = re
			}
		}
	}
	return p
}

// project renders one source line into a candidate. ok is false when the line
// should be dropped: a Match-mode line that does not match, or a line whose
// rendered value is empty.
func (p projector) project(line string) (rank.Candidate, bool) {
	// Build the name -> text map the Label/Value templates resolve against.
	// <0> is always the whole line/match; positional <1>,<2>,... and any named
	// columns/groups are added by mode.
	var fields map[string]string

	switch {
	case p.match != nil:
		sub := p.match.FindStringSubmatch(line)
		if sub == nil {
			return rank.Candidate{}, false // no match => drop
		}
		fields = make(map[string]string, len(sub)*2)
		fields["0"] = sub[0]
		for i := 1; i < len(sub); i++ {
			fields[strconv.Itoa(i)] = sub[i]
			if name := p.groupNames[i]; name != "" {
				fields[name] = sub[i]
			}
		}
	case p.splitter != nil:
		cols := p.splitter.Split(strings.TrimSpace(line), -1)
		fields = make(map[string]string, len(cols)*2+1)
		fields["0"] = line
		for i, col := range cols {
			fields[strconv.Itoa(i+1)] = col
			if i < len(p.columns) && p.columns[i] != "" {
				fields[p.columns[i]] = col
			}
		}
	default:
		// No extraction: Display and Value are both the raw line.
		fields = map[string]string{"0": line}
	}

	display := line
	if p.label != "" {
		display = resolve.SubstituteRefs(p.label, fields)
	}
	value := line
	if p.value != "" {
		value = resolve.SubstituteRefs(p.value, fields)
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return rank.Candidate{}, false
	}
	display = strings.TrimSpace(display)
	if display == "" {
		display = value
	}
	return rank.Candidate{Display: display, Value: value}, true
}

// projectLines applies the spec's projector to every line, dropping lines that
// do not match or that render an empty value.
func projectLines(lines []string, spec entry.VarSpec) []rank.Candidate {
	p := newProjector(spec)
	out := make([]rank.Candidate, 0, len(lines))
	for _, line := range lines {
		if c, ok := p.project(line); ok {
			out = append(out, c)
		}
	}
	return out
}
