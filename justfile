# gelm dev tasks.

default:
    @just --list

# The release gates: vet + lint + test + build.
check: vet lint test build

vet:
    go vet ./...

# golangci-lint against .golangci.yml.
lint:
    golangci-lint run

test:
    go test ./...

build:
    go build -o /dev/null ./cmd/gelm-bar

fmt:
    gofmt -w .

# Every widget in one window: clicks, drag, tooltips, menu, Tab focus.
demo:
    go run ./cmd/gelm-hello

# The layer-shell panel: right-anchored widgets with live input.
panel:
    go run ./cmd/gelm-panel

# The status bar.
bar:
    go run ./cmd/gelm-bar
