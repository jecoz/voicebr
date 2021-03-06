VERSION          := $(shell git describe --tags --always --dirty="-dev")
COMMIT           := $(shell git rev-parse --short HEAD)
DATE             := $(shell date -u '+%Y-%m-%d-%H%M UTC')
VERSION_FLAGS    := -ldflags='-X "main.version=$(VERSION)" -X "main.commit=$(COMMIT)" -X "main.buildTime=$(DATE)"'

export GO111MODULE=on

.PHONY: all voicebr clean test format deploy
all: voicebr
voicebr:
	go build -v -o bin/voicebr $(VERSION_FLAGS)
clean:
	rm -rf bin/
	rm -rf dist/
test:
	go test ./...
format:
	go fmt ./...
