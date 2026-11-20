package config

import (
	"regexp"
	"strings"
	"sync"
)

// ShouldIgnore reports whether a command line should be excluded from history
// (or from memory and bookmarks) because it looks like it carries a secret.
//
// Two checks apply:
//
//   - patterns: shell-style globs matched case-insensitively against the whole
//     command line and against each whitespace-separated token. Unlike
//     filepath.Match, `*` matches any run of characters including `/`, so
//     "*password*" catches "mysql --password=a/b". A pattern without any
//     wildcard is a substring match ("password").
//   - LooksSecret: built-in value-shaped detectors (cloud keys, GitHub/Slack/
//     OpenAI tokens, JWTs, URLs with embedded credentials, `mysql -p<pw>`).
//     These always apply, even when the user replaces the glob list.
func ShouldIgnore(patterns []string, cmd string) bool {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" {
		return true // never record empty commands
	}
	if LooksSecret(trimmed) {
		return true
	}
	return matchesIgnorePatterns(patterns, trimmed)
}

func matchesIgnorePatterns(patterns []string, cmd string) bool {
	lc := strings.ToLower(cmd)
	var tokens []string
	for _, p := range patterns {
		lp := strings.ToLower(strings.TrimSpace(p))
		if lp == "" {
			continue
		}
		if !strings.ContainsAny(lp, "*?[") {
			if strings.Contains(lc, lp) {
				return true
			}
			continue
		}
		re := compileGlob(lp)
		if re == nil {
			continue
		}
		if re.MatchString(lc) {
			return true
		}
		// Also try each whitespace-separated token so an anchored pattern like
		// "aws_*=*" catches "AWS_SECRET=x" mid-line.
		if tokens == nil {
			tokens = strings.Fields(lc)
		}
		for _, tok := range tokens {
			if re.MatchString(tok) {
				return true
			}
		}
	}
	return false
}

var globCache sync.Map // pattern -> *regexp.Regexp (nil stored as typed nil)

// compileGlob converts a glob into an anchored regular expression where `*`
// matches any sequence (including `/`), `?` matches one character, and
// `[...]` is a character class (`[!...]` negates). A malformed pattern returns
// nil and is ignored.
func compileGlob(glob string) *regexp.Regexp {
	if v, ok := globCache.Load(glob); ok {
		return v.(*regexp.Regexp)
	}
	var b strings.Builder
	b.WriteString(`(?s)^`)
	rs := []rune(glob)
	for i := 0; i < len(rs); i++ {
		switch r := rs[i]; r {
		case '*':
			b.WriteString(`.*`)
		case '?':
			b.WriteString(`.`)
		case '[':
			end := i + 1
			if end < len(rs) && (rs[end] == '!' || rs[end] == '^') {
				end++
			}
			if end < len(rs) && rs[end] == ']' {
				end++
			}
			for end < len(rs) && rs[end] != ']' {
				end++
			}
			if end >= len(rs) {
				b.WriteString(`\[`) // unterminated: literal bracket
				continue
			}
			class := rs[i+1 : end]
			b.WriteByte('[')
			if len(class) > 0 && (class[0] == '!' || class[0] == '^') {
				b.WriteByte('^')
				class = class[1:]
			}
			for _, c := range class {
				if c == '\\' || c == '[' || c == ']' {
					b.WriteByte('\\')
				}
				b.WriteRune(c)
			}
			b.WriteByte(']')
			i = end
		case '\\':
			if i+1 < len(rs) {
				i++
				b.WriteString(regexp.QuoteMeta(string(rs[i])))
			} else {
				b.WriteString(`\\`)
			}
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString(`$`)
	re, err := regexp.Compile(b.String())
	if err != nil {
		re = nil
	}
	globCache.Store(glob, re)
	return re
}

// secretValueREs detect secret-shaped values regardless of the surrounding
// command. They are case-sensitive on purpose: token prefixes are.
var secretValueREs = []*regexp.Regexp{
	regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`),                                // AWS access key id
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}`),                                 // GitHub tokens
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}`),                               // GitHub fine-grained PAT
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}`),                                      // OpenAI/Stripe-style keys
	regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}`),                               // Slack tokens
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}`), // JWT
	regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^/\s:@]+:[^/\s@]+@`),                // URL with user:pass@
	regexp.MustCompile(`(?:^|[\s;&|(])mysql(?:dump|admin)?\b.*\s-p[^\s-]\S*`),          // mysql -p<pw>
	regexp.MustCompile(`(?i)\bauthorization:\s*\S`),                                    // Authorization header
	regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{8,}`),                        // Bearer token
}

// LooksSecret reports whether s contains a value shaped like a credential
// (cloud access keys, GitHub/Slack/OpenAI tokens, JWTs, URLs with embedded
// user:password, an inline `mysql -p<password>`, or an Authorization/Bearer
// header). It is independent of the configurable glob patterns.
func LooksSecret(s string) bool {
	for _, re := range secretValueREs {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}
