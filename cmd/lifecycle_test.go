package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/doitintl/kubeip/internal/address"
	"github.com/doitintl/kubeip/internal/config"
	nd "github.com/doitintl/kubeip/internal/node"
	"github.com/doitintl/kubeip/internal/telemetry"
	"github.com/doitintl/kubeip/internal/types"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

type lifecycleCloud struct {
	current                          string
	assignments, repairs, releases   int
	assignErr, observeErr, repairErr error
}

func (c *lifecycleCloud) Assign(context.Context, string, string, []string, string) (string, error) {
	c.assignments++
	if c.assignErr != nil {
		return "", c.assignErr
	}
	c.current = "203.0.113.10"
	return c.current, nil
}
func (c *lifecycleCloud) Unassign(context.Context, string, string) error { c.releases++; return nil }
func (c *lifecycleCloud) CurrentAddress(context.Context, string, string) (string, error) {
	return c.current, c.observeErr
}
func (c *lifecycleCloud) RestoreAddress(_ context.Context, _, _, ip string, _ []string) error {
	c.repairs++
	if c.repairErr != nil {
		return c.repairErr
	}
	c.current = ip
	return nil
}

func lifecycleFixture() (*fake.Clientset, *types.Node, *config.Config, *lifecycleCloud) {
	client := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node", UID: "node-uid", Labels: map[string]string{"kubeip.com/public": "true"}, Annotations: map[string]string{"keep": "value"}}, Spec: corev1.NodeSpec{Taints: []corev1.Taint{{Key: "ready", Effect: corev1.TaintEffectNoSchedule}, {Key: "complete", Effect: corev1.TaintEffectNoSchedule}, {Key: "unrelated", Effect: corev1.TaintEffectNoSchedule}}}})
	return client, &types.Node{UID: "node-uid", Name: "node", Instance: "instance", Zone: "zone", Cloud: types.CloudProviderGCP}, &config.Config{CompletionTaintKey: "complete", TaintKey: "ready", LabelAssignedAddress: true, RetryInterval: time.Millisecond, LeaseDuration: 5}, &lifecycleCloud{}
}

func TestCompletionSurvivesRestart(t *testing.T) {
	for _, exhausted := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "exhausted"}[exhausted], func(t *testing.T) {
			client, n, cfg, cloud := lifecycleFixture()
			if exhausted {
				cloud.assignErr = address.ErrNoAvailableAddresses
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			watcher, err := client.CoreV1().Nodes().Watch(ctx, metav1.ListOptions{})
			require.NoError(t, err)
			defer watcher.Stop()
			done := make(chan struct{})
			go func() {
				defer close(done)
				for {
					select {
					case <-ctx.Done():
						return
					case event := <-watcher.ResultChan():
						node, ok := event.Object.(*corev1.Node)
						if !ok {
							continue
						}
						hasComplete, hasReady := false, false
						for _, taint := range node.Spec.Taints {
							hasComplete = hasComplete || taint.Key == "complete"
							hasReady = hasReady || taint.Key == "ready"
						}
						if !hasComplete && ((exhausted && hasReady) || (!exhausted && !hasReady && node.Labels[nd.AssignedAddressLabel] != "")) {
							cancel()
							return
						}
					}
				}
			}()
			require.NoError(t, runNode(ctx, prepareLogger("error", false), client, cloud, n, cfg))
			<-done
			require.Equal(t, 1, cloud.assignments)
			require.Zero(t, cloud.releases)
			node, err := client.CoreV1().Nodes().Get(context.Background(), n.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.NotEmpty(t, node.Annotations[nd.AssignmentStateAnnotation])
			require.Equal(t, "value", node.Annotations["keep"])
			require.Equal(t, "true", node.Labels["kubeip.com/public"])
			restart, err := newManagedAssigner(client, cloud, n, cfg)
			require.NoError(t, err)
			ip, err := restart.Assign(context.Background(), n.Instance, n.Zone, nil, "")
			if exhausted {
				require.ErrorIs(t, err, errAssignmentComplete)
				require.Empty(t, ip)
			} else {
				require.NoError(t, err)
				require.Equal(t, "203.0.113.10", ip)
			}
			require.Equal(t, 1, cloud.assignments)
			require.Zero(t, cloud.repairs)
		})
	}
}
func TestCompletionRefusesChangedAddress(t *testing.T) {
	t.Parallel()
	client, n, cfg, cloud := lifecycleFixture()
	a, err := newManagedAssigner(client, cloud, n, cfg)
	require.NoError(t, err)
	_, err = a.Assign(context.Background(), n.Instance, n.Zone, nil, "")
	require.NoError(t, err)
	cloud.current = "203.0.113.99"
	_, err = a.Assign(context.Background(), n.Instance, n.Zone, nil, "")
	require.ErrorContains(t, err, "drain")
	require.Equal(t, 1, cloud.assignments)
	require.Zero(t, cloud.repairs)
}
func TestCompletionKeepsTaintForAmbiguousFailure(t *testing.T) {
	t.Parallel()
	client, n, cfg, cloud := lifecycleFixture()
	cloud.assignErr = errors.New("cloud request timeout")
	require.Error(t, runNode(context.Background(), prepareLogger("error", false), client, cloud, n, cfg))
	node, err := client.CoreV1().Nodes().Get(context.Background(), n.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, node.Annotations[nd.AssignmentStateAnnotation])
	require.Len(t, node.Spec.Taints, 3)
	require.Zero(t, cloud.releases)
}
func TestCompletionDoesNotPublishAfterStateFailure(t *testing.T) {
	t.Parallel()
	client, n, cfg, cloud := lifecycleFixture()
	cfg.LabelAssignedAddress = false
	client.PrependReactor("patch", "nodes", func(ktesting.Action) (bool, runtime.Object, error) { return true, nil, errors.New("forbidden") })
	require.Error(t, runNode(context.Background(), prepareLogger("error", false), client, cloud, n, cfg))
	node, err := client.CoreV1().Nodes().Get(context.Background(), n.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Len(t, node.Spec.Taints, 3)
	require.Zero(t, cloud.releases)
}
func TestReconcileRestoresPersistedTargetAndLabel(t *testing.T) {
	t.Parallel()
	client, n, cfg, cloud := lifecycleFixture()
	cfg.CompletionTaintKey = ""
	cfg.ReconcileInterval = time.Second
	a, err := newManagedAssigner(client, cloud, n, cfg)
	require.NoError(t, err)
	_, err = a.Assign(context.Background(), n.Instance, n.Zone, nil, "")
	require.NoError(t, err)
	cloud.current = "203.0.113.99"
	m := &telemetry.Metrics{}
	require.NoError(t, reconcileNode(context.Background(), prepareLogger("error", false), client, a, m))
	require.Equal(t, "203.0.113.10", cloud.current)
	require.Equal(t, 1, cloud.repairs)
	require.Equal(t, 1, cloud.assignments)
	node, err := client.CoreV1().Nodes().Get(context.Background(), n.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, "203-0-113-10", node.Labels[nd.AssignedAddressLabel])
	// A previous failed label publication must recover even when the cloud address already matches.
	require.NoError(t, nd.SetAssignedAddress(context.Background(), client, n.Name, "", n.UID))
	require.NoError(t, reconcileNode(context.Background(), prepareLogger("error", false), client, a, m))
	require.Equal(t, 1, cloud.repairs)
	node, err = client.CoreV1().Nodes().Get(context.Background(), n.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, "203-0-113-10", node.Labels[nd.AssignedAddressLabel])
	cloud.current = "203.0.113.99"
	cloud.repairErr = errors.New("address occupied")
	require.Error(t, reconcileNode(context.Background(), prepareLogger("error", false), client, a, m))
	node, err = client.CoreV1().Nodes().Get(context.Background(), n.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Empty(t, node.Labels[nd.AssignedAddressLabel])
	require.Equal(t, "true", node.Labels["kubeip.com/public"])
}
func TestPersistentModeValidation(t *testing.T) {
	for _, scenario := range []string{"release", "same taint", "combined modes", "IPAM completion", "negative interval"} {
		t.Run(scenario, func(t *testing.T) {
			client, n, cfg, cloud := lifecycleFixture()
			switch scenario {
			case "release":
				cfg.ReleaseOnExit = true
			case "same taint":
				cfg.TaintKey = cfg.CompletionTaintKey
			case "combined modes":
				cfg.ReconcileInterval = time.Second
			case "IPAM completion":
				cfg.AWSIPAMPoolID = "pool"
			case "negative interval":
				cfg.ReconcileInterval = -time.Second
			}
			_, err := newManagedAssigner(client, cloud, n, cfg)
			require.Error(t, err)
			require.Zero(t, cloud.assignments)
		})
	}
}

func TestCompletionDoesNotTreatUncertainAttemptAsExhaustion(t *testing.T) {
	t.Parallel()
	client, n, cfg, cloud := lifecycleFixture()
	a, err := newManagedAssigner(client, cloud, n, cfg)
	require.NoError(t, err)
	cloud.assignErr = errors.New("ambiguous cloud timeout")
	_, err = a.Assign(context.Background(), n.Instance, n.Zone, nil, "")
	require.Error(t, err)
	cloud.assignErr = address.ErrNoAvailableAddresses
	restarted, err := newManagedAssigner(client, cloud, n, cfg)
	require.NoError(t, err)
	_, err = restarted.Assign(context.Background(), n.Instance, n.Zone, nil, "")
	require.ErrorContains(t, err, "unresolved")
	complete, err := restarted.finishExhausted(context.Background(), address.ErrNoAvailableAddresses)
	require.NoError(t, err)
	require.False(t, complete)
	require.Equal(t, 1, cloud.assignments)
	state, err := a.store.Load(context.Background())
	require.NoError(t, err)
	require.Contains(t, state, `"complete":false`)
}

func TestCompletionDoesNotTreatLeaseFailureAsExhaustion(t *testing.T) {
	t.Parallel()
	client, n, cfg, cloud := lifecycleFixture()
	a, err := newManagedAssigner(client, cloud, n, cfg)
	require.NoError(t, err)
	cloud.assignErr = address.ErrNoAvailableAddresses
	_, err = a.Assign(context.Background(), n.Instance, n.Zone, nil, "")
	require.ErrorIs(t, err, address.ErrNoAvailableAddresses)
	complete, err := a.finishExhausted(context.Background(), errors.New("failed to acquire lease"))
	require.NoError(t, err)
	require.False(t, complete)
}
