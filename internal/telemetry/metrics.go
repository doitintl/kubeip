package telemetry

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Metrics struct {
	mu                                          sync.RWMutex
	ready                                       bool
	attempts, failures, repairs, repairFailures uint64
	inventorySupported, inventoryUp             bool
	available, assigned                         int
	inventoryTime                               int64
}

func (m *Metrics) Assignment(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attempts++
	if err != nil {
		m.failures++
	}
}
func (m *Metrics) Ready(ready bool) { m.mu.Lock(); defer m.mu.Unlock(); m.ready = ready }
func (m *Metrics) Repair(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.repairs++
	if err != nil {
		m.repairFailures++
	}
}
func (m *Metrics) Inventory(available, assigned int, supported bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inventorySupported = supported
	m.inventoryUp = supported && err == nil
	if m.inventoryUp {
		m.available = available
		m.assigned = assigned
		m.inventoryTime = time.Now().Unix()
	}
}
func gauge(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (m *Metrics) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		m.mu.RLock()
		defer m.mu.RUnlock()
		if !m.ready {
			http.Error(w, "address not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/metrics", m.serveMetrics)
	return mux
}

func (m *Metrics) serveMetrics(w http.ResponseWriter, _ *http.Request) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprintf(w, `# HELP kubeip_assignment_ready Whether the node has completed address readiness checks.
# TYPE kubeip_assignment_ready gauge
kubeip_assignment_ready %d
# HELP kubeip_assignment_attempts_total Address assignment attempts in this process.
# TYPE kubeip_assignment_attempts_total counter
kubeip_assignment_attempts_total %d
# HELP kubeip_assignment_errors_total Failed address assignment attempts in this process.
# TYPE kubeip_assignment_errors_total counter
kubeip_assignment_errors_total %d
# HELP kubeip_reconcile_repairs_total Address drift repairs attempted in this process.
# TYPE kubeip_reconcile_repairs_total counter
kubeip_reconcile_repairs_total %d
# HELP kubeip_reconcile_errors_total Failed address drift repairs in this process.
# TYPE kubeip_reconcile_errors_total counter
kubeip_reconcile_errors_total %d
# HELP kubeip_pool_inventory_supported Whether this provider supports filtered address inventory.
# TYPE kubeip_pool_inventory_supported gauge
kubeip_pool_inventory_supported %d
# HELP kubeip_pool_inventory_up Whether the latest inventory refresh succeeded.
# TYPE kubeip_pool_inventory_up gauge
kubeip_pool_inventory_up %d
# HELP kubeip_pool_inventory_timestamp_seconds Unix timestamp of the last successful inventory refresh.
# TYPE kubeip_pool_inventory_timestamp_seconds gauge
kubeip_pool_inventory_timestamp_seconds %d
# HELP kubeip_pool_addresses_available Last observed unused reservations matching the filter.
# TYPE kubeip_pool_addresses_available gauge
kubeip_pool_addresses_available %d
# HELP kubeip_pool_addresses_assigned Last observed used reservations matching the filter.
# TYPE kubeip_pool_addresses_assigned gauge
kubeip_pool_addresses_assigned %d
# HELP kubeip_pool_addresses_total Last observed total reservations matching the filter.
# TYPE kubeip_pool_addresses_total gauge
kubeip_pool_addresses_total %d
`, gauge(m.ready), m.attempts, m.failures, m.repairs, m.repairFailures, gauge(m.inventorySupported), gauge(m.inventoryUp), m.inventoryTime, m.available, m.assigned, m.available+m.assigned)
}
