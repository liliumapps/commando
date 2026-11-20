package resolve

import "strings"

// A src command is arbitrary shell text (it may contain '[', ']', '\', quotes,
// etc.), so it must NOT be run through the command-template grammar, which would
// treat those characters as optional segments or escapes. Instead we scan only
// for <name> placeholders, where name is a run of [A-Za-z0-9_.-], and leave
// every other character verbatim.

// isVarNameByte reports whether b is a valid character inside a <placeholder>
// name. It matches the set accepted by the template variable validator.
func isVarNameByte(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z':
		return true
	case b >= 'A' && b <= 'Z':
		return true
	case b >= '0' && b <= '9':
		return true
	case b == '_' || b == '-' || b == '.':
		return true
	default:
		return false
	}
}

// scanPlaceholder checks whether s[i:] begins with a "<name>" placeholder. On
// success it returns the name and the index just past the closing '>'. On
// failure ok is false.
func scanPlaceholder(s string, i int) (name string, next int, ok bool) {
	name, mode, next, ok := scanSrcPlaceholder(s, i)
	return name, next, ok && mode == ""
}

// scanSrcPlaceholder checks whether s[i:] begins with a src-style placeholder:
// "<name>", "<name:shell>", or "<name:raw>". The optional mode is used only by
// shell command substitution; plain command templates still accept just <name>.
func scanSrcPlaceholder(s string, i int) (name, mode string, next int, ok bool) {
	if i >= len(s) || s[i] != '<' {
		return "", "", 0, false
	}
	j := i + 1
	for j < len(s) && isVarNameByte(s[j]) {
		j++
	}
	if j == i+1 || j >= len(s) {
		return "", "", 0, false // empty name or no closing '>'
	}
	name = s[i+1 : j]
	if s[j] == ':' {
		k := j + 1
		for k < len(s) && isVarNameByte(s[k]) {
			k++
		}
		if k == j+1 || k >= len(s) || s[k] != '>' {
			return "", "", 0, false
		}
		return name, s[j+1 : k], k + 1, true
	}
	if s[j] != '>' {
		return "", "", 0, false
	}
	return name, "", j + 1, true
}

// SrcRefs returns the distinct names of <name> placeholders referenced in a src
// command, in order of first appearance. A src that references none returns nil.
func SrcRefs(src string) []string {
	if src == "" {
		return nil
	}
	var names []string
	seen := make(map[string]bool)
	for i := 0; i < len(src); {
		if name, _, next, ok := scanSrcPlaceholder(src, i); ok {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
			i = next
			continue
		}
		i++
	}
	return names
}

// SubstituteRefs replaces every <name> placeholder in src with vals[name],
// leaving all other text (brackets, backslashes, quotes) untouched. A missing
// value substitutes the empty string. The referenced values come from prior
// field inputs/selections and are inserted verbatim, so callers are responsible
// for ensuring they are shell-safe.
func SubstituteRefs(src string, vals map[string]string) string {
	if src == "" || !strings.ContainsRune(src, '<') {
		return src
	}
	var b strings.Builder
	b.Grow(len(src))
	for i := 0; i < len(src); {
		if name, next, ok := scanPlaceholder(src, i); ok {
			b.WriteString(vals[name])
			i = next
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}

// SubstituteShellRefs replaces <name> placeholders in shell text with
// shell-quoted values by default. Use <name:raw> only when the replacement must
// deliberately expand as shell syntax; <name:shell> is accepted for explicitness.
func SubstituteShellRefs(src string, vals map[string]string) string {
	if src == "" || !strings.ContainsRune(src, '<') {
		return src
	}
	var b strings.Builder
	b.Grow(len(src))
	for i := 0; i < len(src); {
		if name, mode, next, ok := scanSrcPlaceholder(src, i); ok {
			v := vals[name]
			if mode == "raw" {
				b.WriteString(v)
			} else {
				b.WriteString(ShellQuote(v))
			}
			i = next
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}

// ShellQuote returns s as a POSIX-shell single-quoted word.
func ShellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
