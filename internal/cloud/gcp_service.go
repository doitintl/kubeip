package cloud

import (
	"context"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
	htransport "google.golang.org/api/transport/http"
)

const (
	// http2SendPingTimeout is how long a connection may stay silent before the
	// transport probes it with a PING frame.
	http2SendPingTimeout = 2 * time.Second
	// http2PingTimeout is how long the transport waits for the PING response
	// before it discards the connection.
	http2PingTimeout = 2 * time.Second
)

// NewComputeTransport returns the base HTTP transport used by the Compute Engine
// client: the default transport with fast HTTP/2 health checks. The agent removes
// the instance's own egress address, which kills the pooled connection to the API
// without any notice; the Google API client only probes such a connection after
// 31 seconds of silence, while these settings drop it within about 4 seconds so
// the operation wait can retry on a fresh connection.
func NewComputeTransport() *http.Transport {
	transport := &http.Transport{ForceAttemptHTTP2: true}
	if defaultTransport, ok := http.DefaultTransport.(*http.Transport); ok {
		transport = defaultTransport.Clone()
	}
	transport.HTTP2 = &http.HTTP2Config{
		SendPingTimeout: http2SendPingTimeout,
		PingTimeout:     http2PingTimeout,
	}
	return transport
}

// NewComputeService builds a Compute Engine client on top of base, adding the
// usual Google API authentication and applying opts.
func NewComputeService(ctx context.Context, base http.RoundTripper, opts ...option.ClientOption) (*compute.Service, error) {
	transport, err := htransport.NewTransport(ctx, base, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "creating compute transport")
	}
	opts = append(opts, option.WithHTTPClient(&http.Client{Transport: transport}))
	service, err := compute.NewService(ctx, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "creating compute service")
	}
	return service, nil
}
