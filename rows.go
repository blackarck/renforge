package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// rowInfo is what one preview row shows.
type rowInfo struct {
	orig     string
	parts    []diffPart // the new name in pieces; changed pieces are highlighted
	note     string     // warning or hint shown after the name
	kind     widget.Importance
	selected bool
}

// previewLen is the number of characters in the preview text (for row heights).
func (r rowInfo) previewLen() int {
	n := len([]rune(r.note))
	for _, p := range r.parts {
		n += len([]rune(p.text))
	}
	return n
}

func textSeg(text string, color fyne.ThemeColorName, bold bool) widget.RichTextSegment {
	return &widget.TextSegment{Text: text, Style: widget.RichTextStyle{
		ColorName: color, Inline: true, TextStyle: fyne.TextStyle{Bold: bold},
	}}
}

// previewSegments builds the coloured text of the Preview column: the changed
// part of the name is highlighted, warnings use the warning / error colours.
func previewSegments(r rowInfo) []widget.RichTextSegment {
	plain, changed := theme.ColorNameForeground, theme.ColorNamePrimary
	switch {
	case !r.selected || r.kind == widget.LowImportance:
		plain, changed = theme.ColorNameDisabled, theme.ColorNameDisabled
	case r.kind == widget.DangerImportance:
		changed = theme.ColorNameError
	case r.kind == widget.WarningImportance:
		changed = theme.ColorNameWarning
	}

	var out []widget.RichTextSegment
	for _, p := range r.parts {
		if p.text == "" {
			continue
		}
		if p.changed {
			out = append(out, textSeg(p.text, changed, true))
		} else {
			out = append(out, textSeg(p.text, plain, false))
		}
	}
	if r.note != "" {
		noteColor := theme.ColorNameDisabled
		switch r.kind {
		case widget.DangerImportance:
			noteColor = theme.ColorNameError
		case widget.WarningImportance:
			noteColor = theme.ColorNameWarning
		}
		out = append(out, textSeg(r.note, noteColor, false))
	}
	if len(out) == 0 {
		out = append(out, textSeg(" ", plain, false))
	}
	return out
}

// twoCol lays out two columns whose width ratio can be changed (and is shared
// by every row and the header).
type twoCol struct{ ratio *float32 }

func (l *twoCol) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	pad := theme.Padding()
	w1 := (size.Width - pad) * *l.ratio
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(w1, size.Height))
	objs[1].Move(fyne.NewPos(w1+pad, 0))
	objs[1].Resize(fyne.NewSize(size.Width-pad-w1, size.Height))
	if len(objs) > 2 { // optional drag handle, centred on the boundary
		const hw = 12
		objs[2].Move(fyne.NewPos(w1+pad/2-hw/2, 0))
		objs[2].Resize(fyne.NewSize(hw, size.Height))
	}
}

func (l *twoCol) MinSize(objs []fyne.CanvasObject) fyne.Size {
	a, b := objs[0].MinSize(), objs[1].MinSize()
	h := a.Height
	if b.Height > h {
		h = b.Height
	}
	return fyne.NewSize(a.Width+b.Width+theme.Padding(), h)
}

// fileRow is one reusable row of the virtualized preview list:
// [checkbox] original name | new name. Long names wrap onto several lines; the
// list gives such rows a taller height (see updateRowHeights).
type fileRow struct {
	widget.BaseWidget
	chk  *widget.Check
	orig *widget.RichText
	prev *widget.RichText
	box  *fyne.Container
}

func newFileRow(ratio *float32) *fileRow {
	r := &fileRow{
		chk:  widget.NewCheck("", nil),
		orig: widget.NewRichText(),
		prev: widget.NewRichText(),
	}
	r.orig.Wrapping = fyne.TextWrapWord
	r.prev.Wrapping = fyne.TextWrapWord
	r.box = container.NewBorder(nil, nil, container.NewCenter(r.chk), nil,
		container.New(&twoCol{ratio: ratio}, container.NewPadded(r.orig), container.NewPadded(r.prev)))
	r.ExtendBaseWidget(r)
	return r
}

func (r *fileRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.box)
}

// set fills the row. The checkbox callback is detached while its state is set
// so that recycling a row never fires a spurious "toggled" event.
func (r *fileRow) set(info rowInfo, onToggle func(bool)) {
	r.chk.OnChanged = nil
	r.chk.SetChecked(info.selected)
	r.chk.OnChanged = onToggle

	origColor := theme.ColorNameForeground
	if !info.selected {
		origColor = theme.ColorNameDisabled
	}
	r.orig.Segments = []widget.RichTextSegment{textSeg(info.orig, origColor, false)}
	r.orig.Refresh()
	r.prev.Segments = previewSegments(info)
	r.prev.Refresh()
}

// dragHandle is the thin draggable divider between the two preview columns.
type dragHandle struct {
	widget.BaseWidget
	onDrag func(dx float32)
	onEnd  func()
}

func newDragHandle(onDrag func(dx float32), onEnd func()) *dragHandle {
	h := &dragHandle{onDrag: onDrag, onEnd: onEnd}
	h.ExtendBaseWidget(h)
	return h
}

func (h *dragHandle) CreateRenderer() fyne.WidgetRenderer {
	line := canvas.NewRectangle(theme.Color(theme.ColorNameSeparator))
	line.SetMinSize(fyne.NewSize(3, 18))
	return widget.NewSimpleRenderer(container.NewCenter(line))
}

func (h *dragHandle) Dragged(e *fyne.DragEvent) { h.onDrag(e.Dragged.DX) }
func (h *dragHandle) DragEnd()                  { h.onEnd() }
func (h *dragHandle) Cursor() desktop.Cursor    { return desktop.HResizeCursor }

// resizeWatcher lays out a single child to fill the space and reports size
// changes (asynchronously, on the UI thread).
type resizeWatcher struct {
	onWidth func(w float32)
	onSize  func(s fyne.Size)
	last    fyne.Size
}

func (l *resizeWatcher) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(size)
	}
	if size == l.last {
		return
	}
	widthChanged := size.Width != l.last.Width
	l.last = size
	if l.onWidth != nil && widthChanged {
		w := size.Width
		fyne.Do(func() { l.onWidth(w) })
	}
	if l.onSize != nil {
		fyne.Do(func() { l.onSize(size) })
	}
}

func (l *resizeWatcher) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) == 0 {
		return fyne.NewSize(0, 0)
	}
	return objs[0].MinSize()
}
