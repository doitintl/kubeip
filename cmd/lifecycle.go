package main

import (
	"context"
	"encoding/json"
	"net/netip"
	"time"

	"github.com/doitintl/kubeip/internal/address"
	"github.com/doitintl/kubeip/internal/config"
	"github.com/doitintl/kubeip/internal/lease"
	nd "github.com/doitintl/kubeip/internal/node"
	"github.com/doitintl/kubeip/internal/telemetry"
	"github.com/doitintl/kubeip/internal/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
)

var errAssignmentComplete = errors.New("assignment attempts already completed without an available address")

type assignmentState struct {
	Instance string `json:"instance"`
	Mode     string `json:"mode"`
	Address  string `json:"address"`
	Complete bool   `json:"complete"`
}

type managedAssigner struct {
	address.Assigner
	cfg          *config.Config
	node         *types.Node
	store        *nd.AnnotationState
	mode         string
	allExhausted bool
	attempted    bool
}

func newManagedAssigner(client kubernetes.Interface, assigner address.Assigner, n *types.Node, cfg *config.Config) (*managedAssigner, error) {
	a := &managedAssigner{Assigner: assigner, cfg: cfg, node: n, store: nd.NewAssignmentState(client, n.Name, n.UID), allExhausted: true}
	if cfg.ReconcileInterval < 0 {
		return nil, errors.New("reconcile interval must not be negative")
	}
	if cfg.CompletionTaintKey != "" {
		if cfg.CompletionTaintKey == cfg.TaintKey || cfg.ReconcileInterval != 0 || cfg.AWSIPAMPoolID != "" {
			return nil, errors.New("completion taint must differ from readiness taint and cannot be combined with drift repair or IPAM")
		}
		if _, ok := assigner.(address.Observer); !ok {
			return nil, errors.New("completion taint requires a provider with read-only address observation")
		}
		a.mode = "complete"
	}
	if cfg.ReconcileInterval > 0 {
		if _, ok := assigner.(address.Restorer); !ok {
			return nil, errors.New("drift repair is supported only by the GCP provider")
		}
		a.mode = "reconcile"
	}
	if a.mode != "" && (cfg.RetryAttempts < 0 || cfg.RetryInterval <= 0) {
		return nil, errors.New("persistent assignment modes require nonnegative retries and a positive retry interval")
	}
	if a.mode != "" && cfg.ReleaseOnExit {
		return nil, errors.New("persistent assignment modes require RELEASE_ON_EXIT=false")
	}
	return a, nil
}

func (a *managedAssigner) load(ctx context.Context) (assignmentState, string, error) {
	raw, err := a.store.Load(ctx)
	if err != nil {
		return assignmentState{}, "", errors.Wrap(err, "loading assignment state")
	}
	var state assignmentState
	if raw == "" {
		return state, raw, nil
	}
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return state, raw, errors.Wrap(err, "decoding assignment state")
	}
	if state.Instance != a.node.Instance || state.Mode != a.mode || (a.mode == "reconcile" && state.Complete) {
		return state, raw, errors.New("persisted assignment state does not match this node or mode")
	}
	if a.mode == "complete" && !state.Complete {
		return state, raw, errors.New("previous assignment outcome is unresolved; inspect cloud operations and drain before resetting state")
	}
	if state.Address == "" && a.mode == "complete" {
		return state, raw, nil
	}
	if _, err := netip.ParseAddr(state.Address); err != nil {
		return state, raw, errors.Wrap(err, "invalid persisted assignment address")
	}
	return state, raw, nil
}

func (a *managedAssigner) save(ctx context.Context, oldValue, ip string) error {
	encoded, err := json.Marshal(assignmentState{Instance: a.node.Instance, Mode: a.mode, Address: ip, Complete: a.mode == "complete"})
	if err != nil {
		return errors.Wrap(err, "encoding assignment state")
	}
	return errors.Wrap(a.store.CompareAndSwap(ctx, oldValue, string(encoded)), "persisting assignment result")
}

func (a *managedAssigner) Assign(ctx context.Context, instance, zone string, filter []string, orderBy string) (string, error) {
	if a.mode == "" {
		return a.Assigner.Assign(ctx, instance, zone, filter, orderBy) //nolint:wrapcheck
	}
	state, raw, err := a.load(ctx)
	if err != nil {
		return "", err
	}
	if raw != "" {
		return a.resume(ctx, state)
	}
	intent, err := a.beginAttempt(ctx)
	if err != nil {
		return "", err
	}
	ip, err := a.Assigner.Assign(ctx, instance, zone, filter, orderBy)
	a.attempted = true
	if err != nil && !errors.Is(err, address.ErrStaticIPAlreadyAssigned) {
		if !errors.Is(err, address.ErrNoAvailableAddresses) {
			a.allExhausted = false
		} else if intent != "" {
			if clearErr := a.store.CompareAndSwap(ctx, intent, ""); clearErr != nil {
				a.allExhausted = false
				return "", errors.Wrap(clearErr, "clearing confirmed exhausted attempt")
			}
		}
		return "", err //nolint:wrapcheck
	}
	if _, err := netip.ParseAddr(ip); err != nil {
		a.allExhausted = false
		return "", errors.Wrap(err, "invalid assigned address")
	}
	if err := a.save(ctx, intent, ip); err != nil {
		a.allExhausted = false
		return "", err
	}
	return ip, nil
}

func (a *managedAssigner) resume(ctx context.Context, state assignmentState) (string, error) {
	if a.mode == "complete" {
		if state.Address == "" {
			return "", errAssignmentComplete
		}
		observer := a.Assigner.(address.Observer)
		current, err := observer.CurrentAddress(ctx, a.node.Instance, a.node.Zone)
		if err != nil {
			return "", errors.Wrap(err, "verifying completed assignment")
		}
		if current != state.Address {
			return "", errors.New("completed address changed; drain the node before resetting completion state")
		}
	} else {
		restorer := a.Assigner.(address.Restorer)
		if err := restorer.RestoreAddress(ctx, a.node.Instance, a.node.Zone, state.Address, a.cfg.Filter); err != nil {
			return "", errors.Wrap(err, "restoring persisted address")
		}
	}
	return state.Address, nil
}

func (a *managedAssigner) finishExhausted(ctx context.Context, assignmentErr error) (bool, error) {
	if a.mode != "complete" {
		return false, nil
	}
	if ctx.Err() != nil {
		return false, errors.Wrap(ctx.Err(), "assignment interrupted before completion")
	}
	if errors.Is(assignmentErr, errAssignmentComplete) {
		return true, nil
	}
	if !a.attempted || !a.allExhausted || !errors.Is(assignmentErr, address.ErrNoAvailableAddresses) {
		return false, nil
	}
	if err := a.save(ctx, "", ""); err != nil {
		return false, err
	}
	return true, nil
}

func removeCompletionTaint(ctx context.Context, client kubernetes.Interface, n *types.Node, key string) error {
	if key == "" {
		return nil
	}
	_, err := nd.NewTainter(client).RemoveTaintKey(ctx, n, key)
	return errors.Wrap(err, "removing completion taint")
}

func waitWithReconciliation(ctx context.Context, log *logrus.Entry, client kubernetes.Interface, a *managedAssigner, m *telemetry.Metrics) {
	if a.cfg.ReconcileInterval == 0 {
		<-ctx.Done()
		return
	}
	ticker := time.NewTicker(a.cfg.ReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := reconcileNode(ctx, log, client, a, m); err != nil && ctx.Err() == nil {
				log.WithError(err).Error("reconciling node address")
			}
		}
	}
}

func reconcileNode(ctx context.Context, log *logrus.Entry, client kubernetes.Interface, a *managedAssigner, m *telemetry.Metrics) error {
	lock := lease.NewKubeLeaseLock(client, kubeipLockName, a.cfg.LeaseNamespace, a.node.Instance, a.cfg.LeaseDuration)
	if err := lock.Lock(ctx); err != nil {
		m.Ready(false)
		return errors.Wrap(err, "locking address repair")
	}
	defer func() { lock.Unlock(ctx) }() //nolint:errcheck
	state, raw, err := a.load(ctx)
	if err != nil || raw == "" {
		m.Ready(false)
		return errors.New("cannot reconcile without valid persisted assignment state")
	}
	restorer := a.Assigner.(address.Restorer)
	current, err := restorer.CurrentAddress(ctx, a.node.Instance, a.node.Zone)
	if err != nil || current != state.Address {
		m.Ready(false)
		if a.cfg.LabelAssignedAddress {
			if patchErr := nd.SetAssignedAddress(ctx, client, a.node.Name, "", a.node.UID); patchErr != nil {
				return errors.Wrap(patchErr, "clearing drifted address label")
			}
		}
		if err != nil {
			m.Repair(err)
			return errors.Wrap(err, "checking current address")
		}
		err = restorer.RestoreAddress(ctx, a.node.Instance, a.node.Zone, state.Address, a.cfg.Filter)
		m.Repair(err)
		if err != nil {
			return errors.Wrap(err, "repairing address drift")
		}
	}
	m.Ready(false)
	if err := waitForAddressToBeReported(ctx, log, nd.NewExplorer(client), a.node, state.Address, a.cfg); err != nil {
		return err
	}
	if a.cfg.LabelAssignedAddress {
		if err := nd.SetAssignedAddress(ctx, client, a.node.Name, state.Address, a.node.UID); err != nil {
			return errors.Wrap(err, "restoring assignment label")
		}
	}
	m.Ready(true)
	return nil
}

func publishAssignment(ctx context.Context, log *logrus.Entry, clientset kubernetes.Interface, managed *managedAssigner, assignedAddress string) error {
	cfg, n, assigner := managed.cfg, managed.node, managed.Assigner
	if cfg.TaintKey != "" || cfg.LabelAssignedAddress {
		if err := waitForAddressToBeReported(ctx, log, nd.NewExplorer(clientset), n, assignedAddress, cfg); err != nil {
			return errors.Wrap(err, "waiting for node to report assigned address")
		}
	}
	if cfg.LabelAssignedAddress {
		if assignedAddress == "" {
			return errors.New("cannot label node with an empty assigned address")
		}
		if err := nd.SetAssignedAddress(ctx, clientset, n.Name, assignedAddress, n.UID); err != nil {
			return errors.Wrap(err, "setting assignment label")
		}
	}

	if cfg.TaintKey != "" {
		logger := log.WithField("taint-key", cfg.TaintKey)
		tainter := nd.NewTainter(clientset)

		didRemoveTaint, err := tainter.RemoveTaintKey(ctx, n, cfg.TaintKey)
		if err != nil {
			logger.Error("removing taint key failed")
			if managed.mode == "" {
				if releaseErr := releaseIP(assigner, n, clientset, cfg.LabelAssignedAddress); releaseErr != nil { //nolint:contextcheck
					log.WithError(releaseErr).Error("releasing static public IP address after taint key removal failed")
				}
			}
			return errors.Wrap(err, "removing node taint key")
		}

		if didRemoveTaint {
			logger.Info("taint key removed successfully")
		} else {
			logger.Warning("taint key not present on node, skipped removal")
		}
	}

	return nil
}

func (a *managedAssigner) beginAttempt(ctx context.Context) (string, error) {
	if a.mode != "complete" {
		return "", nil
	}
	encoded, err := json.Marshal(assignmentState{Instance: a.node.Instance, Mode: a.mode})
	if err != nil {
		return "", errors.Wrap(err, "encoding assignment attempt")
	}
	intent := string(encoded)
	if err := a.store.CompareAndSwap(ctx, "", intent); err != nil {
		return "", errors.Wrap(err, "recording assignment attempt")
	}
	return intent, nil
}
