package main

import (
	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

//go:embed icon.png
var iconPNG []byte

// appIcon is used for the window, taskbar and dock/app switcher at run time.
// (Packaged builds additionally embed it in the executable / app bundle.)
var appIcon = fyne.NewStaticResource("icon.png", iconPNG)

// brandImage returns the RenForge icon at the given size (in dp).
func brandImage(size float32) fyne.CanvasObject {
	img := canvas.NewImageFromResource(appIcon)
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(size, size))
	return container.NewCenter(img)
}

// brandTitle is "RenForge" in a heading style, with an optional second line.
func brandTitle(subtitle string, centered bool) fyne.CanvasObject {
	align := fyne.TextAlignLeading
	if centered {
		align = fyne.TextAlignCenter
	}
	title := widget.NewRichText(&widget.TextSegment{
		Text: "RenForge",
		Style: widget.RichTextStyle{
			Alignment: align,
			SizeName:  theme.SizeNameHeadingText,
			TextStyle: fyne.TextStyle{Bold: true},
		},
	})
	if subtitle == "" {
		return title
	}
	sub := widget.NewLabelWithStyle(subtitle, align, fyne.TextStyle{})
	sub.Importance = widget.LowImportance
	return container.NewVBox(title, sub)
}

// brandHeader is the icon next to the RenForge name, used on the Help dialog.
func brandHeader(subtitle string) fyne.CanvasObject {
	return container.NewHBox(brandImage(44), brandTitle(subtitle, false))
}

// openInfoDialog is the Help or About dialog currently on screen (if any); the
// Esc key closes it.
var openInfoDialog dialog.Dialog

// showInfoDialog shows d and remembers it so Esc can close it.
func showInfoDialog(d dialog.Dialog) {
	closeInfoDialog()
	openInfoDialog = d
	d.SetOnClosed(func() {
		if openInfoDialog == d {
			openInfoDialog = nil
		}
	})
	d.Show()
}

// closeInfoDialog closes the Help/About dialog if one is open.
func closeInfoDialog() {
	if d := openInfoDialog; d != nil {
		openInfoDialog = nil
		d.Hide()
	}
}
