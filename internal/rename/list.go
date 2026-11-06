package rename

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ListFiles returns the regular (non-directory) files in folder in natural
// order (img2 before img10). With recursive set it descends into sub-folders;
// unreadable sub-folders are skipped rather than aborting the scan. The scan
// stops early with ctx.Err() when ctx is cancelled. progress, if non-nil, is
// called every 500 files with the running count.
func ListFiles(ctx context.Context, folder string, recursive bool, progress func(n int)) ([]string, error) {
	var files []string
	add := func(p string) {
		files = append(files, p)
		if progress != nil && len(files)%500 == 0 {
			progress(len(files))
		}
	}

	if recursive {
		err := filepath.WalkDir(folder, func(path string, d fs.DirEntry, err error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				if path == folder {
					return err
				}
				if d != nil && d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() || strings.TrimSpace(d.Name()) == "" {
				return nil
			}
			add(path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		entries, err := os.ReadDir(folder)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() || strings.TrimSpace(e.Name()) == "" {
				continue
			}
			add(filepath.Join(folder, e.Name()))
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return NaturalLess(files[i], files[j]) })
	return files, nil
}

// NaturalLess orders strings so embedded numbers compare by value
// ("img2" < "img10") and letters compare case-insensitively. Ties fall back to
// plain string order so the result is deterministic.
func NaturalLess(a, b string) bool {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if isDigit(a[i]) && isDigit(b[j]) {
			si := i
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			sj := j
			for j < len(b) && isDigit(b[j]) {
				j++
			}
			na, nb := strings.TrimLeft(a[si:i], "0"), strings.TrimLeft(b[sj:j], "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		ca, cb := lower(a[i]), lower(b[j])
		if ca != cb {
			return ca < cb
		}
		i++
		j++
	}
	if len(a)-i != len(b)-j {
		return len(a)-i < len(b)-j
	}
	return a < b
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 32
	}
	return c
}
