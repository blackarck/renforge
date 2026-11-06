package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"FileRenUtil/internal/preset"
	"FileRenUtil/internal/rename"
)

// presetHooks connects the presets dialog to the main window.
type presetHooks struct {
	current func() preset.Preset // snapshot of the current filters + steps
	apply   func(preset.Preset)  // replace the current filters + steps
}

const builtinSuffix = "  (built-in)"

// describeStep renders a one-line description of a step for the presets dialog.
func describeStep(s rename.Step) string {
	d := string(s.Op)
	switch {
	case rename.Options(s.Op) != nil:
		d += ": " + s.A
	case s.Op == rename.OpReplaceText || s.Op == rename.OpRegex:
		d += fmt.Sprintf(": %q → %q", s.A, s.B)
	case s.Op == rename.OpNumber || s.Op == rename.OpTemplate:
		t := s.A
		if t == "" {
			t = "(default template)"
		}
		d += fmt.Sprintf(": %s (start %s)", t, strings.TrimSpace(orDefault(s.B, "1")))
	case s.Op == rename.OpFrames:
		d += fmt.Sprintf(": offset %s", orDefault(s.A, "0"))
	case s.A != "":
		d += fmt.Sprintf(": %q", s.A)
	}
	if s.Disabled {
		d += " [off]"
	}
	return d
}

func showPresets(w fyne.Window, h presetHooks) {
	dir, err := preset.Dir()
	if err != nil {
		dialog.ShowError(fmt.Errorf("cannot find a place to store presets: %w", err), w)
		return
	}

	byLabel := map[string]preset.Preset{}
	sel := widget.NewSelect(nil, nil)
	sel.PlaceHolder = "Choose a preset…"

	desc := widget.NewLabel("Presets save your filters and rename steps so you can reuse them.")
	desc.Wrapping = fyne.TextWrapWord

	loadBtn := widget.NewButton("Load", nil)
	deleteBtn := widget.NewButton("Delete", nil)
	loadBtn.Disable()
	deleteBtn.Disable()

	var dlg dialog.Dialog

	reload := func(selectLabel string) {
		list, lerr := preset.List(dir)
		if lerr != nil {
			dialog.ShowError(lerr, w)
		}
		byLabel = map[string]preset.Preset{}
		var labels []string
		for _, p := range list {
			label := p.Name
			if p.Builtin {
				label += builtinSuffix
			}
			byLabel[label] = p
			labels = append(labels, label)
		}
		sel.SetOptions(labels)
		if _, ok := byLabel[selectLabel]; ok {
			sel.SetSelected(selectLabel)
		} else {
			sel.ClearSelected()
			desc.SetText("Presets save your filters and rename steps so you can reuse them.")
			loadBtn.Disable()
			deleteBtn.Disable()
		}
	}

	sel.OnChanged = func(label string) {
		p, ok := byLabel[label]
		if !ok {
			return
		}
		var b strings.Builder
		for i, s := range p.Steps {
			b.WriteString(fmt.Sprintf("%d. %s\n", i+1, describeStep(s)))
		}
		if len(p.Filters) > 0 {
			b.WriteString(fmt.Sprintf("\n%d filter(s) included.", len(p.Filters)))
		}
		if b.Len() == 0 {
			b.WriteString("(empty preset)")
		}
		desc.SetText(b.String())
		loadBtn.Enable()
		if p.Builtin {
			deleteBtn.Disable()
		} else {
			deleteBtn.Enable()
		}
	}

	loadBtn.OnTapped = func() {
		if p, ok := byLabel[sel.Selected]; ok {
			h.apply(p)
			dlg.Hide()
		}
	}

	deleteBtn.OnTapped = func() {
		p, ok := byLabel[sel.Selected]
		if !ok || p.Builtin {
			return
		}
		dialog.ShowConfirm("Delete preset", fmt.Sprintf("Delete the preset %q?", p.Name), func(yes bool) {
			if !yes {
				return
			}
			if err := preset.Delete(dir, p.Name); err != nil {
				dialog.ShowError(err, w)
			}
			reload("")
		}, w)
	}

	saveBtn := widget.NewButton("Save current as…", func() {
		cur := h.current()
		if len(cur.Steps) == 0 && len(cur.Filters) == 0 {
			dialog.ShowInformation("Nothing to save", "Add some filters or rename steps first.", w)
			return
		}
		entry := widget.NewEntry()
		entry.SetPlaceHolder("e.g. Wedding photos")
		form := dialog.NewForm("Save preset", "Save", "Cancel",
			[]*widget.FormItem{widget.NewFormItem("Name", entry)},
			func(ok bool) {
				if !ok {
					return
				}
				name := strings.TrimSpace(entry.Text)
				if err := preset.ValidateName(name); err != nil {
					dialog.ShowError(err, w)
					return
				}
				save := func() {
					cur.Name = name
					if err := preset.Save(dir, cur); err != nil {
						dialog.ShowError(err, w)
						return
					}
					reload(name)
				}
				if _, exists := byLabel[name]; exists {
					dialog.ShowConfirm("Replace preset", fmt.Sprintf("A preset named %q already exists. Replace it?", name),
						func(yes bool) {
							if yes {
								save()
							}
						}, w)
					return
				}
				save()
			}, w)
		form.Resize(fyne.NewSize(420, 180))
		form.Show()
	})

	content := container.NewVBox(
		sel,
		container.NewHBox(loadBtn, deleteBtn),
		widget.NewSeparator(),
		container.NewVScroll(desc),
		widget.NewSeparator(),
		saveBtn,
	)
	reload("")

	dlg = dialog.NewCustom("Presets", "Close", content, w)
	dlg.Resize(fyne.NewSize(520, 460))
	dlg.Show()
}
