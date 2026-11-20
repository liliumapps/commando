package state

import "sort"

const maxUsageEntries = 5000

// usageRow tracks how often and how recently an entry has been launched.
type usageRow struct {
	Freq   int
	LastTS int64
}

type usageMutation struct {
	EntryID string
	LastTS  int64
}

// NoteUse records that the entry with the given ID was launched: it bumps the
// launch count and last-used time, then persists. This is what feeds entry
// ranking in the list.
func (s *State) NoteUse(entryID string) error {
	if entryID == "" {
		return nil
	}
	s.mu.Lock()
	mutation := usageMutation{EntryID: entryID, LastTS: s.now().Unix()}
	applyUsageMutation(s.usage, mutation)
	s.pendingUsage = append(s.pendingUsage, mutation)
	deferred := s.batchDepth > 0
	s.mu.Unlock()
	if deferred {
		return nil
	}
	return s.flushUsage()
}

// EntryUsage returns how recently (unix ts, 0 if never) and how often an entry
// has been launched. It feeds the ranker's recency-first ordering.
func (s *State) EntryUsage(entryID string) (lastUsed int64, count int) {
	if entryID == "" {
		return 0, 0
	}
	s.mu.RLock()
	u := s.usage[entryID]
	s.mu.RUnlock()
	return u.LastTS, u.Freq
}

// --- on-disk representation ---
//
// usage.toml is a flat array of tables, one per launched entry:
//
//   [[entry]]
//   id = "<entry id>"
//   freq = 12
//   last_used = 1700000000

type usageFile struct {
	Entries []usageEntryRow `toml:"entry"`
}

type usageEntryRow struct {
	ID     string `toml:"id"`
	Freq   int    `toml:"freq"`
	LastTS int64  `toml:"last_used"`
}

func usageMapToFile(usage map[string]usageRow) usageFile {
	var uf usageFile
	for id, u := range usage {
		uf.Entries = append(uf.Entries, usageEntryRow{ID: id, Freq: u.Freq, LastTS: u.LastTS})
	}
	sort.Slice(uf.Entries, func(i, j int) bool { return uf.Entries[i].ID < uf.Entries[j].ID })
	return uf
}

func applyUsageMutation(usage map[string]usageRow, mutation usageMutation) {
	row := usage[mutation.EntryID]
	row.Freq++
	row.LastTS = mutation.LastTS
	usage[mutation.EntryID] = row
}

func pruneUsage(usage map[string]usageRow) {
	if len(usage) <= maxUsageEntries {
		return
	}
	type item struct {
		id  string
		row usageRow
	}
	items := make([]item, 0, len(usage))
	for id, row := range usage {
		items = append(items, item{id: id, row: row})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].row.LastTS != items[j].row.LastTS {
			return items[i].row.LastTS > items[j].row.LastTS
		}
		if items[i].row.Freq != items[j].row.Freq {
			return items[i].row.Freq > items[j].row.Freq
		}
		return items[i].id < items[j].id
	})
	for _, old := range items[maxUsageEntries:] {
		delete(usage, old.id)
	}
}

func (uf usageFile) toMap() map[string]usageRow {
	m := make(map[string]usageRow, len(uf.Entries))
	for _, r := range uf.Entries {
		m[r.ID] = usageRow{Freq: r.Freq, LastTS: r.LastTS}
	}
	return m
}
