package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liliumapps/commando/internal/entry"
)

func TestFlattenOneLine(t *testing.T) {
	cases := []struct{ in, want string }{
		{"git status", "git status"},
		// A trailing backslash is literal command text; only the newline+spacing
		// is collapsed to a single space.
		{"echo one \\\n&& echo two", "echo one \\ && echo two"},
		{"a\tb\tc", "a b c"},
		{"line1\nline2\nline3", "line1 line2 line3"},
		{"  leading and  \n  trailing  ", "leading and trailing"},
		// No newline/tab present: internal spacing is left untouched (fast path).
		{"multiple   spaces", "multiple   spaces"},
		// With a newline present, whitespace runs collapse.
		{"a  b\nc", "a b c"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := flattenOneLine(tc.in); got != tc.want {
			t.Errorf("flattenOneLine(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMultilineHistoryRendersSingleRow(t *testing.T) {
	// A multi-line history command must not spill across terminal lines: each
	// list row is exactly one line.
	entries := []entry.Entry{
		{Desc: "clip", Template: "clip", Source: entry.SourceCheat},
		{
			Desc:     "git mv src/Button.tsx src/button.tsx && \\\n# Rename to lowercase",
			Template: "cp a.tsx b.tsx && \\\n# copy",
			Source:   entry.SourceHistory,
		},
	}
	m := New(entries, newFakeBackend())
	m = send(m, tea.WindowSizeMsg{Width: 120, Height: 20})

	// Render one result row for the history entry and confirm the embedded
	// newline was flattened instead of adding terminal rows.
	row := m.renderEntryRow(entries[1], false)
	if got := strings.Count(stripANSI(row), "\n"); got != 0 {
		t.Errorf("history row should be exactly one line, got %d newlines:\n%q", got, stripANSI(row))
	}
	// And the whole view's line count still equals the height (no overflow).
	if got := len(lines(m.View())); got != 20 {
		t.Errorf("view has %d lines, want 20 (multi-line entry must not add rows)", got)
	}
}
