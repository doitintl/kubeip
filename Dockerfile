FROM --platform=$BUILDPLATFORM golang:1.21-alpine AS builder

# add CA certificates and TZ for local time
RUN apk --update add ca-certificates tzdata make git

# create a working directory
WORKDIR /app

# Retrieve application dependencies.
# This allows the container build to reuse cached dependencies.
# Expecting to copy go.mod and if present go.sum.
COPY go.* ./
RUN go mod download

# Copy local code to the container image.
COPY . ./

# get version, commit and branch from build args
ARG VERSION
ARG COMMIT
ARG BRANCH
ARG TARGETOS
ARG TARGETARCH

# Build the binary with make (using the version, commit and branch)
RUN make build VERSION=${VERSION} COMMIT=${COMMIT} BRANCH=${BRANCH} TARGETOS=${TARGETOS} TARGETARCH=${TARGETARCH}

# final image
FROM scratch
# copy CA certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
# copy timezone settings
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
# copy the binary to the production image from the builder stage
COPY --from=builder /app/.bin/kubeip-agent /kubeip-agent

USER 1001

ENTRYPOINT ["/kubeip-agent"]
CMD ["run"]