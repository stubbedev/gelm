package buffer

import (
	"errors"
	"testing"
)

func newTestBuffer(w, h int) *Buffer {
	stride := Stride(w)
	return &Buffer{
		Data:   make([]byte, stride*h),
		Width:  w,
		Height: h,
		Stride: stride,
		Scale:  1,
	}
}

func countingCreate(counter *int, w, h int) func() (*Buffer, error) {
	return func() (*Buffer, error) {
		*counter++
		return newTestBuffer(w, h), nil
	}
}

func TestAcquireAllocatesUpToCapacity(t *testing.T) {
	created := 0
	p := New(countingCreate(&created, 100, 20), 3)

	for i := range 3 {
		b, err := p.Acquire()
		if err != nil {
			t.Fatalf("acquire %d: %v", i, err)
		}
		if !b.Busy() {
			t.Error("acquired buffer must be busy")
		}
	}

	t.Run("capacity exhausted yields ErrBusy, not a busy buffer", func(t *testing.T) {
		b, err := p.Acquire()
		if !errors.Is(err, ErrBusy) {
			t.Errorf("want ErrBusy, got %v", err)
		}
		if b != nil {
			t.Error("ErrBusy must not return a buffer")
		}
	})
}

func TestReleasedBufferIsReusedBeforeAllocating(t *testing.T) {
	created := 0
	p := New(countingCreate(&created, 100, 20), 3)

	first, err := p.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	first.Release()

	second, err := p.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if created != 1 {
		t.Errorf("released buffer must be reused, created %d buffers", created)
	}
	if first != second {
		t.Error("release then acquire must return the same buffer")
	}
}

func TestCreateErrorIsReturnedVerbatim(t *testing.T) {
	want := errors.New("shm pool exploded")
	p := New(func() (*Buffer, error) { return nil, want }, 2)

	b, err := p.Acquire()
	if !errors.Is(err, want) {
		t.Errorf("want the create error, got %v", err)
	}
	if b != nil {
		t.Error("failed create must not return a buffer")
	}
}

func TestResizeDropsAllBuffers(t *testing.T) {
	created := 0
	create := countingCreate(&created, 100, 20)
	p := New(create, 3)

	a, err := p.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	a.Release()

	old := map[*Buffer]bool{a: true, b: true}

	p.Resize(countingCreate(&created, 200, 40))

	got, err := p.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if old[got] {
		t.Error("resize must not hand out a pre-resize buffer")
	}
	if created != 3 {
		t.Errorf("resize must allocate through the new create, created %d", created)
	}
	if got.Width != 200 || got.Height != 40 {
		t.Errorf("resized buffer has size %dx%d, want 200x40", got.Width, got.Height)
	}
}

func TestStride(t *testing.T) {
	if got := Stride(800); got != 3200 {
		t.Errorf("Stride(800) = %d, want 3200", got)
	}
	if got := Stride(0); got != 0 {
		t.Errorf("Stride(0) = %d, want 0", got)
	}
}

func TestNewFileRejectsBadGeometry(t *testing.T) {
	t.Run("zero and negative sizes", func(t *testing.T) {
		if _, err := NewFile(nil, 0, 10, 1); err == nil {
			t.Error("zero width must error before touching shm")
		}
		if _, err := NewFile(nil, 10, -5, 1); err == nil {
			t.Error("negative height must error before touching shm")
		}
	})

	t.Run("scale below one", func(t *testing.T) {
		if _, err := NewFile(nil, 10, 10, 0); err == nil {
			t.Error("scale 0 must error before touching shm")
		}
	})
}
