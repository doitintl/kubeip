.PHONY: default build builder-image binary-image test stop clean-images clean

BUILDER = kubeip-builder
BINARY = kubeip


GOCMD = go
GOFLAGS ?= $(GOFLAGS:)
LDFLAGS =

BUILD_DATE=`date +%Y-%m-%d\-%H:%M`
VERSION=`git log | head -n 1 | cut  -f 2 -d ' '`

default: build test

build:
	"$(GOCMD)" build ${GOFLAGS} ${LDFLAGS} -o "${BINARY}"

    builder-image:
	@docker build -t "${BUILDER}" --build-arg VERSION=${VERSION} --build-arg BUILD_DATE=${BUILD_DATE}  -f Dockerfile.build .

binary-image: builder-image
	@docker run --rm "${BUILDER}" | docker build -t "${BINARY}" -f Dockerfile.run -

test:
	"$(GOCMD)" test -race -v $(shell go list ./... | grep -v '/vendor/')

stop:
	@docker stop "${BINARY}"

clean-images: stop
	@docker rmi "${BUILDER}" "${BINARY}"

clean:
	"$(GOCMD)" clean -i