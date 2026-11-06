// Package filter implements the file-name filter rules.
package filter

import (
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// Filter modes.
const (
	Contains   = "contains"
	StartsWith = "starts with"
	EndsWith   = "ends with"
	Extension  = "extension"
	Regex      = "regex"
)

// Modes lists every mode in display order.
var Modes = []string{Contains, StartsWith, EndsWith, Extension, Regex}

// Rule is one filter condition. A rule with an empty value matches everything.
type Rule struct {
	ID    int    `json:"-"`
	Mode  string `json:"mode"`
	Value string `json:"value"`
}

// Files returns the paths from all whose base name satisfies the rules.
func Files(all []string, rules []Rule, matchAll, caseSensitive bool) []string {
	out := make([]string, 0, len(all))
	for _, full := range all {
		if Match(filepath.Base(full), rules, matchAll, caseSensitive) {
			out = append(out, full)
		}
	}
	return out
}

// Match reports whether filename satisfies the rules (all of them when
// matchAll is set, otherwise at least one). No rules matches everything.
func Match(filename string, rules []Rule, matchAll, caseSensitive bool) bool {
	if len(rules) == 0 {
		return true
	}
	for _, r := range rules {
		ok := matchRule(filename, r, caseSensitive)
		if matchAll && !ok {
			return false
		}
		if !matchAll && ok {
			return true
		}
	}
	return matchAll
}

func matchRule(filename string, r Rule, caseSensitive bool) bool {
	v := strings.TrimSpace(r.Value)
	if v == "" {
		return true
	}
	if r.Mode == Regex {
		// Match against the original case: (?i) is added for case-insensitive.
		pat := strings.TrimSpace(r.Value)
		if !caseSensitive {
			pat = "(?i)" + pat
		}
		re, err := compile(pat)
		return err == nil && re.MatchString(filename)
	}
	name := filename
	if !caseSensitive {
		name, v = strings.ToLower(name), strings.ToLower(v)
	}
	switch r.Mode {
	case StartsWith:
		return strings.HasPrefix(name, v)
	case EndsWith:
		return strings.HasSuffix(name, v)
	case Extension:
		// ".png" and "png" both work, and so does "tar.gz".
		suffix := "." + strings.TrimPrefix(v, ".")
		return len(name) > len(suffix) && strings.HasSuffix(name, suffix)
	default:
		return strings.Contains(name, v)
	}
}

var reCache sync.Map

func compile(pattern string) (*regexp.Regexp, error) {
	type entry struct {
		re  *regexp.Regexp
		err error
	}
	if v, ok := reCache.Load(pattern); ok {
		e := v.(entry)
		return e.re, e.err
	}
	re, err := regexp.Compile(pattern)
	reCache.Store(pattern, entry{re, err})
	return re, err
}

// Validate returns a human-readable problem with a rule (an invalid regular
// expression), or "".
func Validate(r Rule) string {
	if r.Mode != Regex || strings.TrimSpace(r.Value) == "" {
		return ""
	}
	if _, err := compile(strings.TrimSpace(r.Value)); err != nil {
		return "Invalid regular expression: " + strings.TrimPrefix(err.Error(), "error parsing regexp: ")
	}
	return ""
}
