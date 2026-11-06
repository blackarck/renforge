package meta

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStatLevelAndCache(t *testing.T) {
	Reset()
	p := filepath.Join(t.TempDir(), "a.txt")
	os.WriteFile(p, []byte("hello"), 0o644)
	mod := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	os.Chtimes(p, mod, mod)

	if _, ok := Cached(p, Stat); ok {
		t.Fatal("must not be cached yet")
	}
	info := Load(p, Stat)
	if info.Size != 5 || !info.ModTime.Equal(mod) || info.Level != Stat {
		t.Fatalf("%+v", info)
	}
	if !info.Date().Equal(mod) {
		t.Fatal("Date() should fall back to ModTime")
	}
	if _, ok := Cached(p, EXIF); ok {
		t.Fatal("a Stat entry must not satisfy an EXIF request")
	}
	if Missing([]string{p}, Stat) != 0 || Missing([]string{p}, EXIF) != 1 {
		t.Fatal("Missing is wrong")
	}
}

func TestNonImageAndMissingFilesNeverFail(t *testing.T) {
	Reset()
	dir := t.TempDir()
	txt := filepath.Join(dir, "a.jpg") // claims to be a JPEG but is not
	os.WriteFile(txt, []byte("not an image"), 0o644)
	info := Load(txt, EXIF)
	if info.Level != EXIF || !info.Taken.IsZero() || info.Size == 0 {
		t.Fatalf("%+v", info)
	}
	if info := Load(filepath.Join(dir, "missing.jpg"), EXIF); info.Size != 0 {
		t.Fatalf("%+v", info)
	}
}

func TestEnsureAndCancel(t *testing.T) {
	Reset()
	dir := t.TempDir()
	var files []string
	for _, n := range []string{"a", "b", "c"} {
		p := filepath.Join(dir, n+".txt")
		os.WriteFile(p, nil, 0o644)
		files = append(files, p)
	}
	if err := Ensure(context.Background(), files, Stat, nil); err != nil || Missing(files, Stat) != 0 {
		t.Fatalf("err=%v missing=%d", err, Missing(files, Stat))
	}
	Reset()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Ensure(ctx, files, Stat, nil); err == nil {
		t.Fatal("expected cancellation error")
	}
}

// A tiny hand-built JPEG with an EXIF block (Make, Model, DateTimeOriginal).
func TestEXIFFromJPEG(t *testing.T) {
	Reset()
	p := filepath.Join(t.TempDir(), "x.jpg")
	if err := os.WriteFile(p, buildExifJPEG("TestCam", "Model X", "2021:03:04 05:06:07"), 0o644); err != nil {
		t.Fatal(err)
	}
	info := Load(p, EXIF)
	if info.Make != "TestCam" || info.Model != "Model X" {
		t.Fatalf("make/model not read: %+v", info)
	}
	if got := info.Taken.Format("2006-01-02 15:04:05"); got != "2021-03-04 05:06:07" {
		t.Fatalf("taken = %q", got)
	}
}

// buildExifJPEG assembles a minimal JPEG (SOI + APP1/EXIF + EOI) with a
// little-endian TIFF block holding Make (0x010F), Model (0x0110) and an Exif
// sub-IFD (0x8769) with DateTimeOriginal (0x9003).
func buildExifJPEG(make_, model, dt string) []byte {
	le16 := func(v uint16) []byte { return []byte{byte(v), byte(v >> 8)} }
	le32 := func(v uint32) []byte { return []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)} }
	asciiZ := func(s string) []byte { return append([]byte(s), 0) }

	mk, md, d := asciiZ(make_), asciiZ(model), asciiZ(dt)

	// layout: header(8) | IFD0 (2 + 3*12 + 4) | data: make, model | exif IFD (2+12+4) | data: dt
	ifd0Off := uint32(8)
	ifd0Len := uint32(2 + 3*12 + 4)
	dataOff := ifd0Off + ifd0Len
	mkOff := dataOff
	mdOff := mkOff + uint32(len(mk))
	exifIfdOff := mdOff + uint32(len(md))
	dtOff := exifIfdOff + 2 + 12 + 4

	entry := func(tag, typ uint16, count, val uint32) []byte {
		b := append(le16(tag), le16(typ)...)
		b = append(b, le32(count)...)
		return append(b, le32(val)...)
	}
	var tiff []byte
	tiff = append(tiff, 'I', 'I', 42, 0)
	tiff = append(tiff, le32(ifd0Off)...)
	tiff = append(tiff, le16(3)...)
	tiff = append(tiff, entry(0x010F, 2, uint32(len(mk)), mkOff)...)
	tiff = append(tiff, entry(0x0110, 2, uint32(len(md)), mdOff)...)
	tiff = append(tiff, entry(0x8769, 4, 1, exifIfdOff)...)
	tiff = append(tiff, le32(0)...)
	tiff = append(tiff, mk...)
	tiff = append(tiff, md...)
	tiff = append(tiff, le16(1)...)
	tiff = append(tiff, entry(0x9003, 2, uint32(len(d)), dtOff)...)
	tiff = append(tiff, le32(0)...)
	tiff = append(tiff, d...)

	app1 := append([]byte("Exif\x00\x00"), tiff...)
	n := uint16(len(app1) + 2)
	out := []byte{0xFF, 0xD8, 0xFF, 0xE1, byte(n >> 8), byte(n)}
	out = append(out, app1...)
	return append(out, 0xFF, 0xD9)
}
