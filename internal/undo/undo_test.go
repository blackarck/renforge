package undo

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"renforge/internal/rename"
)

func TestRoundTripAndRevert(t *testing.T) {
	dir := t.TempDir()
	mk := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a, b := mk("a.txt", "A"), mk("b.txt", "B")

	steps := []rename.Step{{Op: rename.OpPrepend, A: "new_"}}
	plan, _ := rename.BuildPlan([]string{a, b}, steps, rename.PlanOptions{})
	done := rename.Execute(plan)

	logPath := filepath.Join(t.TempDir(), "log.csv")
	if err := WriteFile(logPath, done); err != nil {
		t.Fatal(err)
	}
	entries, err := ReadFile(logPath)
	if err != nil || len(entries) != 2 {
		t.Fatalf("entries=%v err=%v", entries, err)
	}

	revert, sum := BuildRevertPlan(entries)
	if sum.Total != 2 || sum.Ready != 2 {
		t.Fatalf("summary %+v", sum)
	}
	out := rename.Execute(revert)
	for _, it := range out {
		if it.Status != rename.StatusRenamed {
			t.Fatalf("revert failed: %+v", it)
		}
	}
	for name, want := range map[string]string{"a.txt": "A", "b.txt": "B"} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(got) != want {
			t.Fatalf("%s: got %q err %v", name, got, err)
		}
	}
}

func TestRevertReportsMissingAndBlocked(t *testing.T) {
	dir := t.TempDir()
	newA := filepath.Join(dir, "new_a.txt")
	newB := filepath.Join(dir, "new_b.txt")
	oldB := filepath.Join(dir, "b.txt")
	for _, p := range []string{newB, oldB} { // new_a missing; b's original name is taken again
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entries := []rename.Item{
		{OldPath: filepath.Join(dir, "a.txt"), NewPath: newA, Status: rename.StatusRenamed},
		{OldPath: oldB, NewPath: newB, Status: rename.StatusRenamed},
		{OldPath: filepath.Join(dir, "c.txt"), NewPath: filepath.Join(dir, "x"), Status: rename.StatusDryRun},
	}
	_, sum := BuildRevertPlan(entries)
	if sum.Total != 2 || sum.Ready != 0 || len(sum.Missing) != 1 || len(sum.Blocked) != 1 {
		t.Fatalf("summary %+v", sum)
	}
}

func TestReadRejectsForeignCSV(t *testing.T) {
	if _, err := Read(strings.NewReader("x,y\n1,2\n")); err == nil {
		t.Fatal("expected header error")
	}
	if _, err := Read(strings.NewReader("")); err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestWriteEscapesCSV(t *testing.T) {
	var buf bytes.Buffer
	items := []rename.Item{{OldPath: `/d/a,"b".txt`, NewPath: "/d/c.txt", Status: rename.StatusRenamed}}
	if err := Write(&buf, items); err != nil {
		t.Fatal(err)
	}
	got, err := Read(&buf)
	if err != nil || len(got) != 1 || got[0].OldPath != items[0].OldPath {
		t.Fatalf("got %+v err %v", got, err)
	}
}

// A frame shift is a chain (1→2, 2→3). Undoing it must work, not report
// "original name is in use".
func TestRevertChainedRenames(t *testing.T) {
	dir := t.TempDir()
	var files []string
	for _, n := range []string{"s.0001.exr", "s.0002.exr", "s.0003.exr"} {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte(n), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}
	plan, _ := rename.BuildPlan(files, []rename.Step{{Op: rename.OpFrames, A: "+1"}}, rename.PlanOptions{})
	done := rename.Execute(plan)

	revert, sum := BuildRevertPlan(done)
	if sum.Total != 3 || sum.Ready != 3 || len(sum.Blocked) != 0 {
		t.Fatalf("summary %+v", sum)
	}
	for _, it := range rename.Execute(revert) {
		if it.Status != rename.StatusRenamed {
			t.Fatalf("%+v", it)
		}
	}
	for _, n := range []string{"s.0001.exr", "s.0002.exr", "s.0003.exr"} {
		got, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil || string(got) != n {
			t.Fatalf("%s: %q %v", n, got, err)
		}
	}
}
