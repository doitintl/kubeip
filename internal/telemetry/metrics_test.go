package telemetry

import (
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMetricsReadinessAndInventoryFailure(t *testing.T) {
	t.Parallel()
	m := &Metrics{}
	handler := m.Handler()
	get := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		return r
	}
	require.Equal(t, 200, get("/healthz").Code)
	require.Equal(t, 503, get("/readyz").Code)
	m.Assignment(nil)
	m.Assignment(errors.New("denied"))
	m.Ready(true)
	m.Inventory(3, 2, true, nil)
	require.Equal(t, 200, get("/readyz").Code)
	body := get("/metrics").Body.String()
	for _, line := range []string{"kubeip_assignment_ready 1\n", "kubeip_assignment_attempts_total 2\n", "kubeip_assignment_errors_total 1\n", "kubeip_pool_addresses_total 5\n", "kubeip_pool_inventory_up 1\n"} {
		require.Contains(t, body, line)
	}
	m.Inventory(0, 0, true, errors.New("timeout"))
	m.Ready(false)
	body = get("/metrics").Body.String()
	require.Contains(t, body, "kubeip_pool_inventory_up 0\n")
	require.Contains(t, body, "kubeip_pool_addresses_total 5\n")
	require.NotContains(t, body, "kubeip_pool_inventory_timestamp_seconds 0\n")
	require.Equal(t, 503, get("/readyz").Code)
	require.Equal(t, 404, get("/missing").Code)
}
func TestConcurrentScrapes(t *testing.T) {
	t.Parallel()
	m := &Metrics{}
	handler := m.Handler()
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				m.Assignment(nil)
				m.Ready(true)
				m.Inventory(1, 2, true, nil)
				r := httptest.NewRecorder()
				handler.ServeHTTP(r, httptest.NewRequest("GET", "/metrics", nil))
				if !strings.Contains(r.Body.String(), "kubeip_assignment_attempts_total") {
					t.Error("missing counter")
				}
			}
		}()
	}
	wg.Wait()
	r := httptest.NewRecorder()
	handler.ServeHTTP(r, httptest.NewRequest("GET", "/metrics", nil))
	require.Contains(t, r.Body.String(), "kubeip_assignment_attempts_total 1000\n")
}
