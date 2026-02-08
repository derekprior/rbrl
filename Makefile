.PHONY: build test vet clean

build: vet
	go build -o bin/rbrl ./cmd/rbrl/

test:
	go test -race ./...

vet:
	go vet ./...

clean:
	rm -rf bin/
