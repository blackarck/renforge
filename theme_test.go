package main

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func TestScaledThemeSizes(t *testing.T) {
	base := theme.DefaultTheme()
	big := newScaledTheme(1.2, modeAuto)
	if got, want := big.Size(theme.SizeNameText), base.Size(theme.SizeNameText)*1.2; got != want {
		t.Errorf("text size %v, want %v", got, want)
	}
	if big.Size(theme.SizeNamePadding) != base.Size(theme.SizeNamePadding) {
		t.Error("padding must not be scaled")
	}
}

func TestThemeModeForcesVariant(t *testing.T) {
	test.NewApp() // the built-in theme reads the current app
	light := newScaledTheme(1, modeLight).Color(theme.ColorNameBackground, theme.VariantDark)
	dark := newScaledTheme(1, modeDark).Color(theme.ColorNameBackground, theme.VariantLight)
	if light == dark {
		t.Fatal("forced light and dark must differ")
	}
	auto := newScaledTheme(1, modeAuto)
	if auto.Color(theme.ColorNameBackground, theme.VariantLight) != theme.DefaultTheme().Color(theme.ColorNameBackground, theme.VariantLight) {
		t.Fatal("auto must follow the variant it is given")
	}
}

func TestPreviewSegments(t *testing.T) {
	segs := previewSegments(rowInfo{orig: "a.txt", parts: []diffPart{{text: "x_"}, {text: "NEW", changed: true}, {text: ".txt"}}, selected: true, kind: widget.MediumImportance})
	if len(segs) != 3 {
		t.Fatalf("want 3 segments, got %d", len(segs))
	}
	text := ""
	for _, sg := range segs {
		text += sg.(*widget.TextSegment).Text
	}
	if text != "x_NEW.txt" {
		t.Fatalf("segments rebuild %q", text)
	}
	// never empty (RichText needs at least one segment)
	if len(previewSegments(rowInfo{})) != 1 {
		t.Fatal("empty row must still yield a segment")
	}
	withNote := previewSegments(rowInfo{parts: []diffPart{{text: "n"}}, note: "  ⚠ x", kind: widget.DangerImportance, selected: true})
	if len(withNote) != 2 {
		t.Fatalf("note segment missing: %d", len(withNote))
	}
}
