package rank

import (
	"fmt"
	"testing"

	"github.com/liliumapps/commando/internal/entry"
)

// benchEntries builds n history-like entries with realistic command text.
func benchEntries(n int) []entry.Entry {
	verbs := []string{"git commit -m", "kubectl get pods -n", "docker run --rm -it", "npm run", "cargo test --package", "ssh deploy@host", "grep -rn TODO", "make -C build"}
	out := make([]entry.Entry, n)
	for i := range out {
		cmd := fmt.Sprintf("%s target-%d/path/to/Some_File_%d.go --flag=%d", verbs[i%len(verbs)], i%997, i, i%13)
		out[i] = entry.Entry{
			Title:         cmd,
			Desc:          cmd,
			Template:      cmd,
			Kind:          entry.CommandLiteral,
			Source:        entry.SourceHistory,
			ProviderOrder: int64(i + 1),
		}
		if i%50 == 0 {
			out[i].LastUsed = int64(1_700_000_000 + i)
			out[i].UseCount = i % 7
		}
		out[i].PrepareSearch()
	}
	return out
}

// BenchmarkEntries100k measures one keystroke's ranking over a 100k-entry
// history. Target: well under 30ms/op.
func BenchmarkEntries100k(b *testing.B) {
	entries := benchEntries(100_000)
	for _, q := range []string{"", "g", "kub pods", "file_42", "xqz"} {
		b.Run("q="+q, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = Entries(entries, q)
			}
		})
	}
}
