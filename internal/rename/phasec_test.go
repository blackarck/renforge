package rename

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"renforge/internal/meta"
)

func TestShiftFrames(t *testing.T) {
	cases := []struct {
		in, off, pad, want string
	}{
		{"shot.0001.exr", "+1000", "", "shot.1001.exr"},
		{"shot_0099.exr", "1", "", "shot_0100.exr"},
		{"shot.0042.exr", "-41", "", "shot.0001.exr"},
		{"shot.1.exr", "9", "4", "shot.0010.exr"},
		{"shot.0001.exr", "-5", "", "shot.0001.exr"},          // would go negative: unchanged
		{"shot_010_0005.exr", "100", "", "shot_010_0105.exr"}, // only the last number
		{"noframes.exr", "5", "", "noframes.exr"},
		{"a2b3.txt", "1", "", "a2b4.txt"},
		{"shot.0001.tar.gz", "1", "", "shot.0002.tar.gz"},
		{"shot.0001.exr", "abc", "", "shot.0001.exr"}, // invalid offset: unchanged
	}
	for _, c := range cases {
		got := ApplySteps(c.in, []Step{{Op: OpFrames, A: c.off, B: c.pad}})
		if got != c.want {
			t.Errorf("frames(%q,%q,%q) = %q, want %q", c.in, c.off, c.pad, got, c.want)
		}
	}
	for _, s := range []Step{{Op: OpFrames, A: "x"}, {Op: OpFrames, A: "1", B: "0"}, {Op: OpFrames, A: "1", B: "99"}} {
		if ValidateStep(s) == "" {
			t.Errorf("expected a validation message for %+v", s)
		}
	}
	if m := ValidateStep(Step{Op: OpFrames, A: "+1000", B: "4"}); m != "" {
		t.Errorf("unexpected: %s", m)
	}
}

func TestTemplateTokens(t *testing.T) {
	taken := time.Date(2024, 6, 1, 14, 5, 9, 0, time.UTC)
	mod := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	ctx := FileCtx{
		Path: filepath.Join("/photos", "Italy", "IMG_1.CR2"), Index: 4, FolderIndex: 1, HasMeta: true,
		Meta: meta.Info{Level: meta.EXIF, Taken: taken, ModTime: mod, Model: "EOS R5", Make: "Canon", Lens: "RF50", ISO: 400},
	}
	cases := []struct {
		name string
		step Step
		want string
	}{
		{"date default", Step{Op: OpTemplate, A: "{date}_{n:3}"}, "2024-06-01_005.CR2"},
		{"date format", Step{Op: OpTemplate, A: "{date:YYYYMMDD_HHmmss}"}, "20240601_140509.CR2"},
		{"short year", Step{Op: OpTemplate, A: "{date:YY-MM-DD}"}, "24-06-01.CR2"},
		{"modified", Step{Op: OpTemplate, A: "{modified:YYYY}_{n}"}, "2020_5.CR2"},
		{"camera etc", Step{Op: OpTemplate, A: "{make}-{camera}-{lens}-ISO{iso}"}, "Canon-EOS R5-RF50-ISO400.CR2"},
		{"parent and name", Step{Op: OpTemplate, A: "{parent}_{name}"}, "Italy_IMG_1.CR2"},
		{"start,step", Step{Op: OpTemplate, A: "{n:2}", B: "10,5"}, "30.CR2"},
		{"per folder", Step{Op: OpTemplate, A: "{n:2}", Flag: true}, "02.CR2"},
		{"default template", Step{Op: OpTemplate}, "2024-06-01_005.CR2"},
		{"unknown token kept", Step{Op: OpTemplate, A: "{foo}_{n}"}, "{foo}_5.CR2"},
	}
	for _, c := range cases {
		if got := ApplyStepsCtx("IMG_1.CR2", []Step{c.step}, ctx); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}

	// without EXIF the date falls back to the modification time; the camera is "unknown"
	ctx.Meta = meta.Info{Level: meta.EXIF, ModTime: mod}
	if got := ApplyStepsCtx("a.jpg", []Step{{Op: OpTemplate, A: "{date}_{camera}_{iso}"}}, ctx); got != "2020-01-02_unknown_unknown.jpg" {
		t.Errorf("fallback: %q", got)
	}
	// metadata not loaded yet
	if got := ApplyStepsCtx("a.jpg", []Step{{Op: OpTemplate, A: "{date}_{n}"}}, FileCtx{}); got != "…_1.jpg" {
		t.Errorf("not loaded: %q", got)
	}
}

func TestMetaNeedsAndValidate(t *testing.T) {
	if MetaNeeds(nil) != meta.None || MetaNeeds([]Step{{Op: OpNumber, A: "{name}_{n}"}}) != meta.None {
		t.Error("plain numbering needs no metadata")
	}
	if MetaNeeds([]Step{{Op: OpTemplate, A: "{modified}_{n}"}}) != meta.Stat {
		t.Error("{modified} needs Stat")
	}
	if MetaNeeds([]Step{{Op: OpTemplate}}) != meta.EXIF { // default template uses {date}
		t.Error("default template needs EXIF")
	}
	if MetaNeeds([]Step{{Op: OpTemplate, A: "{camera}_{n}", Disabled: true}}) != meta.None {
		t.Error("disabled steps need nothing")
	}
	if ValidateStep(Step{Op: OpTemplate, A: "{bogus}_{n}"}) == "" || ValidateStep(Step{Op: OpTemplate, A: "{date}"}) == "" {
		t.Error("expected token / missing {n} messages")
	}
	if ValidateStep(Step{Op: OpTemplate, A: "{date:YYYY}_{n:3}"}) != "" {
		t.Error("valid template rejected")
	}
}

func TestPreviewNamesFolderIndexAndMeta(t *testing.T) {
	meta.Reset()
	dir := t.TempDir()
	sub := filepath.Join(dir, "b")
	os.Mkdir(sub, 0o755)
	a1, a2 := write(t, dir, "a1.jpg", ""), write(t, dir, "a2.jpg", "")
	b1 := write(t, sub, "b1.jpg", "")
	mod := time.Date(2019, 5, 6, 7, 8, 9, 0, time.UTC)
	for _, p := range []string{a1, a2, b1} {
		os.Chtimes(p, mod, mod)
	}
	steps := []Step{{Op: OpTemplate, A: "{modified:YYYY}_{n:2}", Flag: true}}
	files := []string{a1, a2, b1}

	// before metadata is loaded the year is a placeholder
	if got := PreviewNames(files, steps)[0]; got != "…_01.jpg" {
		t.Fatalf("before load: %q", got)
	}
	if err := meta.Ensure(t.Context(), files, MetaNeeds(steps), nil); err != nil {
		t.Fatal(err)
	}
	got := PreviewNames(files, steps)
	want := []string{"2019_01.jpg", "2019_02.jpg", "2019_01.jpg"} // numbering restarts in folder b
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	es, _ := os.ReadDir(dir)
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

func TestChainedFrameShiftIsAllowed(t *testing.T) {
	dir := t.TempDir()
	var files []string
	for _, n := range []string{"shot.0001.exr", "shot.0002.exr", "shot.0003.exr"} {
		files = append(files, write(t, dir, n, n))
	}
	steps := []Step{{Op: OpFrames, A: "+1"}}
	items, sum := BuildPlan(files, steps, PlanOptions{})
	if sum.OkCount != 3 || len(sum.TargetExists) != 0 {
		t.Fatalf("chain must be allowed: %+v", sum)
	}
	out := Execute(items)
	for _, it := range out {
		if it.Status != StatusRenamed {
			t.Fatalf("%+v", it)
		}
	}
	if got := strings.Join(names(t, dir), ","); got != "shot.0002.exr,shot.0003.exr,shot.0004.exr" {
		t.Fatalf("got %s", got)
	}
	if read(t, filepath.Join(dir, "shot.0004.exr")) != "shot.0003.exr" || read(t, filepath.Join(dir, "shot.0002.exr")) != "shot.0001.exr" {
		t.Fatal("contents moved to the wrong names")
	}
}

func TestChainBlockedByForeignFileSkipsWholeChain(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "s.0001.exr", "")
	b := write(t, dir, "s.0002.exr", "")
	write(t, dir, "s.0003.exr", "keep me") // not selected: occupies the end of the chain
	items, sum := BuildPlan([]string{a, b}, []Step{{Op: OpFrames, A: "+1"}}, PlanOptions{})
	if sum.OkCount != 0 || len(sum.TargetExists) != 2 {
		t.Fatalf("whole chain must be skipped: %+v", sum)
	}
	for _, it := range Execute(items) {
		if it.Status == StatusRenamed {
			t.Fatal("nothing may be renamed")
		}
	}
	if len(names(t, dir)) != 3 {
		t.Fatal("files changed unexpectedly")
	}
}

func TestSidecarsRenamedTogether(t *testing.T) {
	dir := t.TempDir()
	raw := write(t, dir, "IMG_001.CR2", "raw")
	write(t, dir, "IMG_001.xmp", "x1")
	write(t, dir, "IMG_001.CR2.xmp", "x2") // Adobe style
	write(t, dir, "IMG_001.JPG", "jpg")
	write(t, dir, "IMG_0010.xmp", "other") // different base: must not be touched
	write(t, dir, "IMG_001.txt", "txt")    // extension not in the list
	all, _ := ListFiles(t.Context(), dir, false, nil)

	steps := []Step{{Op: OpPrepend, A: "Trip_"}}
	items, sum := BuildPlan([]string{raw}, steps, PlanOptions{Sidecars: true, All: all})
	if sum.OkCount != 4 || sum.Sidecars != 3 || sum.Total != 1 {
		t.Fatalf("summary %+v", sum)
	}
	for _, it := range Execute(items) {
		if it.Status != StatusRenamed {
			t.Fatalf("%+v", it)
		}
	}
	want := "IMG_001.txt,IMG_0010.xmp,Trip_IMG_001.CR2,Trip_IMG_001.CR2.xmp,Trip_IMG_001.JPG,Trip_IMG_001.xmp"
	if got := strings.Join(names(t, dir), ","); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}

	// without the option only the primary is renamed
	dir2 := t.TempDir()
	r2 := write(t, dir2, "IMG_001.CR2", "")
	write(t, dir2, "IMG_001.xmp", "")
	all2, _ := ListFiles(t.Context(), dir2, false, nil)
	_, sum = BuildPlan([]string{r2}, steps, PlanOptions{All: all2})
	if sum.OkCount != 1 {
		t.Fatalf("%+v", sum)
	}
}

func TestSidecarExtensionChangeAndSelectedCompanion(t *testing.T) {
	dir := t.TempDir()
	cr2 := write(t, dir, "A.CR2", "")
	write(t, dir, "A.xmp", "")
	jpg := write(t, dir, "A.JPG", "")
	all, _ := ListFiles(t.Context(), dir, false, nil)

	// changing only the extension leaves sidecars alone
	_, sum := BuildPlan([]string{cr2}, []Step{{Op: OpChangeExt, A: "cr3"}}, PlanOptions{Sidecars: true, All: all})
	if sum.OkCount != 1 || sum.Sidecars != 0 {
		t.Fatalf("%+v", sum)
	}
	// a companion that is itself selected is renamed once, as a primary
	items, sum := BuildPlan([]string{cr2, jpg}, []Step{{Op: OpPrepend, A: "x_"}}, PlanOptions{Sidecars: true, All: all})
	if sum.OkCount != 3 || sum.Sidecars != 1 { // CR2, JPG + the single xmp
		t.Fatalf("%+v", sum)
	}
	seen := map[string]int{}
	for _, it := range items {
		seen[it.OldPath]++
	}
	for p, n := range seen {
		if n != 1 {
			t.Fatalf("%s planned %d times", p, n)
		}
	}
}

func TestSidecarGroupIsAllOrNothing(t *testing.T) {
	dir := t.TempDir()
	raw := write(t, dir, "A.CR2", "")
	write(t, dir, "A.xmp", "")
	write(t, dir, "B_A.xmp", "in the way") // blocks A.xmp -> B_A.xmp
	all, _ := ListFiles(t.Context(), dir, false, nil)

	items, sum := BuildPlan([]string{raw}, []Step{{Op: OpPrepend, A: "B_"}}, PlanOptions{Sidecars: true, All: all})
	if sum.OkCount != 0 || len(sum.TargetExists) != 1 || len(sum.GroupSkipped) != 1 {
		t.Fatalf("%+v", sum)
	}
	Execute(items)
	if got := strings.Join(names(t, dir), ","); got != "A.CR2,A.xmp,B_A.xmp" {
		t.Fatalf("nothing may change: %s", got)
	}
}

func TestParseExts(t *testing.T) {
	s := ParseExts(" XMP, .jpg;CR2  ")
	if len(s) != 3 || !s[".xmp"] || !s[".jpg"] || !s[".cr2"] {
		t.Fatalf("%v", s)
	}
	if len(ParseExts("")) != 0 {
		t.Fatal("empty list")
	}
}
