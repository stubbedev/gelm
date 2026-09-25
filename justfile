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

demo:
    go run ./cmd/gelm-bar
