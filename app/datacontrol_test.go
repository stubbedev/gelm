package app

import (
	"errors"
	"testing"

	"github.com/stubbedev/gelm/internal/wlsession"
)

func TestDataControlWithoutTheProtocolIsUnavailable(t *testing.T) {
	a := NewApplication(&wlsession.Session{})

	d, err := a.DataControl()

	if !errors.Is(err, ErrDataControlUnavailable) || d != nil {
		t.Fatalf("DataControl() = %v, %v; want nil, ErrDataControlUnavailable", d, err)
	}
	// A failure is not cached as a device.
	if a.dataControl != nil {
		t.Fatal("a failed bind left a device behind")
	}
}
