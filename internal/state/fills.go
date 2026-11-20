package state

import (
	"fmt"
	"sort"
	"strings"

	"github.com/liliumapps/commando/internal/persist"
	"github.com/liliumapps/commando/internal/rank"
)

// maxFillsPerBucket caps how many completed forms are kept per (entry, scope).
const maxFillsPerBucket = 20

// fillKey identifies a recent-fills bucket: the completed forms of one entry
// within one scope.
type fillKey struct {
	EntryID string
	Scope   string
}

// fillValue is one remembered completed form with its ranking data.
type fillValue struct {
	Values map[string]string
	Count  int
	LastTS int64
}

type fillMutation struct {
	Key    fillKey
	Values map[string]string
	LastTS int64
	Forget bool
}

// RememberedFill is a stored completed form returned to callers: the value of
// every remembered variable, keyed by name, with its use count and last use.
type RememberedFill struct {
	Values map[string]string
	Count  int
	LastN  int64
}

// RememberFill records that the given set of variable values was used together
// for (entryID, scope). An identical set bumps its count and last-used time;
// a new set is added, evicting the lowest-frecency set once the bucket holds
// more than 20. Empty values are dropped; an empty set is ignored. Callers are
// responsible for privacy filtering (remember="none", secret-looking values).
func (s *State) RememberFill(entryID, scope string, values map[string]string) error {
	vals := cleanFillValues(values)
	if entryID == "" || scope == "" || len(vals) == 0 {
		return nil
	}
	s.mu.Lock()
	mutation := fillMutation{Key: fillKey{entryID, scope}, Values: vals, LastTS: s.now().Unix()}
	applyFillMutation(s.fills, mutation)
	s.pendingFills = append(s.pendingFills, mutation)
	deferred := s.batchDepth > 0
	s.mu.Unlock()
	if deferred {
		return nil
	}
	return s.flushFills()
}

// ForgetFill removes the stored set equal to values from (entryID, scope). A
// set that is not stored is a no-op.
func (s *State) ForgetFill(entryID, scope string, values map[string]string) error {
	vals := cleanFillValues(values)
	if entryID == "" || scope == "" || len(vals) == 0 {
		return nil
	}
	s.mu.Lock()
	mutation := fillMutation{Key: fillKey{entryID, scope}, Values: vals, Forget: true}
	applyFillMutation(s.fills, mutation)
	s.pendingFills = append(s.pendingFills, mutation)
	deferred := s.batchDepth > 0
	s.mu.Unlock()
	if deferred {
		return nil
	}
	return s.flushFills()
}

// RecallFills returns the stored completed forms for (entryID, scope), hottest
// first by frecency (ties broken by recency).
func (s *State) RecallFills(entryID, scope string) []RememberedFill {
	s.mu.RLock()
	src := s.fills[fillKey{entryID, scope}]
	out := make([]RememberedFill, len(src))
	for i, f := range src {
		vals := make(map[string]string, len(f.Values))
		for k, v := range f.Values {
			vals[k] = v
		}
		out[i] = RememberedFill{Values: vals, Count: f.Count, LastN: f.LastTS}
	}
	now := s.now().Unix()
	s.mu.RUnlock()
	sort.SliceStable(out, func(i, j int) bool {
		fi := rank.Frecency(max(out[i].Count, 1), out[i].LastN, now)
		fj := rank.Frecency(max(out[j].Count, 1), out[j].LastN, now)
		if fi != fj {
			return fi > fj
		}
		if out[i].LastN != out[j].LastN {
			return out[i].LastN > out[j].LastN
		}
		return FillSignature(out[i].Values) < FillSignature(out[j].Values)
	})
	return out
}

// FillSignature is a canonical string for a set of values, used to detect
// identical sets: names sorted, each "name=value" separated by NUL bytes.
func FillSignature(values map[string]string) string {
	names := make([]string, 0, len(values))
	for k := range values {
		names = append(names, k)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, k := range names {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(values[k])
		b.WriteByte(0)
	}
	return b.String()
}

func cleanFillValues(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for k, v := range values {
		if k != "" && v != "" {
			out[k] = v
		}
	}
	return out
}

func (s *State) flushFills() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.clearFills && len(s.pendingFills) == 0 {
		return nil
	}
	return persist.WithFileLock(s.fillsPath, func() error {
		disk, err := s.readFillsFile()
		if err != nil {
			return err
		}
		merged := disk.toMap()
		if s.clearFills {
			merged = map[fillKey][]fillValue{}
		}
		for _, mutation := range s.pendingFills {
			applyFillMutation(merged, mutation)
		}
		pruneFills(merged, s.now().Unix())
		if err := persist.WriteTOML(s.fillsPath, fillsMapToFile(merged)); err != nil {
			return err
		}
		s.fills = merged
		s.pendingFills = nil
		s.clearFills = false
		return nil
	})
}

func applyFillMutation(fills map[fillKey][]fillValue, mutation fillMutation) {
	sig := FillSignature(mutation.Values)
	vals := fills[mutation.Key]
	for i := range vals {
		if FillSignature(vals[i].Values) != sig {
			continue
		}
		if mutation.Forget {
			vals = append(vals[:i:i], vals[i+1:]...)
			if len(vals) == 0 {
				delete(fills, mutation.Key)
			} else {
				fills[mutation.Key] = vals
			}
			return
		}
		vals[i].Count++
		vals[i].LastTS = mutation.LastTS
		fills[mutation.Key] = vals
		pruneFillBucket(fills, mutation.Key, mutation.LastTS)
		return
	}
	if mutation.Forget {
		return
	}
	copied := make(map[string]string, len(mutation.Values))
	for k, v := range mutation.Values {
		copied[k] = v
	}
	fills[mutation.Key] = append(vals, fillValue{Values: copied, Count: 1, LastTS: mutation.LastTS})
	pruneFillBucket(fills, mutation.Key, mutation.LastTS)
}

func pruneFills(fills map[fillKey][]fillValue, now int64) {
	for key := range fills {
		pruneFillBucket(fills, key, now)
	}
}

// pruneFillBucket keeps the maxFillsPerBucket hottest sets by frecency. A set
// used at now always survives, so recording a new set never drops it at once.
func pruneFillBucket(fills map[fillKey][]fillValue, key fillKey, now int64) {
	vals := fills[key]
	if len(vals) <= maxFillsPerBucket {
		return
	}
	sort.SliceStable(vals, func(i, j int) bool {
		if ni, nj := vals[i].LastTS >= now, vals[j].LastTS >= now; ni != nj {
			return ni
		}
		fi := rank.Frecency(max(vals[i].Count, 1), vals[i].LastTS, now)
		fj := rank.Frecency(max(vals[j].Count, 1), vals[j].LastTS, now)
		if fi != fj {
			return fi > fj
		}
		if vals[i].LastTS != vals[j].LastTS {
			return vals[i].LastTS > vals[j].LastTS
		}
		return vals[i].Count > vals[j].Count
	})
	fills[key] = vals[:maxFillsPerBucket]
}

// --- on-disk representation ---
//
// fills.toml is an array of tables, one per remembered completed form:
//
//   [[fill]]
//   id     = "<entry id>"
//   scope  = "global"
//   values = { namespace = "payments", pod = "api-7d9f" }
//   count  = 4
//   last   = 1763631000

type fillsFile struct {
	Fills []fillRow `toml:"fill"`
}

type fillRow struct {
	ID     string     `toml:"id"`
	Scope  string     `toml:"scope"`
	Values fillValues `toml:"values"`
	Count  int        `toml:"count"`
	Last   int64      `toml:"last"`
}

// fillValues encodes as an inline table so each record stays one readable
// block; it decodes as an ordinary table.
type fillValues map[string]string

// MarshalTOML implements toml.Marshaler, writing an inline table with keys in
// sorted order.
func (v fillValues) MarshalTOML() ([]byte, error) {
	names := make([]string, 0, len(v))
	for k := range v {
		names = append(names, k)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("{")
	for i, k := range names {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(" ")
		b.WriteString(tomlKey(k))
		b.WriteString(" = ")
		b.WriteString(tomlString(v[k]))
	}
	if len(names) > 0 {
		b.WriteString(" ")
	}
	b.WriteString("}")
	return []byte(b.String()), nil
}

// tomlKey returns k as a bare key when possible, otherwise quoted.
func tomlKey(k string) string {
	if k == "" {
		return `""`
	}
	for _, r := range k {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return tomlString(k)
		}
	}
	return k
}

// tomlString renders s as a TOML basic string.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func fillsMapToFile(fills map[fillKey][]fillValue) fillsFile {
	var ff fillsFile
	for key, vals := range fills {
		for _, v := range vals {
			ff.Fills = append(ff.Fills, fillRow{
				ID: key.EntryID, Scope: key.Scope, Values: fillValues(v.Values),
				Count: v.Count, Last: v.LastTS,
			})
		}
	}
	sort.Slice(ff.Fills, func(i, j int) bool {
		a, b := ff.Fills[i], ff.Fills[j]
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		if a.Scope != b.Scope {
			return a.Scope < b.Scope
		}
		if a.Last != b.Last {
			return a.Last > b.Last
		}
		return FillSignature(a.Values) < FillSignature(b.Values)
	})
	return ff
}

func (ff fillsFile) toMap() map[fillKey][]fillValue {
	m := make(map[fillKey][]fillValue)
	for _, r := range ff.Fills {
		vals := cleanFillValues(r.Values)
		if r.ID == "" || r.Scope == "" || len(vals) == 0 {
			continue
		}
		key := fillKey{r.ID, r.Scope}
		// Merge hand-edited duplicates instead of keeping two rows.
		merged := false
		sig := FillSignature(vals)
		for i := range m[key] {
			if FillSignature(m[key][i].Values) == sig {
				m[key][i].Count += max(r.Count, 1)
				m[key][i].LastTS = max(m[key][i].LastTS, r.Last)
				merged = true
				break
			}
		}
		if !merged {
			m[key] = append(m[key], fillValue{Values: vals, Count: max(r.Count, 1), LastTS: r.Last})
		}
	}
	return m
}
