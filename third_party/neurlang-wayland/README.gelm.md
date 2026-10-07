# github.com/neurlang/wayland v0.4.4, patched for gelm

A trimmed copy (only the packages gelm imports) of
github.com/neurlang/wayland v0.4.4, MIT licensed (LICENSE), wired in by
the `replace` directive in gelm's go.mod.

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

Drop the copy and the `replace` once upstream carries the fix; gelm's
headless suites (sway and the Hyprland VM gate) are the regression
check.
