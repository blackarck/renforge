// Package meta reads and caches file metadata (size, modification time and
// photo EXIF data) used by sorting and by the "Rename from template" step.
package meta

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rwcarlsen/goexif/exif"
)

// Level says how much information is needed about a file.
type Level int

const (
	None Level = iota // nothing
	Stat              // size and modification time (cheap)
	EXIF              // also camera/date/ISO from the image (reads the file)
)

// Info is what is known about one file.
type Info struct {
	Level   Level
	ModTime time.Time
	Size    int64

	Taken time.Time // EXIF capture time; zero if the file has none
	Make  string
	Model string
	Lens  string
	ISO   int
}

// Date returns the capture time, falling back to the modification time.
func (i Info) Date() time.Time {
	if !i.Taken.IsZero() {
		return i.Taken
	}
	return i.ModTime
}

var cache sync.Map // path -> Info

// Reset forgets everything that was cached.
func Reset() {
	cache.Range(func(k, _ any) bool {
		cache.Delete(k)
		return true
	})
}

// Cached returns the cached info for path if it was loaded to at least level need.
func Cached(path string, need Level) (Info, bool) {
	if v, ok := cache.Load(path); ok {
		if info := v.(Info); info.Level >= need {
			return info, true
		}
	}
	return Info{}, false
}

// imageExts are the formats EXIF is read from. JPEG and TIFF-based files
// (including many camera RAW formats) work; HEIC and CR3 carry no readable
// EXIF for this reader and fall back to the file's modification time.
var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".tif": true, ".tiff": true, ".png": true,
	".cr2": true, ".cr3": true, ".crw": true, ".dng": true, ".nef": true, ".arw": true, ".rw2": true,
	".heic": true, ".heif": true, ".avif": true,
}

// Load reads info for path (at level need) and caches it. Files without EXIF
// simply get no Taken/Make/Model; errors never propagate.
func Load(path string, need Level) Info {
	if info, ok := Cached(path, need); ok {
		return info
	}
	info := Info{Level: Stat}
	if fi, err := os.Stat(path); err == nil {
		info.ModTime, info.Size = fi.ModTime(), fi.Size()
	}
	if need >= EXIF {
		info.Level = EXIF
		if imageExts[strings.ToLower(filepath.Ext(path))] {
			readEXIF(path, &info)
		}
	}
	cache.Store(path, info)
	return info
}

func readEXIF(path string, info *Info) {
	defer func() { _ = recover() }() // a malformed file must never crash the app
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	x, err := exif.Decode(f)
	if err != nil {
		return
	}
	str := func(name exif.FieldName) string {
		if t, err := x.Get(name); err == nil {
			if s, err := t.StringVal(); err == nil {
				return strings.TrimSpace(s)
			}
		}
		return ""
	}
	info.Make = str(exif.Make)
	info.Model = str(exif.Model)
	info.Lens = str(exif.LensModel)
	if t, err := x.Get(exif.ISOSpeedRatings); err == nil {
		if v, err := t.Int(0); err == nil {
			info.ISO = v
		}
	}
	if t, err := x.DateTime(); err == nil {
		info.Taken = t
	}
}

// Missing reports how many of files are not yet cached at level need.
func Missing(files []string, need Level) int {
	if need == None {
		return 0
	}
	n := 0
	for _, p := range files {
		if _, ok := Cached(p, need); !ok {
			n++
		}
	}
	return n
}

// Ensure loads info for every file that is not cached yet, using several
// workers. progress (optional) is called occasionally with (done, total). It
// returns ctx.Err() if cancelled.
func Ensure(ctx context.Context, files []string, need Level, progress func(done, total int)) error {
	if need == None {
		return nil
	}
	var todo []string
	for _, p := range files {
		if _, ok := Cached(p, need); !ok {
			todo = append(todo, p)
		}
	}
	if len(todo) == 0 {
		return nil
	}

	workers := runtime.NumCPU()
	if workers > 8 {
		workers = 8
	}
	var next, done atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				i := int(next.Add(1)) - 1
				if i >= len(todo) {
					return
				}
				Load(todo[i], need)
				if d := int(done.Add(1)); progress != nil && d%100 == 0 {
					progress(d, len(todo))
				}
			}
		}()
	}
	wg.Wait()
	return ctx.Err()
}
