VERSION := $(shell git describe --always --tags --dirty)

.PHONY: build
build:
	go build -v -ldflags="-X main.version=${VERSION}" -o pivotal-slack-preview

.PRONY: build_image_latest
build_image_latest:
	docker build -f build/Dockerfile -t flexoid/pivotal-slack-preview .

.PHONY: run
run: build
	./pivotal-slack-preview

.PHONY: test
test:
	go test ./... -coverprofile=coverage.out -coverpkg=./...
	go tool cover -func coverage.out

.PHONY: lint
lint:
	golangci-lint run

.PHONY: coverage_report
coverage_report:
	go test ./... -coverprofile=coverage.out -coverpkg=./...
	go tool cover -html coverage.out -o coverage.html
