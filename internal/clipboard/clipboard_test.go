package clipboard

import (
	"errors"
	"os"
	"testing"

	"github.com/neurlang/wayland/wl"

	"github.com/stubbedev/gelm/internal/wlsession"
)

func TestPickTextMime(t *testing.T) {
	t.Run("prefers utf-8 plain text", func(t *testing.T) {
		got := pickTextMime(func(m string) bool {
			return m == "text/plain" || m == "text/plain;charset=utf-8"
		})
		if got != "text/plain;charset=utf-8" {
			t.Errorf("mime = %q, want the utf-8 variant", got)
		}
	})

	t.Run("falls back to plain text", func(t *testing.T) {
		got := pickTextMime(func(m string) bool { return m == "text/plain" })
		if got != "text/plain" {
			t.Errorf("mime = %q, want text/plain", got)
		}
	})

	t.Run("none of the text mimes gives empty", func(t *testing.T) {
		if got := pickTextMime(func(string) bool { return false }); got != "" {
			t.Errorf("mime = %q, want empty", got)
		}
	})
}

func TestClipboardUnavailableWithoutOffer(t *testing.T) {
	c := &Clipboard{sess: &wlsession.Session{}}
	if _, err := c.ReadText(); !errors.Is(err, ErrUnavailable) {
		t.Errorf("ReadText = %v, want ErrUnavailable", err)
	}
}

func TestSendHandlerWritesAndCloses(t *testing.T) {
	c := &Clipboard{out: "clipboard payload"}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	c.HandleDataSourceSend(wl.DataSourceSendEvent{MimeType: "text/plain;charset=utf-8", Fd: w.Fd(), FdError: nil})
	w.Close()

	buf := make([]byte, 64)
	n, _ := r.Read(buf)
	if string(buf[:n]) != "clipboard payload" {
		t.Errorf("received %q, want the clipboard payload", string(buf[:n]))
	}
	// The handler must have closed its fd: a second read gives EOF.
	if _, err := r.Read(buf); err == nil {
		t.Error("fd still open after Send")
	}
}
