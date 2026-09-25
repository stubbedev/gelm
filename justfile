default: check

check: lint test

lint:
    go vet ./...

test:
    go test ./...

fmt:
    gofmt -l -w .

demo:
    go run ./cmd/gelm-bar
