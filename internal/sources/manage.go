package sources

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/liliumapps/commando/internal/config"
)

// Manager adds, updates, lists, and removes sources.
type Manager struct {
	Paths config.Paths
	// Out receives progress and the review of commands to trust.
	Out io.Writer
	// Confirm asks whether to trust the commands just printed to Out. It is
	// only called when there is something to review.
	Confirm func() bool
	// now is replaceable in tests.
	now func() time.Time
}

func (m *Manager) timestamp() string {
	now := time.Now
	if m.now != nil {
		now = m.now
	}
	return now().UTC().Format(time.RFC3339)
}

// command is one src or preview command a source would run.
type command struct {
	title, kind, cmd string
}

// commands lists the distinct src and preview commands in entries, in order,
// each under the title of the first cheat that uses it.
func commands(got []loaded) []command {
	var out []command
	seen := make(map[string]bool)
	for _, l := range got {
		for _, v := range l.entry.Vars {
			for _, c := range []command{{l.entry.TitleText(), "src", v.Src}, {l.entry.TitleText(), "preview", v.Preview}} {
				if strings.TrimSpace(c.cmd) == "" || seen[c.cmd] {
					continue
				}
				seen[c.cmd] = true
				out = append(out, c)
			}
		}
	}
	return out
}

// review prints the commands and asks for trust. Commands not in known are
// marked as new. It returns true without asking when there is nothing to run.
func (m *Manager) review(what, commit string, cmds []command, known map[string]bool) bool {
	fmt.Fprintf(m.Out, "\n%s\ncommit %s\n", what, commit)
	if len(cmds) == 0 {
		fmt.Fprintln(m.Out, "These cheats run no src or preview commands.")
		return true
	}
	fmt.Fprintln(m.Out, "These cheats run the following commands on your machine (src fills candidate lists, preview shows the highlighted value):")
	marked := false
	title := ""
	for _, c := range cmds {
		if c.title != title {
			title = c.title
			fmt.Fprintf(m.Out, "\n  %s\n", title)
		}
		mark := " "
		if known != nil && !known[c.cmd] {
			mark = "*"
			marked = true
		}
		fmt.Fprintf(m.Out, "  %s %-8s %s\n", mark, c.kind, c.cmd)
	}
	if marked {
		fmt.Fprintln(m.Out, "\n  * new or changed since the reviewed commit")
	}
	fmt.Fprintln(m.Out)
	return m.Confirm != nil && m.Confirm()
}

func loadErrors(w io.Writer, errs []error) {
	for _, err := range errs {
		fmt.Fprintf(w, "warning: %v\n", err)
	}
}

// Add adds a source: the whole repository when file is empty, or one file in
// it. The repository is cloned (or the existing clone reused), its commands
// are reviewed, and the source is recorded at the reviewed commit. Declining
// leaves nothing behind.
func (m *Manager) Add(remote, file string) error {
	s := Source{URL: remote, File: file}
	if err := s.validate(); err != nil {
		return err
	}
	srcs, err := Read(m.Paths)
	if err != nil {
		return err
	}
	var shared []Source
	for _, o := range srcs {
		if o.same(s) {
			return fmt.Errorf("%s is already a source", s.Key())
		}
		if o.URL == s.URL {
			shared = append(shared, o)
		}
	}

	dir := CloneDir(m.Paths, remote)
	var commit, tmp string
	if len(shared) > 0 && isClone(dir) {
		// Reuse the clone at the commit its other sources were reviewed at.
		commit = shared[0].Commit
		if commit == "" {
			return fmt.Errorf("%s has an unreviewed source; run `commando cheats update` first", remote)
		}
		if head, err := gitHead(dir); err != nil || head != commit {
			if err := gitReset(dir, commit); err != nil {
				return fmt.Errorf("restore %s to %s: %w", remote, short(commit), err)
			}
		}
	} else {
		if err := os.MkdirAll(ReposDir(m.Paths), 0o755); err != nil {
			return err
		}
		tmp, err = os.MkdirTemp(ReposDir(m.Paths), ".clone-")
		if err != nil {
			return err
		}
		defer func() {
			if tmp != "" {
				_ = os.RemoveAll(tmp) // best effort: a leftover .clone-* dir is inert
			}
		}()
		fmt.Fprintf(m.Out, "Cloning %s ...\n", remote)
		if commit, err = gitClone(remote, tmp); err != nil {
			return err
		}
	}

	work := dir
	if tmp != "" {
		work = tmp
	}
	got, errs := loadScope(work, s)
	loadErrors(m.Out, errs)
	if len(got) == 0 {
		if file != "" {
			return fmt.Errorf("%s has no usable cheats", file)
		}
		return fmt.Errorf("%s has no usable cheat files (*.toml)", remote)
	}
	if !m.review(fmt.Sprintf("Source %s (%s), %d command(s)", remote, s.Scope(), len(got)), commit, commands(got), nil) {
		return errDeclined
	}

	if tmp != "" {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		if err := os.Rename(tmp, dir); err != nil {
			return err
		}
		tmp = ""
	}
	s.Commit = commit
	s.Updated = m.timestamp()
	if err := Write(m.Paths, append(srcs, s)); err != nil {
		if len(shared) == 0 {
			_ = os.RemoveAll(dir) // undo the clone; the write error is what matters
		}
		return err
	}
	fmt.Fprintf(m.Out, "Added %s at %s: %d command(s).\n", s.Key(), short(commit), len(got))
	return nil
}

// errDeclined reports that the user did not trust the reviewed commands.
var errDeclined = errors.New("not trusted; nothing changed")

// IsDeclined reports whether err means the user declined a review.
func IsDeclined(err error) bool { return errors.Is(err, errDeclined) }

// Update fetches every source's repository and moves it to the remote's
// latest default-branch commit. When any src or preview command in a
// source's scope is new or changed, the commands are reviewed first; declining
// keeps the previously reviewed commit checked out. Sources without a clone
// or a recorded commit (for example, added to sources.toml by hand) are
// cloned and reviewed in full.
func (m *Manager) Update() error {
	srcs, err := Read(m.Paths)
	if err != nil {
		return err
	}
	if len(srcs) == 0 {
		fmt.Fprintln(m.Out, "No cheat sources. Add one with `commando cheats add <git-url> [file]`.")
		return nil
	}
	var order []string
	byURL := make(map[string][]int)
	for i, s := range srcs {
		if _, ok := byURL[s.URL]; !ok {
			order = append(order, s.URL)
		}
		byURL[s.URL] = append(byURL[s.URL], i)
	}
	var errs []error
	for _, remote := range order {
		if err := m.updateRepo(remote, srcs, byURL[remote]); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", remote, err))
			if !IsDeclined(err) {
				fmt.Fprintf(m.Out, "error: %s: %v\n", remote, err)
			}
		}
	}
	if err := Write(m.Paths, srcs); err != nil {
		return err
	}
	return errors.Join(errs...)
}

// updateRepo updates the sources at idx (all sharing remote) in place.
func (m *Manager) updateRepo(remote string, srcs []Source, idx []int) error {
	dir := CloneDir(m.Paths, remote)
	reviewed := srcs[idx[0]].Commit
	// Commands already trusted, per source: those at its recorded commit.
	known := make([]map[string]bool, len(idx))
	for i := range known {
		known[i] = map[string]bool{}
	}

	var newCommit, tmp string
	if isClone(dir) && reviewed != "" && gitReset(dir, reviewed) == nil {
		for i, j := range idx {
			if srcs[j].Commit != reviewed {
				continue // never reviewed at this commit: review everything
			}
			got, _ := loadScope(dir, srcs[j])
			for _, c := range commands(got) {
				known[i][c.cmd] = true
			}
		}
		fmt.Fprintf(m.Out, "Fetching %s ...\n", remote)
		var err error
		if newCommit, err = gitFetch(dir); err != nil {
			return err
		}
		if err := gitReset(dir, newCommit); err != nil {
			return err
		}
	} else {
		reviewed = ""
		if err := os.MkdirAll(ReposDir(m.Paths), 0o755); err != nil {
			return err
		}
		var err error
		if tmp, err = os.MkdirTemp(ReposDir(m.Paths), ".clone-"); err != nil {
			return err
		}
		defer func() {
			if tmp != "" {
				_ = os.RemoveAll(tmp) // best effort: a leftover .clone-* dir is inert
			}
		}()
		fmt.Fprintf(m.Out, "Cloning %s ...\n", remote)
		if newCommit, err = gitClone(remote, tmp); err != nil {
			return err
		}
	}
	work := dir
	if tmp != "" {
		work = tmp
	}

	// Gather the new commands of every source; review if any is untrusted.
	var all []command
	var marks map[string]bool // commands to show as already reviewed
	if reviewed != "" {
		marks = map[string]bool{}
		for _, k := range known {
			for c := range k {
				marks[c] = true
			}
		}
	}
	changed := false
	counts := make([]int, len(idx))
	seen := map[string]bool{}
	for i, j := range idx {
		got, errs := loadScope(work, srcs[j])
		loadErrors(m.Out, errs)
		counts[i] = len(got)
		for _, c := range commands(got) {
			if !known[i][c.cmd] {
				changed = true
			}
			if !seen[c.cmd] {
				seen[c.cmd] = true
				all = append(all, c)
			}
		}
	}

	if changed {
		what := fmt.Sprintf("Update %s from %s", remote, short(reviewed))
		if !m.review(what, newCommit, all, marks) {
			if reviewed != "" && tmp == "" {
				if err := gitReset(dir, reviewed); err != nil {
					return fmt.Errorf("restore %s: %w", short(reviewed), err)
				}
				fmt.Fprintf(m.Out, "Kept %s at %s.\n", remote, short(reviewed))
			}
			return errDeclined
		}
	}
	if tmp != "" {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		if err := os.Rename(tmp, dir); err != nil {
			return err
		}
		tmp = ""
	}
	stamp := m.timestamp()
	for i, j := range idx {
		srcs[j].Commit = newCommit
		srcs[j].Updated = stamp
		if reviewed == newCommit {
			fmt.Fprintf(m.Out, "%s: up to date at %s, %d command(s).\n", srcs[j].Key(), short(newCommit), counts[i])
		} else {
			fmt.Fprintf(m.Out, "%s: %s -> %s, %d command(s).\n", srcs[j].Key(), short(reviewed), short(newCommit), counts[i])
		}
	}
	return nil
}

// Remove deletes the source (remote, file) — file empty names the
// whole-repository source — and its clone when no other source uses it.
func (m *Manager) Remove(remote, file string) error {
	srcs, err := Read(m.Paths)
	if err != nil {
		return err
	}
	target := Source{URL: remote, File: file}
	kept := srcs[:0:0]
	found, shared := false, false
	for _, s := range srcs {
		switch {
		case s.same(target):
			found = true
		case s.URL == remote:
			shared = true
			kept = append(kept, s)
		default:
			kept = append(kept, s)
		}
	}
	if !found {
		return fmt.Errorf("%s is not a source (see `commando cheats list`)", target.Key())
	}
	if err := Write(m.Paths, kept); err != nil {
		return err
	}
	if !shared {
		if err := os.RemoveAll(CloneDir(m.Paths, remote)); err != nil {
			return err
		}
	}
	fmt.Fprintf(m.Out, "Removed %s.\n", target.Key())
	return nil
}

// List prints each source: URL, file (or "all"), commit, command count, and
// when it was last added or updated. Load problems follow the table.
func (m *Manager) List() error {
	srcs, err := Read(m.Paths)
	if err != nil {
		return err
	}
	if len(srcs) == 0 {
		fmt.Fprintln(m.Out, "No cheat sources. Add one with `commando cheats add <git-url> [file]`.")
		return nil
	}
	tw := tabwriter.NewWriter(m.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "URL\tFILE\tCOMMIT\tCOMMANDS\tUPDATED")
	var problems []error
	for _, s := range srcs {
		got, err := loadTrusted(m.Paths, s)
		count := fmt.Sprint(len(got))
		if err != nil {
			problems = append(problems, err)
			count += "!"
		}
		updated := "-"
		if t, err := time.Parse(time.RFC3339, s.Updated); err == nil {
			updated = t.Local().Format("2006-01-02 15:04")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", s.URL, s.Scope(), short(s.Commit), count, updated)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	for _, p := range problems {
		fmt.Fprintf(m.Out, "\n! %v", p)
	}
	if len(problems) > 0 {
		fmt.Fprintln(m.Out)
	}
	return nil
}
