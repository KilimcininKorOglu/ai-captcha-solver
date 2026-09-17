.PHONY: all test test-race vet fmt doc-check

all: vet test

test:
	go test -count=1 ./...

test-race:
	go test -count=1 -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

doc-check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "not gofmt-clean:"; echo "$$out"; exit 1; fi
	@echo "doc-check: all source canonically formatted"
