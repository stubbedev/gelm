# gelm dev tasks.

default:
    @just --list

# vet + lint + test + build (the release gates).
check: vet lint test build

vet:
    go vet ./...

# golangci-lint against .golangci.yml (strict set: gosec, errorlint,
# perfsprint, revive, ...; gofumpt + gci formatters).
lint:
    golangci-lint run

test:
    go test ./...

build:
    go build -o /dev/null ./cmd/gelm-bar

fmt:
    gofmt -w .

# The showcase window: every widget in one toplevel (clicks, drag,
# tooltips, right-click menu, Tab focus, esc to close).
demo:
    go run ./cmd/gelm-hello

# The layer-shell panel: right-anchored widgets with live input.
panel:
    go run ./cmd/gelm-panel

# The status bar.
bar:
    go run ./cmd/gelm-bar
