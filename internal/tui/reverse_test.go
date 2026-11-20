package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liliumapps/commando/internal/entry"
)

func revEntries(descs ...string) []entry.Entry {
	var es []entry.Entry
	for _, d := range descs {
		es = append(es, entry.Entry{Desc: d, Template: d, Source: entry.SourceCheat})
	}
	return es
}

// lineIndexContaining returns the first rendered line index containing sub.
func lineIndexContaining(ls []string, sub string) int {
	for i, ln := range ls {
		if strings.Contains(ln, sub) {
			return i
		}
	}
	return -1
}

func TestListReversedBestNearBottom(t *testing.T) {
	m := New(revEntries("AAA", "BBB", "CCC"), newFakeBackend())
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 16})
	ls := lines(m.View())

	// Restrict to the list region: above the counter/input/footer, so the
	// footer's copy of the selected command is not counted.
	counter := lineIndexContaining(ls, "3/3")
	region := ls[:counter]

	iA := lineIndexContaining(region, "AAA")
	iB := lineIndexContaining(region, "BBB")
	iC := lineIndexContaining(region, "CCC")
	if iA < 0 || iB < 0 || iC < 0 {
		t.Fatalf("missing rows in list region: A=%d B=%d C=%d\n%s", iA, iB, iC, strings.Join(region, "\n"))
	}
	// Reversed: the index-0 entry (AAA) is lowest on screen (largest line index).
	if !(iA > iB && iB > iC) {
		t.Errorf("expected reversed order (AAA below BBB below CCC); got A=%d B=%d C=%d", iA, iB, iC)
	}
	// Rows are single-line; the best match is the bottom-most row.
	if iA != len(region)-1 {
		t.Errorf("best match should be the bottom-most list row (%d), got %d", len(region)-1, iA)
	}
}

func TestListArrowsFlipped(t *testing.T) {
	m := New(revEntries("AAA", "BBB", "CCC"), newFakeBackend())
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 16})
	// Cursor starts at index 0 (best, bottom).
	if m.listCursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", m.listCursor)
	}
	// Up moves visually up the screen => higher index.
	m = send(m, key(tea.KeyUp))
	if m.listCursor != 1 {
		t.Errorf("after Up, cursor = %d, want 1", m.listCursor)
	}
	// Down moves back toward the input => lower index.
	m = send(m, key(tea.KeyDown))
	if m.listCursor != 0 {
		t.Errorf("after Down, cursor = %d, want 0", m.listCursor)
	}
	// Down at index 0 stays (can't go below the best match).
	m = send(m, key(tea.KeyDown))
	if m.listCursor != 0 {
		t.Errorf("Down at bottom should clamp at 0, got %d", m.listCursor)
	}
}

func TestFormCandidatesReversedAndArrowsFlipped(t *testing.T) {
	be := newFakeBackend()
	be.data["file"] = fd("aaa", "bbb", "ccc")
	m := New(sampleEntries(), be)
	m = send(m, tea.WindowSizeMsg{Width: 88, Height: 24})
	m = send(m, key(tea.KeyEnter)) // form, author
	m = send(m, key(tea.KeyCtrlN)) // count
	m = send(m, key(tea.KeyCtrlN)) // file (has candidates)
	if m.form.cur().field.Name != "file" {
		t.Fatalf("expected file param, got %q", m.form.cur().field.Name)
	}

	// Cursor starts at 0.
	if m.form.cur().cursor != 0 {
		t.Fatalf("candidate cursor should start at 0, got %d", m.form.cur().cursor)
	}
	// Up moves to a higher index (reversed layout).
	m = send(m, key(tea.KeyUp))
	if m.form.cur().cursor != 1 {
		t.Errorf("after Up, candidate cursor = %d, want 1", m.form.cur().cursor)
	}
	// Down moves back down toward index 0.
	m = send(m, key(tea.KeyDown))
	if m.form.cur().cursor != 0 {
		t.Errorf("after Down, candidate cursor = %d, want 0", m.form.cur().cursor)
	}

	// The highlighted candidate (index 0 = aaa) is the bottom-most candidate row,
	// just above the input line.
	ls := lines(m.View())
	iInput := lineIndexContaining(ls, "file (multi)")
	iaaa := lineIndexContaining(ls, "aaa")
	ibbb := lineIndexContaining(ls, "bbb")
	if !(iaaa < iInput && iaaa > ibbb) {
		t.Errorf("expected reversed candidates (aaa just above input, bbb above aaa); aaa=%d bbb=%d input=%d", iaaa, ibbb, iInput)
	}
}

func TestCursorStartsAtFirstItem(t *testing.T) {
	m := New(revEntries("AAA", "BBB"), newFakeBackend())
	if m.listCursor != 0 {
		t.Errorf("cursor should start at the first item (0), got %d", m.listCursor)
	}
}

func TestCursorMaintainedAndClampedOnFilter(t *testing.T) {
	entries := revEntries("apple", "apricot", "avocado", "banana", "cherry")
	m := New(entries, newFakeBackend())
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 20})

	// Move the cursor up a few positions.
	m = send(m, key(tea.KeyUp))
	m = send(m, key(tea.KeyUp))
	m = send(m, key(tea.KeyUp)) // cursor at 3
	if m.listCursor != 3 {
		t.Fatalf("cursor = %d, want 3", m.listCursor)
	}

	// A query change resets the cursor to the best match (index 0), so it is
	// always in range.
	m = send(m, runes("a"))
	if m.listCursor > len(m.filtered)-1 {
		t.Errorf("cursor %d out of range after filter (len %d)", m.listCursor, len(m.filtered))
	}
	prev := m.listCursor

	// Narrow further so fewer items remain than the cursor position; it must
	// clamp to the last valid index, not go out of range.
	m = send(m, runes("pp")) // "app" -> only "apple"
	if len(m.filtered) == 0 {
		t.Fatal("expected at least one match for 'app'")
	}
	if m.listCursor != len(m.filtered)-1 {
		t.Errorf("cursor = %d, want clamped to %d", m.listCursor, len(m.filtered)-1)
	}
	_ = prev
}
