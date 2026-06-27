B=$(shell git rev-parse --abbrev-ref HEAD)
BRANCH=$(subst /,-,$(B))
GITREV=$(shell git describe --abbrev=7 --always --tags)
REV=$(GITREV)-$(BRANCH)-$(shell date +%Y%m%d-%H:%M:%S)

all: build

## run: run the app locally (set YNAB_TOKEN in env or .env before running)
.PHONY: run
run:
	go run -ldflags "-X main.revision=dev" ./cmd/ynab-helper --addr=:5002

## build: compile binary for linux/amd64
.PHONY: build
build: info
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-X main.revision=$(REV)" -o target/ynab-helper ./cmd/ynab-helper

## docker: build Docker image
.PHONY: docker
docker:
	- docker build -t ynab-helper:$(BRANCH) . --platform linux/amd64

# ==================================================================================== #
# QUALITY CONTROL
# ==================================================================================== #

## check: format, vet, lint and test all code
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

## lint: run golangci-lint
.PHONY: lint
lint:
	golangci-lint run --output.tab.path=stdout ./...

## test: run tests with race detector and coverage
.PHONY: test
test:
	go test -race -vet=off -coverprofile=cover.out ./...
	make test.coverage

## test.coverage: print test coverage report
.PHONY: test.coverage
test.coverage:
	go tool cover -func=cover.out

info:
	- @echo "revision $(REV)"

.PHONY: info
