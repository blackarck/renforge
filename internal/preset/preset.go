// Package preset stores named rename pipelines (steps + filters) as JSON files
// and provides a few built-in ones.
package preset

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"renforge/internal/filter"
	"renforge/internal/rename"
)

// Preset is a saved pipeline.
type Preset struct {
	Name          string        `json:"name"`
	Steps         []rename.Step `json:"steps"`
	Filters       []filter.Rule `json:"filters,omitempty"`
	MatchAll      bool          `json:"match_all"`
	CaseSensitive bool          `json:"case_sensitive,omitempty"`
	Builtin       bool          `json:"-"`
}

// Dir returns the folder where user presets are stored.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "renforge", "presets"), nil
}

// Builtins returns the presets shipped with RenForge.
func Builtins() []Preset {
	return []Preset{
		{Name: "Clean up downloads", MatchAll: true, Builtin: true, Steps: []rename.Step{
			{Op: rename.OpReplaceText, A: "%20", B: " "},
			{Op: rename.OpCleanup, A: rename.CleanDots},
			{Op: rename.OpCleanup, A: rename.CleanCopies},
			{Op: rename.OpCleanup, A: rename.CleanSpaces},
			{Op: rename.OpCleanup, A: rename.CleanTrim},
		}},
		{Name: "Lowercase names and extensions", MatchAll: true, Builtin: true, Steps: []rename.Step{
			{Op: rename.OpCase, A: rename.CaseLower},
			{Op: rename.OpCase, A: rename.CaseExtLower},
		}},
		{Name: "kebab-case names", MatchAll: true, Builtin: true, Steps: []rename.Step{
			{Op: rename.OpCase, A: rename.CaseKebab},
			{Op: rename.OpCase, A: rename.CaseExtLower},
		}},
		{Name: "Photos: Trip_0001", MatchAll: true, Builtin: true, Steps: []rename.Step{
			{Op: rename.OpNumber, A: "Trip_{n:4}", B: "1"},
		}},
		{Name: "Photos by date: 2024-06-01_001", MatchAll: true, Builtin: true, Steps: []rename.Step{
			{Op: rename.OpTemplate, A: "{date:YYYY-MM-DD}_{n:3}", B: "1"},
		}},
		{Name: "Photos by date and time", MatchAll: true, Builtin: true, Steps: []rename.Step{
			{Op: rename.OpTemplate, A: "{date:YYYYMMDD_HHmmss}_{n:2}", B: "1"},
		}},
		{Name: "Image sequence: start at frame 1001", MatchAll: true, Builtin: true, Steps: []rename.Step{
			{Op: rename.OpFrames, A: "+1000"},
		}},
		{Name: "Add number suffix: name_001", MatchAll: true, Builtin: true, Steps: []rename.Step{
			{Op: rename.OpNumber, A: "{name}_{n:3}", B: "1"},
		}},
	}
}

// slug turns a preset name into a safe file name stem.
func slug(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// ValidateName checks a user-chosen preset name.
func ValidateName(name string) error {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return errors.New("enter a name for the preset")
	case len(name) > 60:
		return errors.New("name is too long (60 characters max)")
	case slug(name) == "":
		return errors.New("name must contain letters or numbers")
	}
	for _, b := range Builtins() {
		if strings.EqualFold(b.Name, name) {
			return fmt.Errorf("%q is a built-in preset; choose another name", b.Name)
		}
	}
	return nil
}

// Save writes (or replaces) a user preset in dir.
func Save(dir string, p Preset) error {
	p.Name = strings.TrimSpace(p.Name)
	if err := ValidateName(p.Name); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, slug(p.Name)+".json"), data, 0o644)
}

// Delete removes a user preset by name. Built-ins cannot be deleted.
func Delete(dir, name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	err := os.Remove(filepath.Join(dir, slug(name)+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// List returns the built-in presets followed by the user's, sorted by name.
// Unreadable or malformed files are skipped.
func List(dir string) ([]Preset, error) {
	out := Builtins()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return out, err
	}
	var user []Preset
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var p Preset
		if json.Unmarshal(data, &p) != nil || strings.TrimSpace(p.Name) == "" {
			continue
		}
		user = append(user, p)
	}
	sort.Slice(user, func(i, j int) bool { return strings.ToLower(user[i].Name) < strings.ToLower(user[j].Name) })
	return append(out, user...), nil
}
