package state

import (
	"sort"

	"github.com/liliumapps/commando/internal/rank"
)

const maxRememberedValuesPerBucket = 25

// memoryKey identifies a remembered-value bucket: the values chosen for a given
// variable of a given entry within a given scope.
type memoryKey struct {
	EntryID string
	VarName string
	Scope   string
}

// memoryValue is one remembered value with its ranking data.
type memoryValue struct {
	Value  string `toml:"value"`
	Freq   int    `toml:"freq"`
	LastTS int64  `toml:"last_used"`
}

type memoryMutation struct {
	Key    memoryKey
	Value  string
	LastTS int64
}

// RememberedValue is a remembered candidate value returned to callers.
type RememberedValue struct {
	Value string
	Freq  int
	LastN int64
}

// Remember records that value was chosen for (entryID, varName, scope): it
// increments the frequency and updates the last-used timestamp, creating the
// bucket if needed, then persists. Empty values are ignored.
func (s *State) Remember(entryID, varName, scope, value string) error {
	if value == "" {
		return nil
	}
	s.mu.Lock()
	mutation := memoryMutation{
		Key:    memoryKey{entryID, varName, scope},
		Value:  value,
		LastTS: s.now().Unix(),
	}
	applyMemoryMutation(s.memory, mutation)
	s.pendingMemory = append(s.pendingMemory, mutation)
	deferred := s.batchDepth > 0
	s.mu.Unlock()
	if deferred {
		return nil
	}
	return s.flushMemory()
}

// Recall returns the remembered values for (entryID, varName, scope) with their
// usage data (frequency and last-used time). Ordering is left to the ranker;
// the returned slice carries what it needs to rank by recency.
func (s *State) Recall(entryID, varName, scope string) []RememberedValue {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.memory[memoryKey{entryID, varName, scope}]
	out := make([]RememberedValue, len(src))
	for i, v := range src {
		out[i] = RememberedValue{Value: v.Value, Freq: v.Freq, LastN: v.LastTS}
	}
	return out
}

// --- on-disk representation ---
//
// memory.toml is an array of tables, one per remembered value, so the whole
// file is a flat, greppable list a user can read or prune by hand:
//
//   [[entry]]
//   id = "<entry id>"
//   var = "branch"
//   scope = "project:/path"
//   value = "main"
//   freq = 4
//   last_used = 1700000000

type memoryFile struct {
	Entries []memoryRow `toml:"entry"`
}

type memoryRow struct {
	ID     string `toml:"id"`
	Var    string `toml:"var"`
	Scope  string `toml:"scope"`
	Value  string `toml:"value"`
	Freq   int    `toml:"freq"`
	LastTS int64  `toml:"last_used"`
}

func memoryMapToFile(memory map[memoryKey][]memoryValue) memoryFile {
	var mf memoryFile
	for key, vals := range memory {
		for _, v := range vals {
			mf.Entries = append(mf.Entries, memoryRow{
				ID: key.EntryID, Var: key.VarName, Scope: key.Scope,
				Value: v.Value, Freq: v.Freq, LastTS: v.LastTS,
			})
		}
	}
	// Deterministic ordering keeps the file diff-friendly across saves.
	sort.Slice(mf.Entries, func(i, j int) bool {
		a, b := mf.Entries[i], mf.Entries[j]
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		if a.Var != b.Var {
			return a.Var < b.Var
		}
		if a.Scope != b.Scope {
			return a.Scope < b.Scope
		}
		return a.Value < b.Value
	})
	return mf
}

func applyMemoryMutation(memory map[memoryKey][]memoryValue, mutation memoryMutation) {
	vals := memory[mutation.Key]
	for i := range vals {
		if vals[i].Value != mutation.Value {
			continue
		}
		vals[i].Freq++
		vals[i].LastTS = mutation.LastTS
		memory[mutation.Key] = vals
		pruneMemoryBucket(memory, mutation.Key, mutation.LastTS)
		return
	}
	memory[mutation.Key] = append(vals, memoryValue{
		Value:  mutation.Value,
		Freq:   1,
		LastTS: mutation.LastTS,
	})
	pruneMemoryBucket(memory, mutation.Key, mutation.LastTS)
}

func pruneMemory(memory map[memoryKey][]memoryValue, now int64) {
	for key := range memory {
		pruneMemoryBucket(memory, key, now)
	}
}

// pruneMemoryBucket keeps the maxRememberedValuesPerBucket hottest values by
// frecency (use count weighted by the age of the last use), so a value chosen
// often is not evicted by a burst of one-off values. A value used at `now` is
// always kept, so remembering a new value never drops it straight away.
func pruneMemoryBucket(memory map[memoryKey][]memoryValue, key memoryKey, now int64) {
	vals := memory[key]
	if len(vals) <= maxRememberedValuesPerBucket {
		return
	}
	sort.Slice(vals, func(i, j int) bool {
		// A value used just now (the one being remembered) always survives.
		if ni, nj := vals[i].LastTS >= now, vals[j].LastTS >= now; ni != nj {
			return ni
		}
		fi := rank.Frecency(max(vals[i].Freq, 1), vals[i].LastTS, now)
		fj := rank.Frecency(max(vals[j].Freq, 1), vals[j].LastTS, now)
		if fi != fj {
			return fi > fj
		}
		if vals[i].LastTS != vals[j].LastTS {
			return vals[i].LastTS > vals[j].LastTS
		}
		if vals[i].Freq != vals[j].Freq {
			return vals[i].Freq > vals[j].Freq
		}
		return vals[i].Value < vals[j].Value
	})
	memory[key] = vals[:maxRememberedValuesPerBucket]
}

func (mf memoryFile) toMap() map[memoryKey][]memoryValue {
	m := make(map[memoryKey][]memoryValue)
	for _, r := range mf.Entries {
		key := memoryKey{r.ID, r.Var, r.Scope}
		m[key] = append(m[key], memoryValue{Value: r.Value, Freq: r.Freq, LastTS: r.LastTS})
	}
	return m
}
