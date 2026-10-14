package main

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/doitintl/kubeip/internal/address"
	"github.com/doitintl/kubeip/internal/config"
	"github.com/doitintl/kubeip/internal/telemetry"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	inventoryInterval    = 30 * time.Second
	metricsHeaderTimeout = 5 * time.Second
	metricsWriteTimeout  = 10 * time.Second
	inventoryTimeout     = 20 * time.Second
	metricsIdleTimeout   = 30 * time.Second
)

type observedAssigner struct {
	address.Assigner
	metrics *telemetry.Metrics
}

func (a *observedAssigner) Assign(ctx context.Context, instance, zone string, filter []string, orderBy string) (string, error) {
	ip, err := a.Assigner.Assign(ctx, instance, zone, filter, orderBy)
	metricErr := err
	if errors.Is(err, address.ErrStaticIPAlreadyAssigned) {
		metricErr = nil
	}
	a.metrics.Assignment(metricErr)
	return ip, err //nolint:wrapcheck
}

func startMetrics(ctx context.Context, log *logrus.Entry, cfg *config.Config, assigner address.Assigner, m *telemetry.Metrics) (func(), error) {
	if cfg.MetricsAddress == "" {
		return func() {}, nil
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.MetricsAddress)
	if err != nil {
		return nil, errors.Wrap(err, "listening for metrics")
	}
	server := &http.Server{Handler: m.Handler(), ReadHeaderTimeout: metricsHeaderTimeout, WriteTimeout: metricsWriteTimeout, IdleTimeout: metricsIdleTimeout}
	metricsCtx, cancel := context.WithCancel(ctx)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.WithError(err).Error("metrics server failed")
		}
	}()
	go refreshInventory(metricsCtx, log, cfg, assigner, m)
	return func() {
		cancel()
		if err := server.Close(); err != nil {
			log.WithError(err).Warn("closing metrics server")
		}
	}, nil
}

func refreshInventory(ctx context.Context, log *logrus.Entry, cfg *config.Config, assigner address.Assigner, m *telemetry.Metrics) {
	reader, supported := assigner.(address.InventoryReader)
	if !supported {
		m.Inventory(0, 0, false, nil)
		return
	}
	ticker := time.NewTicker(inventoryInterval)
	defer ticker.Stop()
	for {
		requestCtx, cancel := context.WithTimeout(ctx, inventoryTimeout)
		available, assigned, err := reader.Inventory(requestCtx, cfg.Filter)
		cancel()
		m.Inventory(available, assigned, true, err)
		if err != nil && ctx.Err() == nil {
			log.WithError(err).Warn("refreshing address inventory")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
