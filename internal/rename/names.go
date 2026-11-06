// Package rename contains the pure (UI-free) rename engine: the step
// pipeline, name validation, plan building and the two-phase apply.
package rename

import (
	"path/filepath"
	"strings"
)

// compoundExts are multi-part extensions that should be treated as one unit.
var compoundExts = []string{".tar.gz", ".tar.bz2", ".tar.xz", ".tar.zst", ".tar.lz4"}

// SplitExt splits a file name into base and extension (extension includes the
// leading dot). It understands compound extensions such as ".tar.gz" and
// treats dotfiles such as ".bashrc" as having no extension.
func SplitExt(name string) (base, ext string) {
	lower := strings.ToLower(name)
	for _, c := range compoundExts {
		if strings.HasSuffix(lower, c) && len(name) > len(c) {
			return name[:len(name)-len(c)], name[len(name)-len(c):]
		}
	}
	ext = filepath.Ext(name)
	if ext == name { // dotfile, e.g. ".bashrc"
		return name, ""
	}
	return name[:len(name)-len(ext)], ext
}

var reservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// InvalidReason returns a human-readable reason why name cannot be used as a
// file name on common platforms, or "" if it is fine. The rules are the
// strictest common denominator (Windows) so renamed files stay portable.
func InvalidReason(name string) string {
	if strings.TrimSpace(name) == "" {
		return "empty name"
	}
	if name == "." || name == ".." {
		return "reserved name"
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return "control characters"
		}
	}
	if strings.ContainsAny(name, `<>:"/\|?*`) {
		return "invalid characters"
	}
	if len(name) > 255 {
		return "name longer than 255 bytes"
	}
	if strings.HasPrefix(name, " ") {
		return "starts with a space"
	}
	if strings.HasSuffix(name, " ") || strings.HasSuffix(name, ".") {
		return "ends with a space or dot"
	}
	// Windows reserves the stem before the *first* dot ("CON.tar.gz" is reserved).
	stem := name
	if i := strings.Index(stem, "."); i >= 0 {
		stem = stem[:i]
	}
	if reservedNames[strings.ToUpper(strings.TrimRight(stem, " "))] {
		return "reserved filename"
	}
	return ""
}
