package rename

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"FileRenUtil/internal/meta"
)

// Status describes the state of a plan item.
type Status string

const (
	StatusOK      Status = "ok"      // will be renamed
	StatusSkip    Status = "skip"    // skipped (unchanged / invalid / conflict)
	StatusRenamed Status = "renamed" // renamed successfully
	StatusError   Status = "error"   // rename failed
	StatusDryRun  Status = "dry-run" // would have been renamed
)

// Item is one planned (or executed) rename.
type Item struct {
	OldPath  string
	NewPath  string
	OldName  string
	NewName  string
	Status   Status
	Reason   string
	TempPath string // set only if a file was left stranded under a temp name
	Related  bool   // a sidecar / paired file renamed together with a primary file
	Group    int    // items of one primary file and its related files share a group (1-based)
}

// Summary aggregates the outcome of BuildPlan for the confirmation dialog.
type Summary struct {
	Total        int
	OkCount      int
	Unchanged    int
	Invalid      []string
	Duplicate    []string
	TargetExists []string
	Sidecars     int      // related files that will be renamed along with their primaries
	GroupSkipped []string // skipped because a related file of the same group cannot be renamed
}

// PlanOptions tunes BuildPlan.
type PlanOptions struct {
	// Sidecars also renames related files (same base name, other extension).
	Sidecars    bool
	SidecarExts string   // comma/space separated; empty means DefaultSidecarExts
	All         []string // every file in the folder(s): where related files are searched
}

// caseInsensitiveFS is a best-effort guess: macOS and Windows default to
// case-insensitive file systems, so names differing only by case collide.
var caseInsensitiveFS = runtime.GOOS == "windows" || runtime.GOOS == "darwin"

// DupKey returns the key used to detect two files in one folder that would end
// up with the same name.
func DupKey(dir, name string) string {
	if caseInsensitiveFS {
		name = strings.ToLower(name)
	}
	return dir + "\x00" + name
}

// TargetBlocked reports whether newPath is already occupied by a file other
// than oldPath. A pure case change (IMG.jpg -> img.jpg) on a case-insensitive
// file system resolves to the same file and is therefore NOT blocked.
func TargetBlocked(oldPath, newPath string) bool {
	nfi, err := os.Lstat(newPath)
	if err != nil {
		return false
	}
	if filepath.Dir(oldPath) == filepath.Dir(newPath) &&
		strings.EqualFold(filepath.Base(oldPath), filepath.Base(newPath)) {
		if ofi, err := os.Lstat(oldPath); err == nil && os.SameFile(ofi, nfi) {
			return false
		}
	}
	return true
}

type kind int

const (
	kOK kind = iota
	kUnchanged
	kInvalid
	kDup
	kBlocked
	kGroup
)

// PathKey normalises a path for comparison (case-folded on case-insensitive file systems).
func PathKey(p string) string {
	p = filepath.Clean(p)
	if caseInsensitiveFS {
		p = strings.ToLower(p)
	}
	return p
}

// ResolveTargets skips every StatusOK item whose target name is occupied by a
// file that is NOT itself being renamed away. A target held by a file that
// leaves (chains like frame 1→2, 2→3, or swaps) is fine, because Execute moves
// all files out of the way first. It repeats until stable, since skipping one
// item can make another one's target permanently occupied. onSkip (optional)
// is told which items were skipped. It reports whether anything was skipped.
func ResolveTargets(items []Item, onSkip func(i int)) bool {
	blocked := make([]bool, len(items))
	for i := range items {
		if items[i].Status == StatusOK {
			blocked[i] = TargetBlocked(items[i].OldPath, items[i].NewPath)
		}
	}
	skipped := false
	for {
		leaving := map[string]int{}
		for i := range items {
			if items[i].Status == StatusOK {
				leaving[PathKey(items[i].OldPath)] = i
			}
		}
		progress := false
		for i := range items {
			if items[i].Status != StatusOK || !blocked[i] {
				continue
			}
			if j, ok := leaving[PathKey(items[i].NewPath)]; ok && j != i {
				continue
			}
			items[i].Status, items[i].Reason = StatusSkip, "conflict: target exists on disk"
			if onSkip != nil {
				onSkip(i)
			}
			progress, skipped = true, true
		}
		if !progress {
			return skipped
		}
	}
}

// BuildPlan computes the rename plan for the given files and validates every
// item. Items that cannot be renamed safely are marked StatusSkip with a
// reason. If the steps need file metadata it is loaded first (this may read
// many files, so call BuildPlan off the UI thread). With opt.Sidecars, related
// files are renamed along with their primary file — all or none of a group.
func BuildPlan(files []string, steps []Step, opt PlanOptions) ([]Item, Summary) {
	if need := MetaNeeds(steps); need > meta.None {
		_ = meta.Ensure(context.Background(), files, need, nil)
	}
	names := PreviewNames(files, steps)

	var items []Item
	var kinds []kind
	var ix *SidecarIndex
	var exts map[string]bool
	if opt.Sidecars {
		ix = NewSidecarIndex(opt.All)
		list := opt.SidecarExts
		if strings.TrimSpace(list) == "" {
			list = DefaultSidecarExts
		}
		exts = ParseExts(list)
	}
	selected := make(map[string]bool, len(files))
	for _, p := range files {
		selected[p] = true
	}
	claimed := map[string]string{} // related file -> the new name its first primary gave it
	conflictGroup := map[int]bool{}

	for gi, p := range files {
		g := gi + 1
		dir, oldName, newName := filepath.Dir(p), filepath.Base(p), names[gi]
		items = append(items, Item{
			OldPath: p, NewPath: filepath.Join(dir, newName),
			OldName: oldName, NewName: newName, Status: StatusOK, Group: g,
		})
		kinds = append(kinds, kOK)

		if ix == nil || newName == oldName {
			continue
		}
		for _, r := range ix.For(p, exts) {
			if selected[r.Path] {
				continue // it is a primary file itself and gets its own rename
			}
			rn := relatedNewName(r, oldName, newName)
			if rn == "" {
				continue
			}
			if prev, taken := claimed[r.Path]; taken {
				// shared by two primaries (A.CR2 + A.JPG share A.xmp): fine when
				// both want the same new name, otherwise ambiguous.
				if prev != rn {
					conflictGroup[g] = true
				}
				continue
			}
			claimed[r.Path] = rn
			items = append(items, Item{
				OldPath: r.Path, NewPath: filepath.Join(filepath.Dir(r.Path), rn),
				OldName: filepath.Base(r.Path), NewName: rn, Status: StatusOK, Related: true, Group: g,
			})
			kinds = append(kinds, kOK)
		}
	}

	dup := make(map[string]int, len(items))
	for _, it := range items {
		dup[DupKey(filepath.Dir(it.OldPath), it.NewName)]++
	}
	skip := func(i int, k kind, reason string) {
		items[i].Status, items[i].Reason, kinds[i] = StatusSkip, reason, k
	}
	for i, it := range items {
		switch {
		case it.NewName == it.OldName:
			skip(i, kUnchanged, "unchanged")
		case InvalidReason(it.NewName) != "":
			skip(i, kInvalid, "invalid: "+InvalidReason(it.NewName))
		case dup[DupKey(filepath.Dir(it.OldPath), it.NewName)] > 1:
			skip(i, kDup, "conflict: duplicate preview name")
		case conflictGroup[it.Group] && !it.Related:
			skip(i, kGroup, "conflict: a related file is claimed by another file")
		}
	}

	// Resolve occupied targets and keep every group all-or-nothing.
	for {
		ResolveTargets(items, func(i int) { kinds[i] = kBlocked })
		changed := false
		bad := map[int]string{}
		for i, it := range items {
			if it.Status == StatusSkip && it.Reason != "unchanged" && kinds[i] != kGroup {
				bad[it.Group] = it.OldName
			}
		}
		for i := range items {
			if why, ok := bad[items[i].Group]; ok && items[i].Status == StatusOK {
				skip(i, kGroup, "skipped: related file "+why+" cannot be renamed")
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	sum := Summary{Total: len(files)}
	for i, it := range items {
		pair := fmt.Sprintf("%s → %s", it.OldName, it.NewName)
		switch kinds[i] {
		case kUnchanged:
			if !it.Related {
				sum.Unchanged++
			}
		case kInvalid:
			sum.Invalid = append(sum.Invalid, fmt.Sprintf("%s (%s)", pair, InvalidReason(it.NewName)))
		case kDup:
			sum.Duplicate = append(sum.Duplicate, pair)
		case kBlocked:
			sum.TargetExists = append(sum.TargetExists, pair)
		case kGroup:
			sum.GroupSkipped = append(sum.GroupSkipped, pair)
		default:
			if it.Status == StatusOK {
				sum.OkCount++
				if it.Related {
					sum.Sidecars++
				}
			}
		}
	}
	return items, sum
}

// Execute executes every StatusOK item using a two-phase rename: phase 1 moves
// each file to a unique temporary name, phase 2 moves it to its final name.
// This makes swaps (a→b, b→a) and case-only renames safe. Phase 2 refuses to
// overwrite an existing file. A file that cannot be moved to its final name is
// restored; if even that fails, its TempPath is recorded so it can be reported.
func Execute(plan []Item) []Item {
	out := make([]Item, len(plan))
	copy(out, plan)

	type staged struct {
		idx int
		tmp string
	}
	var phase2 []staged
	ts := time.Now().UnixNano()

	for i := range out {
		if out[i].Status != StatusOK {
			continue
		}
		tmp := filepath.Join(filepath.Dir(out[i].OldPath), fmt.Sprintf(".renforge_tmp_%d_%d", ts, i))
		if err := os.Rename(out[i].OldPath, tmp); err != nil {
			out[i].Status, out[i].Reason = StatusError, err.Error()
			continue
		}
		phase2 = append(phase2, staged{i, tmp})
	}

	for _, s := range phase2 {
		it := &out[s.idx]
		var err error
		if _, statErr := os.Lstat(it.NewPath); statErr == nil {
			err = fmt.Errorf("target appeared on disk: %s", it.NewName)
		} else {
			err = os.Rename(s.tmp, it.NewPath)
		}
		if err == nil {
			it.Status = StatusRenamed
			continue
		}
		it.Status, it.Reason = StatusError, err.Error()
		if rerr := os.Rename(s.tmp, it.OldPath); rerr != nil {
			it.TempPath = s.tmp
			it.Reason += fmt.Sprintf("; could not restore original name, file left at %s", s.tmp)
		}
	}
	return out
}

// Counts tallies items by outcome. Dry-run items count as renamed.
func Counts(items []Item) (renamed, skipped, errors int) {
	for _, it := range items {
		switch it.Status {
		case StatusRenamed, StatusDryRun:
			renamed++
		case StatusSkip:
			skipped++
		case StatusError:
			errors++
		}
	}
	return
}

// Stranded returns the items whose file was left under a temporary name.
func Stranded(items []Item) []Item {
	var s []Item
	for _, it := range items {
		if it.TempPath != "" {
			s = append(s, it)
		}
	}
	return s
}
