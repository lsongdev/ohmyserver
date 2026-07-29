BINARY_NAME=bin/ohmyserver
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --verify HEAD 2>/dev/null || printf unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS = -s -w \
	-X github.com/lsongdev/ohmyserver/buildinfo.Version=$(VERSION) \
	-X github.com/lsongdev/ohmyserver/buildinfo.Commit=$(COMMIT) \
	-X github.com/lsongdev/ohmyserver/buildinfo.BuildTime=$(BUILD_TIME)

build:
	mkdir -p bin
	CGO_ENABLED=0 GOARCH=arm64 GOOS=darwin go build -ldflags="$(LDFLAGS)" -o ${BINARY_NAME}-darwin-arm64
	CGO_ENABLED=0 GOARCH=amd64 GOOS=darwin go build -ldflags="$(LDFLAGS)" -o ${BINARY_NAME}-darwin-amd64
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -ldflags="$(LDFLAGS)" -o ${BINARY_NAME}-linux-amd64
	CGO_ENABLED=0 GOARCH=arm64 GOOS=linux go build -ldflags="$(LDFLAGS)" -o ${BINARY_NAME}-linux-arm64
	CGO_ENABLED=0 GOARCH=amd64 GOOS=windows go build -ldflags="$(LDFLAGS)" -o ${BINARY_NAME}-windows-amd64.exe
	cd bin && if command -v sha256sum >/dev/null 2>&1; then sha256sum ohmyserver-*; else shasum -a 256 ohmyserver-*; fi > checksums.txt

docker:
	docker compose build

docker-run:
	docker compose up -d

docker-stop:
	docker compose down

clean:
	go clean
	rm -f ${BINARY_NAME}-darwin-arm64 ${BINARY_NAME}-darwin-amd64 ${BINARY_NAME}-linux-amd64 ${BINARY_NAME}-linux-arm64 ${BINARY_NAME}-windows-amd64.exe
	rm -f bin/checksums.txt

.PHONY: build docker docker-run docker-stop clean
