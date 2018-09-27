FROM golang:alpine
MAINTAINER "Aviv Laufer <aviv.laufer@gmail.com>"
RUN apk add --no-cache git build-base && \
    mkdir -p "$GOPATH/src/github.com/doitintl/kubeip"

ADD . "$GOPATH/src/github.com/doitintl/kubeip"

RUN go get github.com/golang/dep/cmd/dep && \
    cd "$GOPATH/src/github.com/doitintl/kubeip" && \
    $GOPATH/bin/dep ensure && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a --installsuffix cgo --ldflags="-s"  -ldflags "-X main.version=$(git log | head -n 1 | cut  -f 2 -d ' ') -X main.build_date=$(date +%Y-%m-%d\-%H:%M)" -o /kubeip

FROM alpine:3.4
RUN apk add --no-cache ca-certificates

COPY --from=0 /kubeip /bin/kubeip

ENTRYPOINT ["/bin/kubeip"]
