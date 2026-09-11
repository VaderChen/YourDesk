.PHONY: test build build-all clean

test:
	go test ./...

build:
	mkdir -p bin
	go build -o bin/yourdesk-client ./cmd/client
	go build -o bin/yourdesk-remote ./cmd/remote

build-all:
	./scripts/build-all.sh

clean:
	rm -rf bin dist
