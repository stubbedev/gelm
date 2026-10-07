package transfer

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"slices"
	"testing"
)

// Text offers every text mime with the same bytes; Merge keeps the
// first part's payload for a shared mime.
func TestTextAndMerge(t *testing.T) {
	c := Text("hi")
	if !slices.Equal(c.Mimes, TextMimes) {
		t.Errorf("text mimes = %v", c.Mimes)
	}
	for _, m := range TextMimes {
		if b, err := c.Bytes(m); err != nil || string(b) != "hi" {
			t.Errorf("%s = %q, %v", m, b, err)
		}
	}
	h := HTML("<b>hi</b>", "hi")
	if h.Mimes[0] != MimeHTML || !h.Has(MimeText) {
		t.Errorf("html mimes = %v", h.Mimes)
	}
	m := Merge(Text("first"), Text("second"))
	if len(m.Mimes) != len(TextMimes) {
		t.Errorf("merge duplicated mimes: %v", m.Mimes)
	}
	if b, _ := m.Bytes(MimeText); string(b) != "first" {
		t.Errorf("merge served %q, want the first part", b)
	}
	if b, _ := m.Bytes("application/none"); len(b) != 0 {
		t.Error("merge wrote for a mime it lacks")
	}
	if !(Content{}).Empty() || c.Empty() {
		t.Error("Empty")
	}
}

// Files round-trip through a uri list to the same absolute paths;
// remote uris and comments drop out.
func TestURIList(t *testing.T) {
	c := Files("/tmp/a b.txt", "/srv/x")
	list, _ := c.Bytes(MimeURIList)
	if string(list) != "file:///tmp/a%20b.txt\r\nfile:///srv/x\r\n" {
		t.Errorf("uri list = %q", list)
	}
	if plain, _ := c.Bytes(MimeText); string(plain) != "file:///tmp/a%20b.txt\nfile:///srv/x" {
		t.Errorf("plain = %q", plain)
	}
	uris := ParseURIList(append([]byte("# comment\r\n\r\nhttps://example.com/\r\n"), list...))
	if len(uris) != 3 {
		t.Fatalf("parsed %v", uris)
	}
	if got := FilePaths(uris); !slices.Equal(got, []string{"/tmp/a b.txt", "/srv/x"}) {
		t.Errorf("paths = %v", got)
	}
	if got := FilePaths([]string{"file://otherhost/x", "file://localhost/y"}); !slices.Equal(got, []string{"/y"}) {
		t.Errorf("hosts: %v", got)
	}
}

// HTML decodes as UTF-8 (BOM dropped) or as BOM-marked UTF-16.
func TestDecodeHTML(t *testing.T) {
	if got := DecodeHTML([]byte("\xEF\xBB\xBF<p>æ</p>")); got != "<p>æ</p>" {
		t.Errorf("utf-8 = %q", got)
	}
	le := []byte{0xFF, 0xFE, '<', 0, 'p', 0, '>', 0, 0xE6, 0}
	if got := DecodeHTML(le); got != "<p>æ" {
		t.Errorf("utf-16le = %q", got)
	}
	be := []byte{0xFE, 0xFF, 0, '<', 0, 0xE6}
	if got := DecodeHTML(be); got != "<æ" {
		t.Errorf("utf-16be = %q", got)
	}
}

// Image offers an exact PNG and a JPEG flattened onto white.
func TestImage(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255}) // the rest transparent
	c, err := Image(img)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.Mimes, []string{MimePNG, MimeJPEG}) {
		t.Errorf("mimes = %v", c.Mimes)
	}
	p, _ := c.Bytes(MimePNG)
	if dec, err := png.Decode(bytes.NewReader(p)); err != nil || dec.Bounds() != img.Bounds() {
		t.Errorf("png: %v", err)
	}
	j, _ := c.Bytes(MimeJPEG)
	dec, err := jpeg.Decode(bytes.NewReader(j))
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := dec.At(7, 7).RGBA(); r>>8 < 240 || g>>8 < 240 || b>>8 < 240 {
		t.Errorf("transparent pixel flattened to %d,%d,%d, want white", r>>8, g>>8, b>>8)
	}
	if _, err := Image(image.NewRGBA(image.Rect(0, 0, 0, 0))); err == nil {
		t.Error("empty image made content")
	}
}

// Prefer takes the first offered preference, then copy, then move.
func TestPrefer(t *testing.T) {
	both := ActionCopy | ActionMove
	cases := []struct {
		offered Action
		prefs   []Action
		want    Action
	}{
		{both, nil, ActionCopy},
		{both, []Action{ActionMove}, ActionMove},
		{ActionMove, nil, ActionMove},
		{ActionAsk, nil, ActionNone},
		{ActionAsk | ActionMove, []Action{ActionAsk}, ActionAsk},
	}
	for _, c := range cases {
		if got := Prefer(c.offered, c.prefs...); got != c.want {
			t.Errorf("Prefer(%d, %v) = %d, want %d", c.offered, c.prefs, got, c.want)
		}
	}
	if (Drag{}).Offered() != ActionCopy || (Drag{Actions: ActionMove}).Offered() != ActionMove {
		t.Error("Offered")
	}
	if (Feedback{}).Accepted() || !(Feedback{Mime: "a/b"}).Accepted() {
		t.Error("Accepted")
	}
}
