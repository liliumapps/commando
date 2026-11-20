package state

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/liliumapps/commando/internal/persist"
)

// State holds commando's mutable, file-backed state: remembered variable values
// (memory), remembered completed forms (fills), and per-entry launch counts
// (usage). All are persisted as plain
// TOML files in the data directory. State is safe for concurrent use.
type State struct {
	memoryPath string
	usagePath  string
	fillsPath  string

	mu     sync.RWMutex
	memory map[memoryKey][]memoryValue
	usage  map[string]usageRow
	fills  map[fillKey][]fillValue

	pendingMemory []memoryMutation
	pendingUsage  []usageMutation
	pendingFills  []fillMutation
	clearMemory   bool
	clearFills    bool
	clearUsage    bool
	batchDepth    int

	// recovered lists corrupt files moved aside so startup could continue.
	recovered []persist.Quarantine

	// now returns the current time; overridable in tests.
	now func() time.Time
}

// Open loads state from memory.toml, usage.toml, and fills.toml under dir, creating the
// directory if needed. Missing files are treated as empty state (not an error),
// so a fresh install starts clean. A file that exists but does not parse is
// moved aside to "<name>.corrupt-<unix>" and treated as empty, so one bad
// write cannot block startup; Recovered reports what was moved.
func Open(dir string) (*State, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &State{
		memoryPath: filepath.Join(dir, "memory.toml"),
		usagePath:  filepath.Join(dir, "usage.toml"),
		fillsPath:  filepath.Join(dir, "fills.toml"),
		memory:     map[memoryKey][]memoryValue{},
		fills:      map[fillKey][]fillValue{},
		usage:      map[string]usageRow{},
		now:        time.Now,
	}
	if err := s.loadMemory(); err != nil {
		return nil, err
	}
	if err := s.loadUsage(); err != nil {
		return nil, err
	}
	if err := s.loadFills(); err != nil {
		return nil, err
	}
	return s, nil
}

// SetClock overrides the time source; intended for tests.
func (s *State) SetClock(now func() time.Time) { s.now = now }

// Recovered describes corrupt state files that were moved aside (during Open
// or a later flush), for diagnostics.
func (s *State) Recovered() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.recovered))
	for i, q := range s.recovered {
		out[i] = q.String()
	}
	return out
}

// readMemoryFile reads memory.toml, quarantining a corrupt file. Callers hold
// s.mu for writing or are in Open.
func (s *State) readMemoryFile() (memoryFile, error) {
	mf, q, err := persist.ReadTOMLOrQuarantine[memoryFile](s.memoryPath, s.now())
	if q != nil {
		s.recovered = append(s.recovered, *q)
	}
	return mf, err
}

func (s *State) readUsageFile() (usageFile, error) {
	uf, q, err := persist.ReadTOMLOrQuarantine[usageFile](s.usagePath, s.now())
	if q != nil {
		s.recovered = append(s.recovered, *q)
	}
	return uf, err
}

func (s *State) readFillsFile() (fillsFile, error) {
	ff, q, err := persist.ReadTOMLOrQuarantine[fillsFile](s.fillsPath, s.now())
	if q != nil {
		s.recovered = append(s.recovered, *q)
	}
	return ff, err
}

func (s *State) loadFills() error {
	ff, err := s.readFillsFile()
	if err != nil {
		return err
	}
	s.fills = ff.toMap()
	return nil
}

func (s *State) loadMemory() error {
	mf, err := s.readMemoryFile()
	if err != nil {
		return err
	}
	s.memory = mf.toMap()
	return nil
}

func (s *State) loadUsage() error {
	uf, err := s.readUsageFile()
	if err != nil {
		return err
	}
	s.usage = uf.toMap()
	return nil
}

// BeginBatch defers persistence until the matching EndBatch. Mutations remain
// immediately visible through Recall, RecallFills, and EntryUsage. Batches may be nested.
func (s *State) BeginBatch() {
	s.mu.Lock()
	s.batchDepth++
	s.mu.Unlock()
}

// EndBatch closes one batch and flushes when the outermost batch ends.
func (s *State) EndBatch() error {
	s.mu.Lock()
	if s.batchDepth > 0 {
		s.batchDepth--
	}
	flush := s.batchDepth == 0
	s.mu.Unlock()
	if flush {
		return s.Flush()
	}
	return nil
}

// Flush merges pending mutations with the latest files under cross-process
// locks. This prevents concurrent commando processes from overwriting each
// other's usage or memory updates.
func (s *State) Flush() error {
	return errors.Join(s.flushMemory(), s.flushUsage(), s.flushFills())
}

// Close persists any pending batch. It is safe to call more than once.
func (s *State) Close() error { return s.Flush() }

func (s *State) flushMemory() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.clearMemory && len(s.pendingMemory) == 0 {
		return nil
	}

	return persist.WithFileLock(s.memoryPath, func() error {
		disk, err := s.readMemoryFile()
		if err != nil {
			return err
		}
		merged := disk.toMap()
		if s.clearMemory {
			merged = map[memoryKey][]memoryValue{}
		}
		for _, mutation := range s.pendingMemory {
			applyMemoryMutation(merged, mutation)
		}
		pruneMemory(merged, s.now().Unix())
		if err := persist.WriteTOML(s.memoryPath, memoryMapToFile(merged)); err != nil {
			return err
		}
		s.memory = merged
		s.pendingMemory = nil
		s.clearMemory = false
		return nil
	})
}

func (s *State) flushUsage() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.clearUsage && len(s.pendingUsage) == 0 {
		return nil
	}

	return persist.WithFileLock(s.usagePath, func() error {
		disk, err := s.readUsageFile()
		if err != nil {
			return err
		}
		merged := disk.toMap()
		if s.clearUsage {
			merged = map[string]usageRow{}
		}
		for _, mutation := range s.pendingUsage {
			applyUsageMutation(merged, mutation)
		}
		pruneUsage(merged)
		if err := persist.WriteTOML(s.usagePath, usageMapToFile(merged)); err != nil {
			return err
		}
		s.usage = merged
		s.pendingUsage = nil
		s.clearUsage = false
		return nil
	})
}

// ClearMemory removes all remembered argument values, including the recent
// fills (whole completed forms) stored alongside them.
func (s *State) ClearMemory() error {
	s.mu.Lock()
	s.memory = map[memoryKey][]memoryValue{}
	s.pendingMemory = nil
	s.clearMemory = true
	s.fills = map[fillKey][]fillValue{}
	s.pendingFills = nil
	s.clearFills = true
	deferred := s.batchDepth > 0
	s.mu.Unlock()
	if deferred {
		return nil
	}
	return errors.Join(s.flushMemory(), s.flushFills())
}

// ClearUsage removes all entry launch-ranking data.
func (s *State) ClearUsage() error {
	s.mu.Lock()
	s.usage = map[string]usageRow{}
	s.pendingUsage = nil
	s.clearUsage = true
	deferred := s.batchDepth > 0
	s.mu.Unlock()
	if deferred {
		return nil
	}
	return s.flushUsage()
}
