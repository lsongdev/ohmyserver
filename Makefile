BINARY_NAME=bin/ohmyserver

build:
	CGO_ENABLED=0 GOARCH=arm64 GOOS=darwin go build -ldflags="-s -w" -o ${BINARY_NAME}-darwin-arm64
	CGO_ENABLED=0 GOARCH=amd64 GOOS=darwin go build -ldflags="-s -w" -o ${BINARY_NAME}-darwin-amd64
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -ldflags="-s -w" -o ${BINARY_NAME}-linux-amd64
	CGO_ENABLED=0 GOARCH=arm64 GOOS=linux go build -ldflags="-s -w" -o ${BINARY_NAME}-linux-arm64
	CGO_ENABLED=0 GOARCH=amd64 GOOS=windows go build -ldflags="-s -w" -o ${BINARY_NAME}-windows-amd64.exe

docker:
	docker compose build

docker-run:
	docker compose up -d

docker-stop:
	docker compose down

clean:
	go clean
	rm -f ${BINARY_NAME}-darwin-arm64 ${BINARY_NAME}-darwin-amd64 ${BINARY_NAME}-linux-amd64 ${BINARY_NAME}-linux-arm64 ${BINARY_NAME}-windows-amd64.exe

.PHONY: build docker docker-run docker-stop clean
