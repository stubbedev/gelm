# gelm dev tasks.

# List all recipes with their descriptions.
default:
    @just --list

# Directory for headless test sessions: compositor socket, client logs, pids.
test_dir := "/tmp/gelm-test-env"

# Run every release gate in order: vet, lint, test, build.
check: vet lint test build

# Static analysis of every package with go vet.
vet:
    go vet ./...

# Lint every package; settings live in .golangci.yml.
lint:
    golangci-lint run

# Run the test suite for every package.
test:
    go test ./...

# Compile-check the bar binary; the output is discarded.
build:
    go build -o /dev/null ./cmd/gelm-bar

# Format every Go source in place with gofmt.
fmt:
    gofmt -w .

# Run the widget showcase (gelm-hello): clicks, drag, tooltips, menu, Tab focus.
demo:
    go run ./cmd/gelm-hello

# Run the widget showcase inside the headless test compositor with
# input traces on; pair with the move/click/sweep recipes to drive it.
# GOELM_DEBUG picks trace categories (see internal/debug).
demo-headless GOELM_DEBUG="input,frame": test-env test-build
    #!/bin/sh
    dir="{{test_dir}}"
    . "$dir/client.env"
    export WAYLAND_DISPLAY XDG_RUNTIME_DIR="$dir" GOELM_DEBUG="{{GOELM_DEBUG}}"
    "$dir/gelm-hello" >"$dir/demo.log" 2>&1 &
    echo $! >"$dir/demo.pid"
    sleep 1
    if kill -0 "$(cat "$dir/demo.pid")" 2>/dev/null; then
        echo "demo running (pid $(cat "$dir/demo.pid")); traces: just test-log"
    else
        echo "demo died:"; cat "$dir/demo.log"; exit 1
    fi

# Build the headless test binaries: the traced showcase and wlpointer.
test-build:
    go build -tags gelmdebug -o {{test_dir}}/gelm-hello ./cmd/gelm-hello
    go build -o {{test_dir}}/wlpointer ./cmd/wlpointer

# Start the headless test compositor: sway on wlroots' headless backend
# in a private XDG_RUNTIME_DIR, so synthetic-input runs never touch the
# real desktop. Idempotent: a second call reports and exits.
test-env:
    #!/bin/sh
    dir="{{test_dir}}"
    if [ -f "$dir/sway.pid" ] && kill -0 "$(cat "$dir/sway.pid")" 2>/dev/null; then
        echo "test compositor already running in $dir"
        exit 0
    fi
    rm -rf "$dir"
    mkdir -p "$dir" && chmod 700 "$dir"
    printf '%s\n' 'output * mode 1280x800' \
        'default_border none' \
        'default_floating_border none' > "$dir/sway.cfg"
    XDG_RUNTIME_DIR="$dir" WLR_BACKENDS=headless WLR_LIBINPUT_NO_DEVICES=1 \
        WLR_RENDERER=pixman nix develop -c sway -c "$dir/sway.cfg" \
        >"$dir/sway.log" 2>&1 &
    echo $! >"$dir/sway.pid"
    sock=""
    for i in $(seq 1 50); do
        sock=$(ls "$dir"/wayland-[0-9]* 2>/dev/null | head -1)
        [ -n "$sock" ] && break
        sleep 0.2
    done
    if [ -z "$sock" ]; then
        echo "sway failed to start; log: $dir/sway.log"; exit 1
    fi
    echo "WAYLAND_DISPLAY=${sock##*/}" >"$dir/client.env"
    echo "test compositor up: $(cat "$dir/client.env") XDG_RUNTIME_DIR=$dir"

# Stop the headless test compositor and wipe its runtime dir.
test-env-stop:
    #!/bin/sh
    dir="{{test_dir}}"
    if [ -f "$dir/sway.pid" ]; then
        pid="$(cat "$dir/sway.pid")"
        kill "$pid" 2>/dev/null
        for i in $(seq 1 20); do
            kill -0 "$pid" 2>/dev/null || break
            sleep 0.1
        done
        kill -9 "$pid" 2>/dev/null
    fi
    pkill -f "$dir/gelm-hello" 2>/dev/null
    rm -rf "$dir"
    echo "test compositor stopped"

# Follow the headless demo's trace log.
test-log:
    tail -n 40 -f {{test_dir}}/demo.log

# Move the virtual pointer to compositor coordinates in the test env.
move x="220" y="120": test-env test-build
    env XDG_RUNTIME_DIR={{test_dir}} $(cat {{test_dir}}/client.env) {{test_dir}}/wlpointer move {{x}} {{y}}

# Click the virtual pointer at compositor coordinates (optional button code).
click x="220" y="120" button="272": test-env test-build
    env XDG_RUNTIME_DIR={{test_dir}} $(cat {{test_dir}}/client.env) {{test_dir}}/wlpointer click {{x}} {{y}} {{button}}

# Send a vertical wheel tick at compositor coordinates (positive down).
axis x="450" y="350" dy="60": test-env test-build
    env XDG_RUNTIME_DIR={{test_dir}} $(cat {{test_dir}}/client.env) {{test_dir}}/wlpointer axis {{x}} {{y}} {{dy}}

# Sweep the pointer along a row (y0 == y1) or column through the test env.
sweep x0="100" y0="100" x1="600" y1="100" step="40": test-env test-build
    env XDG_RUNTIME_DIR={{test_dir}} $(cat {{test_dir}}/client.env) {{test_dir}}/wlpointer sweep {{x0}} {{y0}} {{x1}} {{y1}} {{step}}

# Run the layer-shell panel (gelm-panel): right-anchored widgets with live input.
panel:
    go run ./cmd/gelm-panel

# Run the layer-shell status bar (gelm-bar).
bar:
    go run ./cmd/gelm-bar
