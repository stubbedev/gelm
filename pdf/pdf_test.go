package pdf

import (
	"bytes"
	"compress/zlib"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

func quadrants() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	img.Set(1, 0, color.RGBA{0, 255, 0, 255})
	img.Set(2, 0, color.RGBA{0, 0, 255, 255})
	img.Set(3, 0, color.RGBA{0, 0, 0, 0})
	img.Set(0, 1, color.RGBA{0, 0, 128, 128})
	return img
}

func document(t *testing.T, title string, pages ...image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := NewWriter(&buf, title)
	for _, p := range pages {
		if err := w.AddPage(Size{Millimetres(210), Millimetres(297)}, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCrossReferencesPointAtTheirObjects(t *testing.T) {
	doc := document(t, "x", quadrants(), quadrants())
	start := regexp.MustCompile(`startxref\n(\d+)\n%%EOF\n$`).FindSubmatch(doc)
	if start == nil {
		t.Fatalf("no startxref trailer in %q", doc[len(doc)-60:])
	}
	at, _ := strconv.Atoi(string(start[1]))
	if !bytes.HasPrefix(doc[at:], []byte("xref\n0 ")) {
		t.Fatalf("startxref %d points at %q", at, doc[at:at+10])
	}
	entries := regexp.MustCompile(`(\d{10}) 00000 n \n`).FindAllSubmatch(doc[at:], -1)
	if len(entries) != 3+2*4 {
		t.Fatalf("%d xref entries, want catalog, pages, info and four per page", len(entries))
	}
	for i, e := range entries {
		off, _ := strconv.Atoi(string(e[1]))
		if want := strconv.Itoa(i+1) + " 0 obj\n"; !bytes.HasPrefix(doc[off:], []byte(want)) {
			t.Errorf("object %d at %d reads %q", i+1, off, doc[off:off+12])
		}
	}
	if !bytes.Contains(doc, []byte("/Count 2")) {
		t.Error("the page tree does not count two pages")
	}
}

func pixels(t *testing.T, doc []byte) []byte {
	t.Helper()
	m := regexp.MustCompile(`(?s)/Subtype /Image .*?stream\n(.*?)\nendstream`).FindSubmatch(doc)
	if m == nil {
		t.Fatal("no image stream")
	}
	r, err := zlib.NewReader(bytes.NewReader(m[1]))
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestPixelsAreCompositedOverWhite(t *testing.T) {
	want := []byte{
		255, 0, 0, 0, 255, 0, 0, 0, 255, 255, 255, 255,
		127, 127, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	}
	if got := pixels(t, document(t, "", quadrants())); !bytes.Equal(got, want) {
		t.Errorf("RGBA pixels\n got %v\nwant %v", got, want)
	}
	other := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	other.Set(0, 0, color.NRGBA{64, 64, 64, 255})
	other.Set(1, 0, color.NRGBA{0, 0, 0, 0})
	if got := pixels(t, document(t, "", other)); !bytes.Equal(got, []byte{64, 64, 64, 255, 255, 255}) {
		t.Errorf("NRGBA pixels %v", got)
	}
}

func TestOutputIsDeterministicAndTitled(t *testing.T) {
	a, b := document(t, "Ærø", quadrants()), document(t, "Ærø", quadrants())
	if !bytes.Equal(a, b) {
		t.Error("the same pages wrote different bytes")
	}
	if !bytes.Contains(a, []byte("/Title <FEFF00C6007200F8>")) {
		t.Error("the title is not a UTF-16 text string")
	}
}

type failAfter struct{ n int }

func (f *failAfter) Write(p []byte) (int, error) {
	if f.n -= len(p); f.n < 0 {
		return 0, errors.New("disk full")
	}
	return len(p), nil
}

func TestErrorsAreReportedAndStick(t *testing.T) {
	if err := NewWriter(io.Discard, "").Close(); !errors.Is(err, ErrNoPages) {
		t.Errorf("an empty document closed with %v", err)
	}
	w := NewWriter(io.Discard, "")
	if err := w.AddPage(Size{0, 10}, quadrants()); err == nil {
		t.Error("a zero-width page was accepted")
	}
	if err := w.AddPage(Size{10, 10}, image.NewRGBA(image.Rectangle{})); err == nil {
		t.Error("an empty image was accepted")
	}
	w = NewWriter(&failAfter{n: 40}, "")
	if err := w.AddPage(Size{10, 10}, quadrants()); err == nil {
		t.Error("a failing writer was not reported")
	}
	if err := w.Close(); err == nil || err.Error() != "pdf: write: disk full" {
		t.Errorf("close after a failed write returned %v", err)
	}
	if err := w.Close(); err == nil {
		t.Error("a second close succeeded")
	}
}

func TestMuPDFRendersThePage(t *testing.T) {
	mutool, lookErr := exec.LookPath("mutool")
	if lookErr != nil {
		t.Skip("mutool not installed")
	}
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := range 100 {
		for x := range 50 {
			img.Set(x, y, color.RGBA{0, 0, 255, 255})
		}
	}
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.pdf"), filepath.Join(dir, "out.png")
	var buf bytes.Buffer
	w := NewWriter(&buf, "")
	if err := w.AddPage(Size{100, 100}, img); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(in, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(mutool, "draw", "-q", "-r", "72", "-o", out, in)
	if msg, err := cmd.CombinedOutput(); err != nil || len(msg) > 0 {
		t.Fatalf("mutool: %v: %s", err, msg)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if b := got.Bounds(); b.Dx() != 100 || b.Dy() != 100 {
		t.Fatalf("rendered %v, want 100x100 at 72 dpi", b)
	}
	for _, p := range []struct {
		x, y int
		want color.RGBA
	}{{10, 50, color.RGBA{0, 0, 255, 255}}, {90, 50, color.RGBA{255, 255, 255, 255}}} {
		if c := color.RGBAModel.Convert(got.At(p.x, p.y)).(color.RGBA); c != p.want {
			t.Errorf("pixel (%d,%d) = %v, want %v", p.x, p.y, c, p.want)
		}
	}
}
