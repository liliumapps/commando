// Package history provides a Provider that surfaces the user's shell command
// history, read live from the shell history file at launch. Nothing is copied
// into commando's own storage — the shell's history file is the single source
// of truth, keeping all data plain and visible.
package history

import (
	"bytes"
	"strings"
)

// HistEntry is one parsed history command with its timestamp (0 when unknown).
type HistEntry struct {
	Cmd string
	TS  int64
}

// Parse reads shell history file contents and returns the commands in file
// order (oldest first). It understands:
//
//   - zsh extended format lines: ": <ts>:<elapsed>;<command>"
//   - bash HISTTIMEFORMAT timestamp lines: "#<unix seconds>" before a command.
//     Once a file uses them, each timestamp starts a new command and the
//     lines up to the next timestamp form that command, so multi-line
//     commands saved with `shopt -s lithist` stay whole.
//   - plain lines: the whole line is the command
//   - multiline commands: a line ending in an unescaped backslash continues on
//     the next line (both zsh and bash write history this way)
//
// CRLF line endings are accepted. Blank commands are skipped. TS is the parsed
// timestamp, or 0 when unknown.
func Parse(content string) []HistEntry {
	lines := strings.Split(content, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	var out []HistEntry

	i := 0
	for i < len(lines) {
		line := lines[i]
		i++

		if ts, ok := bashTimestamp(line); ok {
			// Gather the command lines up to the next timestamp line.
			var parts []string
			for i < len(lines) {
				if _, next := bashTimestamp(lines[i]); next {
					break
				}
				parts = append(parts, lines[i])
				i++
			}
			// Trailing empty lines (end of file) are not part of the command.
			for len(parts) > 0 && strings.TrimSpace(parts[len(parts)-1]) == "" {
				parts = parts[:len(parts)-1]
			}
			cmd := strings.TrimSpace(joinContinuations(parts))
			if cmd != "" {
				out = append(out, HistEntry{Cmd: cmd, TS: ts})
			}
			continue
		}

		ts, cmd := splitExtended(line)

		// Join backslash-continued lines. A trailing backslash that is not
		// itself escaped means "continued on the next line".
		for endsWithContinuation(cmd) && i < len(lines) {
			cmd = strings.TrimSuffix(cmd, "\\") + "\n" + lines[i]
			i++
		}

		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		out = append(out, HistEntry{Cmd: cmd, TS: ts})
	}
	return out
}

// joinContinuations joins the lines of one bash history record. Backslash
// continuations are folded the same way as untimestamped history; other line
// breaks (lithist) are kept as newlines.
func joinContinuations(parts []string) string {
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteByte('\n')
		}
		if endsWithContinuation(p) && i < len(parts)-1 {
			p = strings.TrimSuffix(p, "\\")
		}
		b.WriteString(p)
	}
	return b.String()
}

// bashTimestamp parses a bash HISTTIMEFORMAT comment line "#<digits>".
func bashTimestamp(line string) (int64, bool) {
	if len(line) < 2 || line[0] != '#' {
		return 0, false
	}
	for i := 1; i < len(line); i++ {
		if line[i] < '0' || line[i] > '9' {
			return 0, false
		}
	}
	ts := parseInt(line[1:])
	return ts, ts > 0
}

// zshMeta is zsh's metafication marker: in a history file, a byte that is
// special to zsh is written as 0x83 followed by the byte XOR 0x20.
const zshMeta = 0x83

// unmetafyZsh decodes zsh's metafied history bytes back into the original
// text (so non-ASCII commands are not mangled).
func unmetafyZsh(data []byte) []byte {
	i := bytes.IndexByte(data, zshMeta)
	if i < 0 {
		return data
	}
	out := make([]byte, 0, len(data))
	out = append(out, data[:i]...)
	for ; i < len(data); i++ {
		if data[i] == zshMeta && i+1 < len(data) {
			i++
			out = append(out, data[i]^0x20)
			continue
		}
		out = append(out, data[i])
	}
	return out
}

// parseFish reads Fish's YAML-like history format. Command text uses
// backslash escapes and metadata is indented beneath each "- cmd:" record.
func parseFish(content string) []HistEntry {
	lines := strings.Split(content, "\n")
	var out []HistEntry
	current := -1
	for _, line := range lines {
		if strings.HasPrefix(line, "- cmd:") {
			cmd := strings.TrimSpace(strings.TrimPrefix(line, "- cmd:"))
			cmd = decodeFishEscapes(cmd)
			if cmd == "" {
				current = -1
				continue
			}
			out = append(out, HistEntry{Cmd: cmd})
			current = len(out) - 1
			continue
		}
		if current >= 0 {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "when:") {
				out[current].TS = parseInt(strings.TrimSpace(strings.TrimPrefix(trimmed, "when:")))
			}
		}
	}
	return out
}

func decodeFishEscapes(value string) string {
	var out strings.Builder
	out.Grow(len(value))
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' || i+1 >= len(value) {
			out.WriteByte(value[i])
			continue
		}
		i++
		switch value[i] {
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case '\\':
			out.WriteByte('\\')
		default:
			// Fish escapes other bytes by prefixing a backslash.
			out.WriteByte(value[i])
		}
	}
	return out.String()
}

// splitExtended parses a possible zsh extended-format line
// ": <ts>:<elapsed>;<command>". When the line is not in that form the whole
// line is treated as the command and ts is 0.
func splitExtended(line string) (ts int64, cmd string) {
	if !strings.HasPrefix(line, ": ") {
		return 0, line
	}
	semi := strings.IndexByte(line, ';')
	if semi < 0 {
		return 0, line
	}
	meta := line[2:semi] // "<ts>:<elapsed>"
	colon := strings.IndexByte(meta, ':')
	if colon < 0 {
		return 0, line
	}
	ts = parseInt(meta[:colon])
	return ts, line[semi+1:]
}

// endsWithContinuation reports whether s ends with an odd number of backslashes
// (i.e. a real line continuation rather than an escaped backslash).
func endsWithContinuation(s string) bool {
	n := 0
	for i := len(s) - 1; i >= 0 && s[i] == '\\'; i-- {
		n++
	}
	return n%2 == 1
}

// parseInt parses a base-10 integer, returning 0 on any error (timestamps are
// best-effort).
func parseInt(s string) int64 {
	var n int64
	for _, r := range strings.TrimSpace(s) {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int64(r-'0')
	}
	return n
}
