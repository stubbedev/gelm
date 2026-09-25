// Package buffer owns the wl_shm buffer lifecycle for one surface: a small
// pool of ARGB8888 buffers so the shell never draws into a buffer the
// compositor is still reading from.
package buffer

import (
	"errors"
	"fmt"
	"math"

	"github.com/neurlang/wayland/os"
	"github.com/neurlang/wayland/wl"
)

// ErrBusy reports that every buffer in the pool is held by the compositor.
// The caller skips the frame and retries after the next release event or
// frame callback.
var ErrBusy = errors.New("buffer: all pool buffers busy")

// BytesPerPixel is the pixel size of the pool's format, ARGB8888
// (premultiplied alpha, little endian).
const BytesPerPixel = 4

// Stride returns the row stride in bytes for a width in buffer pixels.
func Stride(widthPx int) int {
	return widthPx * BytesPerPixel
}

// Buffer is one wl_shm-backed pixel buffer.
type Buffer struct {
	// WL is the wayland buffer object; nil only in tests of pool
	// bookkeeping.
	WL *wl.Buffer

	// Data is the mmap view of the buffer: Height rows of Stride bytes,
	// ARGB8888 premultiplied, byte order B, G, R, A.
	Data []byte

	// Width and Height are in buffer pixels (output scale already
	// applied).
	Width, Height int
	Stride        int
	Scale         int

	busy bool
}

// Release marks the buffer free for the next frame. Only the wayland
// release event handler should call it; calling it earlier allows drawing
// into a buffer the compositor may still read.
func (b *Buffer) Release() {
	b.busy = false
}

// Busy reports whether the compositor still holds the buffer.
func (b *Buffer) Busy() bool {
	return b.busy
}

// NewFile allocates a memfd-backed wl_buffer of bufW x bufH buffer pixels
// at the given integer output scale. The wl_shm pool object is destroyed
// immediately; the mapping keeps the pages alive. The mapping is never
// unmapped (the neurlang os package exposes no munmap); pools hold a fixed
// handful of buffers, so the leak is bounded by pool capacity times
// resizes.
func NewFile(shm *wl.Shm, bufW, bufH, scale int) (*Buffer, error) {
	if bufW <= 0 || bufH <= 0 {
		return nil, fmt.Errorf("buffer: invalid size %dx%d", bufW, bufH)
	}
	if scale < 1 {
		return nil, fmt.Errorf("buffer: invalid scale %d", scale)
	}
	stride := Stride(bufW)
	size := stride * bufH
	if size > math.MaxInt32 {
		return nil, fmt.Errorf("buffer: size %d exceeds the wayland int32 range", size)
	}

	fd, err := os.CreateAnonymousFile(int64(size))
	if err != nil {
		return nil, fmt.Errorf("buffer: memfd: %w", err)
	}
	defer fd.Close()

	data, err := os.Mmap(int(fd.Fd()), 0, size, os.ProtRead|os.ProtWrite, os.MapShared)
	if err != nil {
		return nil, fmt.Errorf("buffer: mmap: %w", err)
	}

	pool, err := shm.CreatePool(fd.Fd(), int32(size))
	if err != nil {
		return nil, fmt.Errorf("buffer: wl_shm.create_pool: %w", err)
	}
	wlBuf, err := pool.CreateBuffer(0, int32(bufW), int32(bufH), int32(stride), wl.ShmFormatArgb8888)
	if err != nil {
		return nil, fmt.Errorf("buffer: wl_shm_pool.create_buffer: %w", err)
	}
	if err := pool.Destroy(); err != nil {
		return nil, fmt.Errorf("buffer: wl_shm_pool.destroy: %w", err)
	}

	return &Buffer{WL: wlBuf, Data: data, Width: bufW, Height: bufH, Stride: stride, Scale: scale}, nil
}

// Pool is a fixed-capacity set of buffers for one surface.
type Pool struct {
	// create allocates a buffer at the pool's current size; swapped out
	// by Resize.
	create   func() (*Buffer, error)
	capacity int
	buffers  []*Buffer
}

// New returns a pool holding at most capacity buffers, allocated on demand
// through create. A capacity below 1 is treated as 1.
func New(create func() (*Buffer, error), capacity int) *Pool {
	if capacity < 1 {
		capacity = 1
	}
	return &Pool{create: create, capacity: capacity}
}

// Acquire returns a buffer to draw the next frame into and marks it busy.
// Free buffers are reused before new ones are allocated. ErrBusy means
// every buffer is held by the compositor: skip the frame and retry after a
// release event. Allocation failures are returned verbatim.
func (p *Pool) Acquire() (*Buffer, error) {
	for _, b := range p.buffers {
		if !b.busy {
			return b, nil
		}
	}
	if len(p.buffers) < p.capacity {
		b, err := p.create()
		if err != nil {
			return nil, err
		}
		b.busy = true
		p.buffers = append(p.buffers, b)
		return b, nil
	}
	return nil, ErrBusy
}

// Resize drops every buffer, busy or not, and installs create as the
// allocator for the next Acquire. Destroying a wl_buffer the compositor
// still holds is allowed; the release events of dropped buffers are simply
// never delivered.
func (p *Pool) Resize(create func() (*Buffer, error)) {
	for _, b := range p.buffers {
		if b.WL != nil {
			_ = b.WL.Destroy()
		}
	}
	p.buffers = nil
	p.create = create
}

// ReleaseHandler adapts a Buffer to the wayland release event so the pool
// refills as the compositor returns buffers.
type ReleaseHandler struct {
	B *Buffer
}

// HandleBufferRelease implements wl.BufferReleaseHandler.
func (h ReleaseHandler) HandleBufferRelease(wl.BufferReleaseEvent) {
	h.B.Release()
}
