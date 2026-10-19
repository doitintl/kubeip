package cloud

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

const (
	relayBufferSize    = 32 * 1024
	deadConnectionWait = 10 * time.Second
	// deadConnectionLimit is well below the 31 seconds the Google API client
	// needs by default to notice a connection that stopped carrying traffic.
	deadConnectionLimit = 8 * time.Second
)

// blackholeProxy relays TCP connections to a target. Once Drop is called it
// keeps every connection open but silently discards all traffic, which is what
// a GCE instance observes on its established connections right after its
// egress address has been removed.
type blackholeProxy struct {
	listener net.Listener
	drop     atomic.Bool
}

func newBlackholeProxy(t *testing.T, target string) *blackholeProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	proxy := &blackholeProxy{listener: listener}
	go proxy.serve(target)
	t.Cleanup(func() { _ = listener.Close() })
	return proxy
}

func (p *blackholeProxy) serve(target string) {
	for {
		client, err := p.listener.Accept()
		if err != nil {
			return
		}
		upstream, err := net.Dial("tcp", target)
		if err != nil {
			_ = client.Close()
			return
		}
		go p.relay(client, upstream)
		go p.relay(upstream, client)
	}
}

func (p *blackholeProxy) relay(src, dst net.Conn) {
	defer func() { _ = dst.Close() }()
	buf := make([]byte, relayBufferSize)
	for {
		n, err := src.Read(buf)
		if n > 0 && !p.drop.Load() {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func newOperationServer(t *testing.T, proto *atomic.Value) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proto.Store(r.Proto)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(compute.Operation{Name: "op", Status: "DONE"})
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

func TestComputeServiceFailsFastOnDeadConnection(t *testing.T) {
	t.Parallel()
	var proto atomic.Value
	server := newOperationServer(t, &proto)
	proxy := newBlackholeProxy(t, server.Listener.Addr().String())

	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	base := NewComputeTransport()
	base.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}

	ctx := context.Background()
	service, err := NewComputeService(ctx, base,
		option.WithEndpoint("https://"+proxy.listener.Addr().String()+"/"),
		option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewComputeService() error = %v", err)
	}

	// Warm up: establishes the pooled connection that the next call reuses.
	if _, err = service.ZoneOperations.Get("p", "z", "op").Context(ctx).Do(); err != nil {
		t.Fatalf("warm-up request error = %v", err)
	}
	if got := proto.Load(); got != "HTTP/2.0" {
		t.Fatalf("server saw %v, want HTTP/2.0", got)
	}

	proxy.drop.Store(true)
	waitCtx, cancel := context.WithTimeout(ctx, deadConnectionWait)
	defer cancel()
	start := time.Now()
	_, err = service.ZoneOperations.Wait("p", "z", "op").Context(waitCtx).Do()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("wait on a dead connection succeeded, want an error")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait on a dead connection only ended with the caller's deadline after %s: %v", elapsed, err)
	}
	if elapsed > deadConnectionLimit {
		t.Fatalf("dead connection detected after %s, want under %s", elapsed, deadConnectionLimit)
	}
}
