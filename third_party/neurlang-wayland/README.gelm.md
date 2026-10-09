# github.com/neurlang/wayland v0.4.4, patched for gelm

A trimmed copy (only the packages gelm imports) of
github.com/neurlang/wayland v0.4.4, MIT licensed (LICENSE), carried as
packages of the gelm module itself and imported as
`github.com/stubbedev/gelm/third_party/neurlang-wayland/...`.

It was first wired in with a go.mod `replace`, but Go ignores a
dependency's `replace` directives: every consumer of gelm (wayle)
silently built against the unpatched upstream binding. Folding the
copy into gelm's module is what makes the fix reach them. The
upstream module stays required only as the `tool` that `go generate`
runs for the protocol bindings (wlr/types.go rewrites the generated
imports to this path).

Local changes beyond the patch: the amd64 swizzle assembly names its
frame slots after the Go declaration (`p_base`, `p_len`) so go vet's
asmdecl check passes; no instruction changed.

## The patch

`wl/event.go`, `readEvent`: socket control messages (the fds a
compositor passes - keymaps, clipboard pipes) were parsed into
`ctx.scms` as slices of the read's `control` buffer, which then went
back to `bytePool` and was reused by the next read. An fd still queued
for a later event decoded from whatever bytes the reuse left there:
garbage fds such as 1162944558, `EBADF` on use. Under Hyprland's timing
it hit the keymap fd of most clients - no keymap, every key NoSymbol.
The patch copies each parsed message's data before the buffer returns
to the pool (and parses only the `oobn` bytes actually read).

## Leaving

Drop the copy (rewriting the imports back) once upstream carries the
fix; gelm's headless suites (sway and the Hyprland VM gate) are the
regression check.
