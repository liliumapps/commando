package app

import (
	"errors"
	"testing"

	"github.com/liliumapps/commando/internal/entry"
	"github.com/liliumapps/commando/internal/provider"
)

// staticProvider returns a fixed set of entries.
type staticProvider struct {
	name    string
	entries []entry.Entry
	err     error
}

func (p staticProvider) Name() string                                    { return p.name }
func (p staticProvider) Entries(provider.Context) ([]entry.Entry, error) { return p.entries, p.err }

func templatesOf(entries []entry.Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Template
	}
	return out
}

func TestAggregateDedupHistoryBehindCurated(t *testing.T) {
	// A history entry duplicating a cheat's command is dropped; unique history
	// is kept.
	provs := []provider.Provider{
		staticProvider{name: "cheat", entries: []entry.Entry{
			{Desc: "deploy", Template: "make deploy", Source: entry.SourceCheat},
		}},
		staticProvider{name: "hist", entries: []entry.Entry{
			{Desc: "make deploy", Template: "make deploy", Source: entry.SourceHistory}, // dup of cheat
			{Desc: "ls -la", Template: "ls -la", Source: entry.SourceHistory},           // unique
		}},
	}
	got, diagnostics := aggregate(provs, provider.Context{}, nil)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	tmpls := templatesOf(got)

	// "make deploy" appears once (the cheat wins); "ls -la" survives.
	count := 0
	for _, tm := range tmpls {
		if tm == "make deploy" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 'make deploy' once after dedup, got %d (%v)", count, tmpls)
	}
	found := false
	for _, tm := range tmpls {
		if tm == "ls -la" {
			found = true
		}
	}
	if !found {
		t.Errorf("unique history command should survive dedup, got %v", tmpls)
	}
	// The surviving "make deploy" must be the cheat, not history.
	for _, e := range got {
		if e.Template == "make deploy" && e.Source != entry.SourceCheat {
			t.Errorf("deduped 'make deploy' should be the cheat, got source %v", e.Source)
		}
	}
}

func TestAggregateKeepsPartialEntriesAndDiagnostics(t *testing.T) {
	provs := []provider.Provider{
		staticProvider{
			name:    "partial",
			entries: []entry.Entry{{Title: "healthy", Template: "echo ok"}},
			err:     errors.New("one file was invalid"),
		},
		staticProvider{
			name:    "healthy",
			entries: []entry.Entry{{Title: "other", Template: "echo other"}},
		},
	}
	got, diagnostics := aggregate(provs, provider.Context{}, nil)
	if len(got) != 2 {
		t.Fatalf("partial provider entries were lost: %+v", got)
	}
	if len(diagnostics) != 1 || diagnostics[0].Source != "partial" {
		t.Fatalf("diagnostics = %+v, want partial provider warning", diagnostics)
	}
}
