package app

import (
	"errors"
	"image"
	"testing"

	"github.com/stubbedev/gelm/internal/wlsession"
)

// NewClipboard is the public way in for consumers outside the module:
// the app hands it back from Clipboard once installed, and a session
// without a data device reports the clipboard unavailable instead of
// crashing on a programmatic copy.
func TestClipboardInstalledAndUnavailable(t *testing.T) {
	a := NewApplication(&wlsession.Session{})
	if a.Clipboard() != nil {
		t.Fatal("a fresh application has a clipboard")
	}
	c := NewClipboard(&wlsession.Session{})
	a.SetClipboard(c)
	if a.Clipboard() != c {
		t.Fatal("Clipboard does not return the installed clipboard")
	}
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	if err := a.Clipboard().WriteImage(img); !errors.Is(err, ErrClipboardUnavailable) {
		t.Fatalf("WriteImage without a data device = %v, want ErrUnavailable", err)
	}
}
