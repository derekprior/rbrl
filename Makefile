.PHONY: build test vet fmt clean

build: vet
	go build -o bin/rbrl ./cmd/rbrl/

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

fmt-check:
	@test -z "$$(gofmt -l .)" || (echo "Files not formatted:"; gofmt -l .; exit 1)

clean:
	rm -rf bin/
