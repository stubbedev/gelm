# github.com/neurlang/wayland v0.4.4, patched for gelm

A trimmed copy (only the packages gelm imports) of
github.com/neurlang/wayland v0.4.4, MIT licensed (LICENSE), carried as
packages of the gelm module itself and imported as
`github.com/stubbedev/gelm/third_party/neurlang-wayland/...`.

## Why it lives inside the module

Go ignores a dependency's `replace` directives, so a patched copy
wired in through a go.mod `replace` would reach gelm's own builds and
none of its consumers': every application importing gelm would build
against the unpatched upstream binding. As packages of the gelm module,
the patches reach every consumer. The upstream module stays required
only as the `tool` that `go generate` runs for the protocol bindings;
`wlr/types.go` rewrites the generated imports to this path.

## Patches

### Socket control messages outlive the read buffer

`wl/event.go`, `readEvent`: the socket control messages a compositor
sends (the fds for keymaps and clipboard pipes) are parsed out of the
read's `control` buffer, which goes back to `bytePool` for the next
read. Upstream keeps the parsed messages as slices of that buffer, so
an fd still queued for a later event decodes from whatever bytes the
next read left there: a garbage fd, and `EBADF` on use. Under
Hyprland's timing that hits the keymap fd of most clients, leaving
them with no keymap. The patch copies each parsed message's data
before the buffer returns to the pool, and parses only the `oobn`
bytes actually read.

### Server-created buffers

`wl/servercreated.go` adds `NewServerBuffer`, a `Buffer` initialized
like `NewBuffer`'s but not registered, for ids the compositor creates
in an event (`zwp_linux_buffer_params_v1.created`). The generated wlr
binding cannot call the unexported `initBuffer`, and a bare `Buffer`
has a nil handler map, so `AddReleaseHandler` on it panics. The
linux-dmabuf `go:generate` line in `wlr/types.go` rewrites the
generated constructor to use it.

### go vet clean assembly

The amd64 swizzle assembly names its frame slots after the Go
declaration (`p_base`, `p_len`) so go vet's asmdecl check passes; no
instruction differs from upstream.

## Upstream status

The fd fix is merged upstream (neurlang/wayland#41, e9df493) but not
yet in a tagged release. `NewServerBuffer` is proposed upstream as
neurlang/wayland#42. The copy goes away, with the imports
rewritten back to github.com/neurlang/wayland, once upstream carries
both; gelm's compositor-in-the-loop suites on sway and Hyprland are
the regression check.
