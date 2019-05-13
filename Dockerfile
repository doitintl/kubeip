
FROM golang:1.12-alpine AS build_base
LABEL maintainer="Aviv Laufer <aviv.laufer@gmail.com>"
RUN apk update && apk upgrade && \
    apk add --no-cache git build-base ca-certificates

WORKDIR /go/src/github.com/doitintl/kubeip
ENV GO111MODULE=on
COPY go.mod .
COPY go.sum .
RUN go mod download

FROM build_base AS builder
COPY . .

RUN cd /go/src/github.com/doitintl/kubeip && GO111MODULE=on CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a --installsuffix cgo --ldflags="-s"  -ldflags "-X main.version=$(git log | head -n 1 | cut  -f 2 -d ' ') -X main.build_date=$(date +%Y-%m-%d\-%H:%M)" -o /kubeip

FROM alpine
RUN apk add --no-cache ca-certificates

COPY --from=builder /kubeip /bin/kubeip

ENTRYPOINT ["/bin/kubeip"]
