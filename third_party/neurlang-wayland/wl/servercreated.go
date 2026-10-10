package wl

// NewServerBuffer returns a Buffer for an id the compositor creates
// and announces in an event (zwp_linux_buffer_params_v1.created),
// initialized like NewBuffer's so handlers can be added to it; the
// event's NewId registers it under the server's id.
func NewServerBuffer() *Buffer {
	ret := new(Buffer)
	ret.initBuffer()
	return ret
}
