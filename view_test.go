package main

import (
	"strings"
	"testing"

	"FileRenUtil/internal/rename"
)

func newTestState(files []string, steps []rename.Step) *AppState {
	s := &AppState{
		allFiles:      files,
		filteredFiles: files,
		steps:         steps,
		deselected:    map[string]bool{},
		viewMode:      viewAll,
	}
	s.indexPaths()
	recomputePreview(s)
	return s
}

func TestClassify(t *testing.T) {
	files := []string{"/d/a.txt", "/d/b.txt", "/d/keep.txt", "/d/taken.txt", "/d/x1.txt", "/d/x2.txt"}
	// a -> taken (occupied by a file that stays), b -> ok, x1/x2 -> same name, keep unchanged
	s := newTestState(files, []rename.Step{
		{Op: rename.OpReplaceText, A: "a.txt", B: "taken.txt"},
		{Op: rename.OpReplaceText, A: "b.txt", B: "b2.txt"},
		{Op: rename.OpReplaceText, A: "x1", B: "x"},
		{Op: rename.OpReplaceText, A: "x2", B: "x"},
	})
	want := map[string][2]bool{ // changed, problem
		"/d/a.txt":     {true, true},  // target taken by a file that does not leave
		"/d/b.txt":     {true, false}, // fine
		"/d/keep.txt":  {false, false},
		"/d/taken.txt": {false, false},
		"/d/x1.txt":    {true, true}, // duplicate preview name
		"/d/x2.txt":    {true, true},
	}
	for p, w := range want {
		if c, pr := s.classify(p); c != w[0] || pr != w[1] {
			t.Errorf("%s: changed=%v problem=%v, want %v", p, c, pr, w)
		}
	}
}

func TestClassifyChainAndInvalidAndDeselected(t *testing.T) {
	files := []string{"/d/s.0001.exr", "/d/s.0002.exr", "/d/s.0003.exr"}
	s := newTestState(files, []rename.Step{{Op: rename.OpFrames, A: "+1"}})
	for _, p := range files { // every target is held by a file that is itself leaving, except nothing outside
		if c, pr := s.classify(p); !c || pr {
			t.Errorf("%s: chain member changed=%v problem=%v", p, c, pr)
		}
	}
	// deselect the middle file: the first file's target is now held by a file that stays
	s.deselected["/d/s.0002.exr"] = true
	recomputePreview(s)
	if c, pr := s.classify("/d/s.0001.exr"); !c || !pr {
		t.Errorf("after deselecting the occupant: changed=%v problem=%v, want true,true", c, pr)
	}
	if c, pr := s.classify("/d/s.0002.exr"); c || pr {
		t.Error("deselected file must be neither changed nor a problem")
	}

	inv := newTestState([]string{"/d/a.txt"}, []rename.Step{{Op: rename.OpReplaceText, A: "a", B: "a:"}})
	if c, pr := inv.classify("/d/a.txt"); !c || !pr {
		t.Error("invalid name must be a problem")
	}
}

func TestRebuildVisibleModes(t *testing.T) {
	files := []string{"/d/a.txt", "/d/b.txt", "/d/c.txt", "/d/taken.txt"}
	s := newTestState(files, []rename.Step{
		{Op: rename.OpReplaceText, A: "a.txt", B: "taken.txt"}, // problem
		{Op: rename.OpReplaceText, A: "b.txt", B: "b2.txt"},    // changed ok
	})
	rebuildVisible(s)
	if len(s.visible) != 4 || s.nChanged != 2 || s.nProblems != 1 {
		t.Fatalf("all: visible=%d changed=%d problems=%d", len(s.visible), s.nChanged, s.nProblems)
	}
	s.viewMode = viewChanged
	rebuildVisible(s)
	if len(s.visible) != 2 {
		t.Fatalf("changed view: %v", s.visible)
	}
	s.viewMode = viewProblems
	rebuildVisible(s)
	if len(s.visible) != 1 || s.visible[0] != "/d/a.txt" {
		t.Fatalf("problems view: %v", s.visible)
	}
	// counts do not depend on the view
	if s.nChanged != 2 || s.nProblems != 1 {
		t.Fatalf("counts changed with the view: %d %d", s.nChanged, s.nProblems)
	}
}

func TestDiffRunes(t *testing.T) {
	changedOf := func(parts []diffPart) []string {
		var out []string
		for _, p := range parts {
			if p.changed {
				out = append(out, p.text)
			}
		}
		return out
	}
	cases := []struct {
		orig, name string
		changed    []string
	}{
		{"IMG_0042.JPG", "Paris_0042.jpg", []string{"Paris", "jpg"}},
		{"beach.jpg", "2024_beach.jpg", []string{"2024_"}},
		{"cut.mp4", "cut_final.mp4", []string{"_final"}},
		{"a.jpeg", "a.png", []string{"pn"}}, // the shared trailing "g" stays plain
		{"same.txt", "same.txt", nil},
		{"report (2).pdf", "report.pdf", nil}, // pure deletion: nothing new to highlight
		{"x", "yz", []string{"yz"}},
		{"café.txt", "cafe.txt", []string{"e"}},
		{"my file.txt", "my_file.txt", []string{"_"}},
	}
	for _, c := range cases {
		parts := diffRunes(c.orig, c.name)
		rebuilt := ""
		for _, p := range parts {
			rebuilt += p.text
		}
		if rebuilt != c.name {
			t.Errorf("%q→%q: parts rebuild %q", c.orig, c.name, rebuilt)
		}
		got := changedOf(parts)
		if len(got) != len(c.changed) {
			t.Errorf("%q→%q: changed %q, want %q", c.orig, c.name, got, c.changed)
			continue
		}
		for i := range got {
			if got[i] != c.changed[i] {
				t.Errorf("%q→%q: changed %q, want %q", c.orig, c.name, got, c.changed)
			}
		}
	}
	// very long names fall back to highlighting the whole differing middle (no blow-up)
	long1, long2 := strings.Repeat("a", 600), strings.Repeat("b", 600)
	if parts := diffRunes(long1, long2); len(parts) != 1 || !parts[0].changed {
		t.Errorf("long fallback: %v", parts)
	}
}

func TestKindCountsAndProblemFilter(t *testing.T) {
	files := []string{"/d/a.txt", "/d/b.txt", "/d/c.txt", "/d/keep.txt", "/d/taken.txt", "/d/x1.txt", "/d/x2.txt"}
	s := newTestState(files, []rename.Step{
		{Op: rename.OpReplaceText, A: "a.txt", B: "taken.txt"}, // taken
		{Op: rename.OpReplaceText, A: "b.txt", B: "b:.txt"},    // invalid
		{Op: rename.OpReplaceText, A: "c.txt", B: "c2.txt"},    // ok
		{Op: rename.OpReplaceText, A: "x1", B: "x"},            // duplicate pair
		{Op: rename.OpReplaceText, A: "x2", B: "x"},
	})
	s.deselected["/d/keep.txt"] = true
	s.deselected["/d/taken.txt"] = true // stays where it is, so a.txt -> taken.txt is blocked
	recomputePreview(s)
	rebuildVisible(s)
	c := s.kindCounts
	if c[rowOK] != 1 || c[rowInvalid] != 1 || c[rowDup] != 2 || c[rowTaken] != 1 || c[rowUnchanged] != 0 || c[rowNone] != 2 {
		t.Fatalf("counts %v", c)
	}
	if s.nProblems != 4 || s.nChanged != 5 {
		t.Fatalf("nProblems=%d nChanged=%d", s.nProblems, s.nChanged)
	}
	s.viewMode = viewProblems
	s.problemKind = rowDup
	rebuildVisible(s)
	if len(s.visible) != 2 {
		t.Fatalf("duplicate-only view: %v", s.visible)
	}
	s.problemKind = rowNone
	rebuildVisible(s)
	if len(s.visible) != 4 {
		t.Fatalf("all problems view: %v", s.visible)
	}
}
