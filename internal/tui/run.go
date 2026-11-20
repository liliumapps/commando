package tui

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/liliumapps/commando/internal/entry"
	"github.com/muesli/termenv"
)

// Result is the outcome of an interactive session.
type Result struct {
	// Command is the assembled command line, empty when aborted.
	Command string
	// EntryID is the stable ID of the entry the command came from, for usage
	// tracking. Empty when aborted.
	EntryID string
	// Aborted is true when the user quit without selecting a command.
	Aborted bool
}

// Run launches the interactive TUI over the given entries, using backend for
// candidate data and memory, with the given options. It renders to the terminal
// and returns the chosen command. The program is configured to use stderr for
// output so that stdout is reserved for the assembled command when the caller
// pipes it (e.g. the zsh widget captures stdout).
func Run(entries []entry.Entry, backend Backend, opts Options) (Result, error) {
	// The UI is drawn on stderr so stdout stays clean for the shell widget to
	// capture the chosen command. Colour capability, however, is detected from
	// the output device, so we must pin lipgloss's default renderer to stderr —
	// otherwise, when stdout is a pipe (exactly the widget case), colours would
	// be stripped even though stderr is a real terminal.
	renderer := lipgloss.NewRenderer(os.Stderr)
	if os.Getenv("COMMANDO_FORCE_COLOR") != "" {
		renderer.SetColorProfile(termenv.ANSI256)
	}
	lipgloss.SetDefaultRenderer(renderer)

	m := NewWithOptions(entries, backend, opts)
	p := tea.NewProgram(m, tea.WithOutput(stderr()), tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return Result{Aborted: true}, err
	}
	fm := final.(*Model)
	return Result{Command: fm.Result, EntryID: fm.ResultID, Aborted: fm.Aborted}, nil
}
