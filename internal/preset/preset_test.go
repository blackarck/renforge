package preset

import (
	"os"
	"path/filepath"
	"testing"

	"renforge/internal/filter"
	"renforge/internal/rename"
)

func TestSaveListDeleteRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "presets") // does not exist yet
	p := Preset{
		Name:     "  My Photos! ",
		MatchAll: true,
		Steps:    []rename.Step{{Op: rename.OpNumber, A: "Trip_{n:3}", B: "1", Disabled: true}},
		Filters:  []filter.Rule{{Mode: filter.Extension, Value: "jpg"}},
	}
	if err := Save(dir, p); err != nil {
		t.Fatal(err)
	}
	list, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != len(Builtins())+1 {
		t.Fatalf("got %d presets", len(list))
	}
	got := list[len(list)-1]
	if got.Name != "My Photos!" || got.Builtin || len(got.Steps) != 1 || !got.Steps[0].Disabled ||
		got.Steps[0].A != "Trip_{n:3}" || got.Filters[0].Value != "jpg" {
		t.Fatalf("round trip mismatch: %+v", got)
	}

	// saving the same name replaces it
	p.Steps = nil
	if err := Save(dir, p); err != nil {
		t.Fatal(err)
	}
	if list, _ = List(dir); len(list) != len(Builtins())+1 {
		t.Fatalf("expected replace, got %d presets", len(list))
	}

	if err := Delete(dir, "My Photos!"); err != nil {
		t.Fatal(err)
	}
	if list, _ = List(dir); len(list) != len(Builtins()) {
		t.Fatalf("expected only built-ins, got %d", len(list))
	}
	if err := Delete(dir, "My Photos!"); err != nil {
		t.Fatalf("deleting a missing preset should be a no-op: %v", err)
	}
}

func TestValidateName(t *testing.T) {
	for _, bad := range []string{"", "   ", "!!!", "clean up downloads", string(make([]byte, 61))} {
		if ValidateName(bad) == nil {
			t.Errorf("ValidateName(%q) should fail", bad)
		}
	}
	if err := ValidateName("Wedding 2024"); err != nil {
		t.Error(err)
	}
}

func TestListSkipsMalformed(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{not json"), 0o644)
	os.WriteFile(filepath.Join(dir, "noname.json"), []byte(`{"steps":[]}`), 0o644)
	list, err := List(dir)
	if err != nil || len(list) != len(Builtins()) {
		t.Fatalf("list=%d err=%v", len(list), err)
	}
}

// Every built-in preset must produce valid steps and sensible output.
func TestBuiltinsAreValid(t *testing.T) {
	for _, p := range Builtins() {
		for _, s := range p.Steps {
			if msg := rename.ValidateStep(s); msg != "" {
				t.Errorf("%s: %s", p.Name, msg)
			}
			if opts := rename.Options(s.Op); opts != nil {
				found := false
				for _, o := range opts {
					found = found || o == s.A
				}
				if !found {
					t.Errorf("%s: %q is not a valid option for %s", p.Name, s.A, s.Op)
				}
			}
		}
	}
	var clean Preset
	for _, p := range Builtins() {
		if p.Name == "Clean up downloads" {
			clean = p
		}
	}
	if got := rename.ApplySteps("  My.Cool_Song  (1).MP3", clean.Steps); got != "My Cool Song.MP3" {
		t.Errorf("clean up downloads gave %q", got)
	}
}
