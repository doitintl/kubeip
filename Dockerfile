FROM golang:alpine
LABEL maintainer="Aviv Laufer <aviv.laufer@gmail.com>"
RUN apk update && apk upgrade && \
    apk add --no-cache git build-base

ADD . /go/src/github.com/doitintl/kubeip

RUN cd /go/src/github.com/doitintl/kubeip && GO111MODULE=on CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a --installsuffix cgo --ldflags="-s"  -ldflags "-X main.version=$(git log | head -n 1 | cut  -f 2 -d ' ') -X main.build_date=$(date +%Y-%m-%d\-%H:%M)" -o /kubeip

FROM alpine:3.4
RUN apk add --no-cache ca-certificates

COPY --from=0 /kubeip /bin/kubeip

ENTRYPOINT ["/bin/kubeip"]
