package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// textScales are the selectable text sizes (relative to the default).
var textScales = []float32{0.85, 0.92, 1.0, 1.1, 1.2, 1.35}

const defaultTextScaleIdx = 2

// Colour modes of the theme button.
const (
	modeAuto  = 0 // follow the operating system
	modeLight = 1
	modeDark  = 2
)

var modeNames = []string{"Auto", "Light", "Dark"}

// scaledTheme is the default Fyne theme with all text sizes multiplied by scale
// and, optionally, a forced light or dark variant. Padding and widget sizes are
// unchanged, so the layout stays tidy.
type scaledTheme struct {
	fyne.Theme
	scale float32
	mode  int
}

func newScaledTheme(scale float32, mode int) fyne.Theme {
	return scaledTheme{Theme: theme.DefaultTheme(), scale: scale, mode: mode}
}

func (t scaledTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch t.mode {
	case modeLight:
		variant = theme.VariantLight
	case modeDark:
		variant = theme.VariantDark
	}
	return t.Theme.Color(name, variant)
}

func (t scaledTheme) Size(name fyne.ThemeSizeName) float32 {
	s := t.Theme.Size(name)
	switch name {
	case theme.SizeNameText, theme.SizeNameCaptionText, theme.SizeNameSubHeadingText, theme.SizeNameHeadingText:
		return s * t.scale
	}
	return s
}
