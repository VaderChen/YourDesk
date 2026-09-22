.PHONY: test build build-all clean

TARGET := $(shell go env GOOS)/$(shell go env GOARCH)
DESKTOP := $(filter darwin/% windows/%,$(TARGET))

test:
ifneq ($(DESKTOP),)
	python3 scripts/opus.py $(TARGET) go test ./...
else
	go test ./...
endif

build:
	mkdir -p bin
ifneq ($(DESKTOP),)
	python3 scripts/ffmpeg.py $(TARGET) go build -o bin/yourdesk-client ./cmd/client
	python3 scripts/ffmpeg.py $(TARGET) go build -o bin/yourdesk-remote ./cmd/remote
else
	go build -o bin/yourdesk-client ./cmd/client
	go build -o bin/yourdesk-remote ./cmd/remote
endif

build-all:
	./scripts/build-all.sh

clean:
	rm -rf bin dist
