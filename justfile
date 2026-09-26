# gelm dev tasks.

# List all recipes with their descriptions.
default:
    @just --list

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

# Run the layer-shell panel (gelm-panel): right-anchored widgets with live input.
panel:
    go run ./cmd/gelm-panel

# Run the layer-shell status bar (gelm-bar).
bar:
    go run ./cmd/gelm-bar
