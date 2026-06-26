B=$(shell git rev-parse --abbrev-ref HEAD)
BRANCH=$(subst /,-,$(B))
GITREV=$(shell git describe --abbrev=7 --always --tags)
REV=$(GITREV)-$(BRANCH)-$(shell date +%Y%m%d-%H:%M:%S)

all: build docker

build: info
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-X main.revision=$(REV)" -o target/ynab-helper ./cmd/ynab-helper

docker:
	- docker build -t ynab-helper:$(BRANCH) . --platform linux/amd64

push:
	- docker secrets:${BRANCH}

check2:
	golangci-lint run --output.tab.path=stdout ./...

# ==================================================================================== #
# QUALITY CONTROL
# ==================================================================================== #

## audit: tidy and vendor dependencies and format, vet and test all code
.PHONY: check
check:
	@echo 'Formatting code...'
	go fmt ./...

	@echo 'Vetting code...'
	go vet ./...

	@echo 'Running linter'
	make lint

	@echo 'Running tests...'
	make test

## lint: lint runs a lint check on the source code.
.PHONY: lint
lint:
	golangci-lint run --output.tab.path=stdout ./...

## test: run the tests
.PHONY: test
test:
	go test -race -vet=off -coverprofile=cover.out ./...
	make test.coverage

## test.coverage: shows the test coverage
.PHONY: test.coverage
test.coverage:
	go tool cover -func=cover.out

info:
	- @echo "revision $(REV)"

.PHONY: bin info docker
