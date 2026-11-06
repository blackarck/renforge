package rename

import (
	"path/filepath"
	"strings"
)

// DefaultSidecarExts are the extensions treated as "belonging to" a photo or
// clip with the same name: XMP/edit sidecars and RAW+JPEG pairs.
const DefaultSidecarExts = "xmp, aae, dop, pp3, jpg, jpeg, tif, tiff, png, heic, cr2, cr3, nef, arw, dng, raf, orf, rw2, mov, thm"

// ParseExts turns "xmp, .jpg  CR2" into a lower-case set with leading dots.
func ParseExts(list string) map[string]bool {
	set := map[string]bool{}
	for _, f := range strings.FieldsFunc(list, func(r rune) bool { return r == ',' || r == ' ' || r == ';' }) {
		f = strings.ToLower(strings.TrimSpace(f))
		if f == "" {
			continue
		}
		if !strings.HasPrefix(f, ".") {
			f = "." + f
		}
		set[f] = true
	}
	return set
}

// Related is a file that belongs to a primary file.
type Related struct {
	Path string
	// FullName is true for Adobe-style sidecars named after the *whole*
	// primary name ("IMG_1.CR2.xmp"); otherwise it shares only the base
	// ("IMG_1.xmp", "IMG_1.JPG").
	FullName bool
}

// SidecarIndex finds related files quickly.
type SidecarIndex struct {
	byBase map[string][]string // dir\x00lower(base) -> files
}

func sidecarKey(dir, name string) string { return dir + "\x00" + strings.ToLower(name) }

// NewSidecarIndex indexes every file by its (directory, base name).
func NewSidecarIndex(all []string) *SidecarIndex {
	ix := &SidecarIndex{byBase: make(map[string][]string, len(all))}
	for _, p := range all {
		base, _ := SplitExt(filepath.Base(p))
		k := sidecarKey(filepath.Dir(p), base)
		ix.byBase[k] = append(ix.byBase[k], p)
	}
	return ix
}

// For returns the files related to primary whose extension is in exts.
func (ix *SidecarIndex) For(primary string, exts map[string]bool) []Related {
	if ix == nil {
		return nil
	}
	dir, name := filepath.Dir(primary), filepath.Base(primary)
	base, pext := SplitExt(name)

	var out []Related
	for _, cand := range ix.byBase[sidecarKey(dir, base)] {
		_, cext := SplitExt(filepath.Base(cand))
		if cand != primary && !strings.EqualFold(cext, pext) && exts[strings.ToLower(cext)] {
			out = append(out, Related{Path: cand})
		}
	}
	if pext != "" {
		for _, cand := range ix.byBase[sidecarKey(dir, name)] {
			_, cext := SplitExt(filepath.Base(cand))
			if cand != primary && exts[strings.ToLower(cext)] {
				out = append(out, Related{Path: cand, FullName: true})
			}
		}
	}
	return out
}

// relatedNewName computes the new name of a related file, or "" if the
// primary's rename does not affect it.
func relatedNewName(r Related, oldPrimary, newPrimary string) string {
	_, cext := SplitExt(filepath.Base(r.Path))
	if r.FullName {
		if newPrimary == oldPrimary {
			return ""
		}
		return newPrimary + cext
	}
	oldBase, _ := SplitExt(oldPrimary)
	newBase, _ := SplitExt(newPrimary)
	if newBase == oldBase {
		return ""
	}
	return newBase + cext
}
