package rename

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestSplitExt(t *testing.T) {
	cases := []struct{ in, base, ext string }{
		{"photo.jpg", "photo", ".jpg"},
		{"archive.tar.gz", "archive", ".tar.gz"},
		{"ARCHIVE.TAR.GZ", "ARCHIVE", ".TAR.GZ"},
		{".bashrc", ".bashrc", ""},
		{"noext", "noext", ""},
		{"a.b.c", "a.b", ".c"},
		{".tar.gz", ".tar", ".gz"}, // nothing left for a base, fall back to simple ext
	}
	for _, c := range cases {
		b, e := SplitExt(c.in)
		if b != c.base || e != c.ext {
			t.Errorf("SplitExt(%q) = (%q,%q), want (%q,%q)", c.in, b, e, c.base, c.ext)
		}
	}
}

func TestApplySteps(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		steps []Step
		want  string
	}{
		{"no steps", "a.txt", nil, "a.txt"},
		{"remove", "vivek_pic.jpg", []Step{{Op: OpRemoveText, A: "vivek_"}}, "pic.jpg"},
		{"replace", "a b c.txt", []Step{{Op: OpReplaceText, A: " ", B: "_"}}, "a_b_c.txt"},
		{"replace empty find is no-op", "a.txt", []Step{{Op: OpReplaceText, A: "", B: "x"}}, "a.txt"},
		{"insert before ext", "a.png", []Step{{Op: OpInsertBeforeExt, A: "_v2"}}, "a_v2.png"},
		{"insert keeps tar.gz", "a.tar.gz", []Step{{Op: OpInsertBeforeExt, A: "_v2"}}, "a_v2.tar.gz"},
		{"append", "a.png", []Step{{Op: OpAppend, A: "!"}}, "a!.png"},
		{"prepend", "a.png", []Step{{Op: OpPrepend, A: "NEW_"}}, "NEW_a.png"},
		{"prepend dotfile", ".env", []Step{{Op: OpPrepend, A: "x"}}, "x.env"},
		{"change ext no dot", "a.png", []Step{{Op: OpChangeExt, A: "xyz"}}, "a.xyz"},
		{"change ext with dot", "a.png", []Step{{Op: OpChangeExt, A: ".xyz"}}, "a.xyz"},
		{"change ext to none", "a.png", []Step{{Op: OpChangeExt, A: ""}}, "a"},
		{"change ext adds one", "a", []Step{{Op: OpChangeExt, A: "txt"}}, "a.txt"},
		{"change compound ext", "a.tar.gz", []Step{{Op: OpChangeExt, A: "zip"}}, "a.zip"},
		{"order matters", "a.png", []Step{{Op: OpReplaceText, A: "a", B: "b"}, {Op: OpReplaceText, A: "b", B: "c"}}, "c.png"},
		{"no silent trim", "a.png", []Step{{Op: OpPrepend, A: " "}}, " a.png"},
	}
	for _, c := range cases {
		if got := ApplySteps(c.in, c.steps); got != c.want {
			t.Errorf("%s: ApplySteps(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestInvalidReason(t *testing.T) {
	bad := map[string]string{
		"":                       "empty name",
		"   ":                    "empty name",
		".":                      "reserved name",
		"a:b.txt":                "invalid characters",
		"a?b":                    "invalid characters",
		"a\x01b":                 "control characters",
		" lead.txt":              "starts with a space",
		"trail.txt ":             "ends with a space or dot",
		"trail.":                 "ends with a space or dot",
		"CON":                    "reserved filename",
		"con.txt":                "reserved filename",
		"NUL.tar.gz":             "reserved filename",
		"lpt1.log":               "reserved filename",
		strings.Repeat("a", 256): "name longer than 255 bytes",
	}
	for name, want := range bad {
		if got := InvalidReason(name); got != want {
			t.Errorf("InvalidReason(%q) = %q, want %q", name, got, want)
		}
	}
	for _, ok := range []string{"a.txt", "console.txt", ".bashrc", "my file (1).jpg", "CONX", "résumé.pdf"} {
		if got := InvalidReason(ok); got != "" {
			t.Errorf("InvalidReason(%q) = %q, want valid", ok, got)
		}
	}
}

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

func TestBuildPlanStatuses(t *testing.T) {
	dir := t.TempDir()
	same := write(t, dir, "same.txt", "1")
	a := write(t, dir, "a.txt", "a")
	write(t, dir, "taken.txt", "x") // occupies a rename target
	b := write(t, dir, "b_.txt", "b")
	c := write(t, dir, "c.txt", "c")
	d := write(t, dir, "d.txt", "d")

	steps := []Step{{Op: OpReplaceText, A: "a.txt", B: "taken.txt"}, {Op: OpReplaceText, A: "b_", B: "b"}}
	// a.txt -> taken.txt (blocked), b_.txt -> b.txt (ok), same.txt unchanged.
	items, sum := BuildPlan([]string{same, a, b}, steps, PlanOptions{})
	if sum.Unchanged != 1 || sum.OkCount != 1 || len(sum.TargetExists) != 1 {
		t.Fatalf("unexpected summary: %+v", sum)
	}
	if items[0].Status != StatusSkip || items[1].Status != StatusSkip || items[2].Status != StatusOK {
		t.Fatalf("unexpected statuses: %+v", items)
	}

	// duplicates: c.txt and d.txt both become "same2.txt"
	_, sum = BuildPlan([]string{c, d}, []Step{{Op: OpRemoveText, A: "c"}, {Op: OpRemoveText, A: "d"}, {Op: OpPrepend, A: "same2"}}, PlanOptions{})
	if len(sum.Duplicate) != 2 || sum.OkCount != 0 {
		t.Fatalf("expected 2 duplicate conflicts, got %+v", sum)
	}

	// invalid
	_, sum = BuildPlan([]string{a}, []Step{{Op: OpReplaceText, A: "a", B: "a:"}}, PlanOptions{})
	if len(sum.Invalid) != 1 {
		t.Fatalf("expected invalid, got %+v", sum)
	}
}

func TestExecuteSwap(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.txt", "A")
	b := write(t, dir, "b.txt", "B")
	plan := []Item{
		{OldPath: a, NewPath: b, OldName: "a.txt", NewName: "b.txt", Status: StatusOK},
		{OldPath: b, NewPath: a, OldName: "b.txt", NewName: "a.txt", Status: StatusOK},
	}
	out := Execute(plan)
	for _, it := range out {
		if it.Status != StatusRenamed {
			t.Fatalf("expected renamed, got %+v", it)
		}
	}
	if read(t, a) != "B" || read(t, b) != "A" {
		t.Fatalf("swap failed: a=%q b=%q", read(t, a), read(t, b))
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatalf("temp files left behind: %v", entries)
	}
}

func TestExecuteCaseOnlyRename(t *testing.T) {
	dir := t.TempDir()
	old := write(t, dir, "IMG.jpg", "x")
	items, sum := BuildPlan([]string{old}, []Step{{Op: OpReplaceText, A: "IMG", B: "img"}}, PlanOptions{})
	if sum.OkCount != 1 {
		t.Fatalf("case-only rename must be allowed, got %+v (%+v)", sum, items)
	}
	out := Execute(items)
	if out[0].Status != StatusRenamed {
		t.Fatalf("expected renamed, got %+v", out[0])
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "img.jpg" {
		t.Fatalf("directory listing = %v, want only img.jpg", entries)
	}
}

func TestExecuteDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.txt", "A")
	target := write(t, dir, "b.txt", "B")
	// Plan claims OK even though target exists (race: it appeared after planning).
	out := Execute([]Item{{OldPath: a, NewPath: target, OldName: "a.txt", NewName: "b.txt", Status: StatusOK}})
	if out[0].Status != StatusError {
		t.Fatalf("expected error, got %+v", out[0])
	}
	if read(t, a) != "A" || read(t, target) != "B" {
		t.Fatal("files must be untouched when the target exists")
	}
	if len(Stranded(out)) != 0 {
		t.Fatal("nothing should be stranded")
	}
}

func TestExecuteReportsStrandedFile(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.txt", "A")
	// Target directory does not exist -> phase 2 fails; restore succeeds.
	bad := filepath.Join(dir, "missing", "b.txt")
	out := Execute([]Item{{OldPath: a, NewPath: bad, OldName: "a.txt", NewName: "b.txt", Status: StatusOK}})
	if out[0].Status != StatusError || out[0].TempPath != "" {
		t.Fatalf("expected restored error, got %+v", out[0])
	}
	if !exists(a) {
		t.Fatal("original must be restored")
	}
}

func TestListFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "b.txt", "")
	write(t, dir, "a.txt", "")
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, sub, "c.txt", "")

	flat, err := ListFiles(context.Background(), dir, false, nil)
	if err != nil || len(flat) != 2 || filepath.Base(flat[0]) != "a.txt" {
		t.Fatalf("flat = %v, err = %v", flat, err)
	}
	deep, err := ListFiles(context.Background(), dir, true, nil)
	if err != nil || len(deep) != 3 {
		t.Fatalf("deep = %v, err = %v", deep, err)
	}
	if _, err := ListFiles(context.Background(), filepath.Join(dir, "nope"), true, nil); err == nil {
		t.Fatal("expected error for missing root")
	}
}

func TestNewSteps(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		steps []Step
		idx   int
		want  string
	}{
		{"regex groups", "IMG_2024_0042.jpg", []Step{{Op: OpRegex, A: `IMG_(\d{4})_(\d+)`, B: "$1-$2"}}, 0, "2024-0042.jpg"},
		{"regex invalid is no-op", "a.txt", []Step{{Op: OpRegex, A: `(`, B: "x"}}, 0, "a.txt"},
		{"regex empty is no-op", "a.txt", []Step{{Op: OpRegex, A: "", B: "x"}}, 0, "a.txt"},
		{"disabled step skipped", "a.txt", []Step{{Op: OpPrepend, A: "x", Disabled: true}}, 0, "a.txt"},
		{"lower", "My FILE.TXT", []Step{{Op: OpCase, A: CaseLower}}, 0, "my file.TXT"},
		{"upper", "my file.txt", []Step{{Op: OpCase, A: CaseUpper}}, 0, "MY FILE.txt"},
		{"title", "the don't STOP-me_now.mp3", []Step{{Op: OpCase, A: CaseTitle}}, 0, "The Don't Stop-Me_Now.mp3"},
		{"sentence", "hELLO wORLD.txt", []Step{{Op: OpCase, A: CaseSentence}}, 0, "Hello world.txt"},
		{"snake", "My Great-Photo (final).JPG", []Step{{Op: OpCase, A: CaseSnake}}, 0, "my_great_photo_final.JPG"},
		{"snake camel", "myGreatPhoto.jpg", []Step{{Op: OpCase, A: CaseSnake}}, 0, "my_great_photo.jpg"},
		{"kebab", "My Great Photo.jpg", []Step{{Op: OpCase, A: CaseKebab}}, 0, "my-great-photo.jpg"},
		{"kebab symbols only", "___.jpg", []Step{{Op: OpCase, A: CaseKebab}}, 0, "___.jpg"},
		{"ext lower", "A.JPG", []Step{{Op: OpCase, A: CaseExtLower}}, 0, "A.jpg"},
		{"ext upper", "a.jpg", []Step{{Op: OpCase, A: CaseExtUpper}}, 0, "a.JPG"},
		{"cleanup spaces", "a   b    c.txt", []Step{{Op: OpCleanup, A: CleanSpaces}}, 0, "a b c.txt"},
		{"cleanup trim", "  a b  .txt", []Step{{Op: OpCleanup, A: CleanTrim}}, 0, "a b.txt"},
		{"cleanup accents", "Café Crème.txt", []Step{{Op: OpCleanup, A: CleanAccents}}, 0, "Cafe Creme.txt"},
		{"cleanup dots", "My.Cool_Song.mp3", []Step{{Op: OpCleanup, A: CleanDots}}, 0, "My Cool Song.mp3"},
		{"cleanup copy (1)", "report (2).pdf", []Step{{Op: OpCleanup, A: CleanCopies}}, 0, "report.pdf"},
		{"cleanup copy -Copy", "report - Copy (3).pdf", []Step{{Op: OpCleanup, A: CleanCopies}}, 0, "report.pdf"},
		{"cleanup copy of", "Copy of report.pdf", []Step{{Op: OpCleanup, A: CleanCopies}}, 0, "report.pdf"},
		{"number default", "beach.jpg", []Step{{Op: OpNumber}}, 4, "beach_005.jpg"},
		{"number template", "beach.jpg", []Step{{Op: OpNumber, A: "Trip_{n:4}"}}, 0, "Trip_0001.jpg"},
		{"number start,step", "x.png", []Step{{Op: OpNumber, A: "{n}_{name}", B: "100,10"}}, 2, "120_x.png"},
		{"number no pad", "x.png", []Step{{Op: OpNumber, A: "f{n}"}}, 9, "f10.png"},
		{"number keeps tar.gz", "x.tar.gz", []Step{{Op: OpNumber, A: "b{n:2}"}}, 0, "b01.tar.gz"},
	}
	for _, c := range cases {
		if got := ApplyStepsAt(c.in, c.steps, c.idx); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestValidateStep(t *testing.T) {
	bad := []Step{
		{Op: OpRegex, A: "("},
		{Op: OpNumber, B: "abc"},
		{Op: OpNumber, B: "1,0"},
		{Op: OpNumber, B: "1,2,3"},
		{Op: OpNumber, A: "photo"},
	}
	for _, s := range bad {
		if ValidateStep(s) == "" {
			t.Errorf("ValidateStep(%+v) should report a problem", s)
		}
	}
	good := []Step{{Op: OpRegex, A: `\d+`}, {Op: OpNumber, A: "{n}", B: "5,-1"}, {Op: OpNumber}, {Op: OpReplaceText}}
	for _, s := range good {
		if msg := ValidateStep(s); msg != "" {
			t.Errorf("ValidateStep(%+v) = %q", s, msg)
		}
	}
}

func TestNaturalLess(t *testing.T) {
	in := []string{"img10.jpg", "IMG2.jpg", "img1.jpg", "img02.jpg", "a/img3.jpg", "img100.jpg"}
	want := []string{"a/img3.jpg", "img1.jpg", "IMG2.jpg", "img02.jpg", "img10.jpg", "img100.jpg"}
	sort.Slice(in, func(i, j int) bool { return NaturalLess(in[i], in[j]) })
	for i := range want {
		if in[i] != want[i] {
			t.Fatalf("got %v, want %v", in, want)
		}
	}
}

func TestListFilesCancelled(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.txt", "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ListFiles(ctx, dir, true, nil); err == nil {
		t.Fatal("expected context error")
	}
	if _, err := ListFiles(ctx, dir, false, nil); err == nil {
		t.Fatal("expected context error (flat)")
	}
}

func TestBuildPlanNumbersBySelectionOrder(t *testing.T) {
	dir := t.TempDir()
	a, b := write(t, dir, "a.jpg", "a"), write(t, dir, "b.jpg", "b")
	items, sum := BuildPlan([]string{a, b}, []Step{{Op: OpNumber, A: "p{n:2}", B: "7"}}, PlanOptions{})
	if sum.OkCount != 2 || items[0].NewName != "p07.jpg" || items[1].NewName != "p08.jpg" {
		t.Fatalf("%+v %+v", sum, items)
	}
}
