package shell

import "strings"

func controlKey(keyBinding string) byte {
	key := strings.TrimSpace(keyBinding)
	if len(key) == 2 && key[0] == '^' {
		switch {
		case key[1] >= 'A' && key[1] <= 'Z':
			return key[1] + ('a' - 'A')
		case key[1] >= 'a' && key[1] <= 'z':
			return key[1]
		}
	}
	return 'g'
}

func caretBinding(keyBinding string) string {
	key := controlKey(keyBinding)
	return "^" + string(key-('a'-'A'))
}
