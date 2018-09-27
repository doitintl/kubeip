.PHONY: default build image test stop clean-image clean

BINARY = kubeip


GOCMD = go
GOFLAGS ?= $(GOFLAGS:)
LDFLAGS =

BUILD_DATE=`date +%Y-%m-%d\-%H:%M`
VERSION=`git log | head -n 1 | cut  -f 2 -d ' '`

default: build test

build:
	"$(GOCMD)" build ${GOFLAGS} ${LDFLAGS} -o "${BINARY}"

image:
	@docker build -t "${BINARY}" --build-arg VERSION=${VERSION} --build-arg BUILD_DATE=${BUILD_DATE} -f Dockerfile .

test:
	"$(GOCMD)" test -race -v $(shell go list ./... | grep -v '/vendor/')

stop:
	@docker stop "${BINARY}" || true # Do not fail if container does not exist

clean-image: stop
	@docker rmi "${BINARY}" || true # Do not fail if image does not exist

clean:
	"$(GOCMD)" clean -i
