# syntax = docker/dockerfile:experimental

#
# ----- Go Builder Image ------
#
FROM golang:1.17-alpine AS builder
# curl git bash
RUN apk add --no-cache curl git bash make
#
# ----- build and test -----
#
FROM builder as build
# set working directorydoc
RUN mkdir -p /go/src/app
WORKDIR /go/src/app
# load dependency
COPY go.mod .
COPY go.sum .
RUN --mount=type=cache,target=$GOPATH/pkg/mod go mod download
# copy sources
COPY . .
# build
RUN make binary

#
# ------ release Docker image ------
#
FROM scratch
# copy CA certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
# this is the last command since it's never cached
COPY --from=build /go/src/app/.bin/github.com/doitintl/kubeip /kubeip

ENTRYPOINT ["/kubeip"]


# RUN cd /go/src/github.com/doitintl/kubeip && GO111MODULE=on CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a --installsuffix cgo --ldflags="-s"  -ldflags "-X main.version=$(git log | head -n 1 | cut  -f 2 -d ' ') -X main.buildDate=$(date +%Y-%m-%d\-%H:%M)" -o /kubeip
