// Package inproc runs gelm applications inside a test process against
// the headless compositor and inspects the output through screencopy:
// the in-process half of the compositor-in-the-loop suite, for tests
// that drive the app API directly instead of a demo client.
package inproc

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"testing"
	"time"

	"github.com/stubbedev/gelm/app"
	"github.com/stubbedev/gelm/capture"
	"github.com/stubbedev/gelm/render"
)

// Require skips unless the test runs under just headless.
func Require(t *testing.T) {
	t.Helper()
	if os.Getenv("GELM_HEADLESS") == "" {
		t.Skip("GELM_HEADLESS is not set; in-process compositor tests run under just headless")
	}
}

// Show runs a gelm application on its own goroutine with build adding
// its surfaces, and stops it at test cleanup.
func Show(t *testing.T, build func(a *app.Application, out *app.Output) error) {
	t.Helper()
	sess, err := app.Connect()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	a := app.NewApplication(sess)
	outs := sess.Outputs()
	if len(outs) == 0 {
		sess.Close()
		t.Fatal("no outputs")
	}
	if err := build(a, outs[0]); err != nil {
		sess.Close()
		t.Fatalf("build: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = a.Run()
	}()
	t.Cleanup(func() {
		a.Invoke(a.Quit)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("application did not quit")
		}
		sess.Close()
	})
}

// Capture connects a screencopy client, closed at test cleanup.
func Capture(t *testing.T) *capture.Client {
	t.Helper()
	c, err := capture.Connect()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if len(c.Outputs()) == 0 {
		t.Fatal("no outputs")
	}
	return c
}

// Eventually retries fn until it succeeds or the deadline passes.
func Eventually(t *testing.T, what string, fn func() error) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	var err error
	for time.Now().Before(deadline) {
		if err = fn(); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("%s: %v", what, err)
}

// WantColor checks one pixel of a captured frame.
func WantColor(img *image.RGBA, x, y int, want render.Color) error {
	got := img.RGBAAt(x, y)
	w := color.RGBA{R: uint8(want >> 16), G: uint8(want >> 8), B: uint8(want), A: 0xff}
	if got != w {
		return fmt.Errorf("pixel (%d,%d) = %v, want %v", x, y, got, w)
	}
	return nil
}
