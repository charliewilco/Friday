default: check

fmt:
	find . -type f -name '*.go' -not -path './vendor/*' -exec gofmt -w {} +

fmt-check:
	find . -type f -name '*.go' -not -path './vendor/*' -exec gofmt -l {} + | tee /tmp/friday-gofmt.txt >/dev/null; test ! -s /tmp/friday-gofmt.txt

test:
	go test ./...

build:
	go build ./...

check: fmt-check test build
