package tui

import (
	"io"
	"os"
)

// stderr returns the writer the TUI renders to. Rendering to stderr keeps
// stdout clean so a shell widget can capture the assembled command from stdout
// while the interactive UI is drawn on stderr (the terminal).
func stderr() io.Writer { return os.Stderr }
