# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GORUN=$(GOCMD) run
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOTOOL=$(GOCMD) tool
GOLINT=golangci-lint
GOMOCK=mockery
LINT_CONFIG = $(CURDIR)/.golangci.yaml

BIN=$(CURDIR)/.bin
BINARY_NAME=kubeip-agent
TARGETOS   := $(or $(TARGETOS), linux)
TARGETARCH := $(or $(TARGETARCH), amd64)

DATE    ?= $(shell date +%FT%T%z)

# get version from environment variable if set or use git describe (match SemVer)
VERSION := $(if $(VERSION),$(VERSION),$(shell git describe --tags --always --dirty --match="[0-9]*.[0-9]*.[0-9]*" 2> /dev/null || \
			cat $(CURDIR)/.version 2> /dev/null || echo v0))

# get commit from environment variable if set or use git commit
COMMIT := $(if $(COMMIT),$(COMMIT),$(shell git rev-parse --short HEAD 2>/dev/null))
# get branch from environment variable if set or use git branch
BRANCH := $(if $(BRANCH),$(BRANCH),$(shell git rev-parse --abbrev-ref HEAD 2>/dev/null))

Q = $(if $(filter 1,$V),,@)
M = $(shell printf "\033[34;1m▶\033[0m")

export CGO_ENABLED=0
export GOOS=$(TARGETOS)
export GOARCH=$(TARGETARCH)

# main task
all: lint test build ; $(info $(M) build, test and deploy ...) @ ## release cycle

# Tools
setup-lint:
	$(GOCMD) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.57.1
setup-mockery:
	$(GOCMD) install github.com/vektra/mockery/v2@v2.35.2

# Tasks

build: ; $(info $(M) building $(GOOS)/$(GOARCH) binary...) @ ## build with local Go SDK
	$(GOBUILD) -v \
	-tags release \
	-ldflags '-s -w -X main.version=$(VERSION) -X main.buildDate=$(DATE) -X main.gitCommit=$(COMMIT) -X main.gitBranch=$(BRANCH)' \
	-o $(BIN)/$(BINARY_NAME) ./cmd/.

lint: setup-lint; $(info $(M) running golangci-lint ...) @ ## run golangci-lint linters
	# updating path since golangci-lint is looking for go binary and this may lead to
	# conflict when multiple go versions are installed
	$Q $(GOLINT) run -v -c $(LINT_CONFIG) --out-format checkstyle ./... > golangci-lint.out

mock: setup-mockery ; $(info $(M) running mockery ...) @ ## run mockery to generate mocks
	$Q $(GOMOCK) --dir internal --all --keeptree --with-expecter --exported

test: ; $(info $(M) running test ...) @ ## run tests with coverage
	$Q $(GOCMD) fmt ./...
	$Q $(GOTEST) -v -cover ./... -coverprofile=coverage.out
	$Q $(GOTOOL) cover -func=coverage.out

test-json: ; $(info $(M) running test output JSON ...) @ ## run tests with JSON report and coverage
	$Q $(GOTEST) -v -cover ./... -coverprofile=coverage.out -json > test-report.out

precommit: lint test ; $(info $(M) test and lint ...) @ ## release cycle: test > lint

testview: ; $(info $(M) generating coverage report ...) @ ## generate HTML coverage report
	$(GOTOOL) cover -html=coverage.out

clean: ; $(info $(M) cleaning...)	@ ## cleanup everything
	$Q $(GOCLEAN)
	@rm -rf $(BIN)
	@rm -rf test/tests.* test/coverage.*

run: ; $(info $(M) running ...) @ ## run locally
	$Q $(GORUN) -v cmd/main.go

help: ## display help
	@grep -E '^[ a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'
