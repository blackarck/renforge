package main

import (
	"path/filepath"

	"renforge/internal/rename"
)

// Which rows the preview list shows.
const (
	viewAll      = "All"
	viewChanged  = "Changed"
	viewProblems = "Problems"
)

// rowKind classifies a file for the preview, the summary strip and the Show filter.
type rowKind int

const (
	rowNone      rowKind = iota // not selected, or no preview yet
	rowUnchanged                // selected, name stays the same
	rowOK                       // will be renamed
	rowInvalid                  // new name is not allowed
	rowDup                      // another selected file gets the same name
	rowTaken                    // name is held by a file that is not being renamed
)

func (k rowKind) isProblem() bool { return k >= rowInvalid }

// classifyKind says what will happen to a file. It uses only data in memory, so
// it is cheap for huge folders; the authoritative check against the disk still
// happens when you press Apply.
func (s *AppState) classifyKind(full string) rowKind {
	if s.deselected[full] {
		return rowNone
	}
	name, ok := s.previewNames[full]
	if !ok {
		return rowNone
	}
	if name == filepath.Base(full) {
		return rowUnchanged
	}
	dir := filepath.Dir(full)
	switch {
	case rename.InvalidReason(name) != "":
		return rowInvalid
	case s.previewCounts[rename.DupKey(dir, name)] > 1:
		return rowDup
	}
	key := rename.PathKey(filepath.Join(dir, name))
	if key == rename.PathKey(full) {
		return rowOK // case-only change of the same file
	}
	if occupant, taken := s.pathIndex[key]; taken {
		// fine if the occupying file is itself being renamed away
		if occName, selected := s.previewNames[occupant]; selected && occName != filepath.Base(occupant) {
			return rowOK
		}
		return rowTaken
	}
	return rowOK
}

// classify reports whether a file will change and whether that has a problem.
func (s *AppState) classify(full string) (changed, problem bool) {
	k := s.classifyKind(full)
	return k >= rowOK, k.isProblem()
}

// rebuildVisible recomputes the rows shown for the current view and the counts
// used by the view buttons and the summary strip.
func rebuildVisible(s *AppState) {
	var counts [rowTaken + 1]int
	s.visible = s.visible[:0]
	for _, p := range s.filteredFiles {
		k := s.classifyKind(p)
		counts[k]++
		show := true
		switch s.viewMode {
		case viewChanged:
			show = k >= rowOK
		case viewProblems:
			show = k.isProblem() && (s.problemKind == rowNone || k == s.problemKind)
		}
		if show {
			s.visible = append(s.visible, p)
		}
	}
	s.kindCounts = counts
	s.nProblems = counts[rowInvalid] + counts[rowDup] + counts[rowTaken]
	s.nChanged = counts[rowOK] + s.nProblems
}

// indexPaths builds the in-memory index of every scanned file used by classify.
func (s *AppState) indexPaths() {
	s.pathIndex = make(map[string]string, len(s.allFiles))
	for _, p := range s.allFiles {
		s.pathIndex[rename.PathKey(p)] = p
	}
}

// diffPart is a piece of the new name; changed pieces are highlighted.
type diffPart struct {
	text    string
	changed bool
}

// diffRunes compares the new name with the original character by character
// (longest common subsequence) and returns the new name in pieces, marking the
// characters that are not in the original. Very short matches next to a
// change are folded into it so the highlight is not choppy.
func diffRunes(orig, name string) []diffPart {
	if orig == name {
		return []diffPart{{text: name}}
	}
	o, n := []rune(orig), []rune(name)
	p := 0
	for p < len(o) && p < len(n) && o[p] == n[p] {
		p++
	}
	q := 0
	for q < len(o)-p && q < len(n)-p && o[len(o)-1-q] == n[len(n)-1-q] {
		q++
	}
	om, nm := o[p:len(o)-q], n[p:len(n)-q]

	changed := make([]bool, len(nm))
	for i := range changed {
		changed[i] = true
	}
	if len(om) > 0 && len(nm) > 0 && len(om)*len(nm) <= 250000 {
		// dp[i][j] = length of the LCS of om[i:] and nm[j:]
		dp := make([][]int32, len(om)+1)
		for i := range dp {
			dp[i] = make([]int32, len(nm)+1)
		}
		for i := len(om) - 1; i >= 0; i-- {
			for j := len(nm) - 1; j >= 0; j-- {
				if om[i] == nm[j] {
					dp[i][j] = dp[i+1][j+1] + 1
				} else if dp[i+1][j] >= dp[i][j+1] {
					dp[i][j] = dp[i+1][j]
				} else {
					dp[i][j] = dp[i][j+1]
				}
			}
		}
		for i, j := 0, 0; i < len(om) && j < len(nm); {
			switch {
			case om[i] == nm[j]:
				changed[j] = false
				i++
				j++
			case dp[i+1][j] >= dp[i][j+1]:
				i++
			default:
				j++
			}
		}
		// fold short unchanged runs (1-2 characters) that touch a changed
		// character into the change, so the highlight is not choppy
		for j := 0; j < len(nm); {
			if changed[j] {
				j++
				continue
			}
			k := j
			for k < len(nm) && !changed[k] {
				k++
			}
			if k-j <= 2 && ((j > 0 && changed[j-1]) || (k < len(nm) && changed[k])) {
				for x := j; x < k; x++ {
					changed[x] = true
				}
			}
			j = k
		}
	}

	var parts []diffPart
	add := func(text []rune, ch bool) {
		if len(text) == 0 {
			return
		}
		if k := len(parts) - 1; k >= 0 && parts[k].changed == ch {
			parts[k].text += string(text)
			return
		}
		parts = append(parts, diffPart{string(text), ch})
	}
	add(n[:p], false)
	for j, r := range nm {
		add([]rune{r}, changed[j])
	}
	add(n[len(n)-q:], false)
	return parts
}
