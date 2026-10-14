package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/doitintl/kubeip/internal/address"
	"github.com/doitintl/kubeip/internal/config"
	"github.com/doitintl/kubeip/internal/node"
	"github.com/doitintl/kubeip/internal/types"
	mocks "github.com/doitintl/kubeip/mocks/address"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestAssignmentLabelLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		existing, release, taint bool
	}{
		{"new assignment retained", false, false, false},
		{"restart existing assignment retained", true, false, false},
		{"shutdown removes label before release", false, true, false},
		{"label precedes taint removal", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()
			client := fake.NewSimpleClientset(&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node", Labels: map[string]string{node.AssignedAddressLabel: "stale", "custom": "keep"}}})
			nodeObject, err := client.CoreV1().Nodes().Get(ctx, "node", metav1.GetOptions{})
			require.NoError(t, err)
			nodeObject.Spec.ProviderID = "aws:///zone/instance"
			nodeObject.Labels["topology.kubernetes.io/region"] = "region"
			nodeObject.Labels["topology.kubernetes.io/zone"] = "zone"
			nodeObject.Status.Addresses = []v1.NodeAddress{{Type: v1.NodeExternalIP, Address: "203.0.113.10"}}
			if tc.taint {
				nodeObject.Spec.Taints = []v1.Taint{{Key: "ready", Effect: v1.TaintEffectNoSchedule}}
				client.PrependReactor("patch", "nodes", func(a ktesting.Action) (bool, runtime.Object, error) {
					var patch map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(a.(ktesting.PatchAction).GetPatch(), &patch))
					if _, ok := patch["spec"]; ok {
						obj, e := client.Tracker().Get(v1.SchemeGroupVersion.WithResource("nodes"), "", "node")
						require.NoError(t, e)
						require.Equal(t, "203-0-113-10", obj.(*v1.Node).Labels[node.AssignedAddressLabel])
					}
					return false, nil, nil
				})
			}
			_, err = client.CoreV1().Nodes().Update(ctx, nodeObject, metav1.UpdateOptions{})
			require.NoError(t, err)
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
					case event, ok := <-watcher.ResultChan():
						if !ok {
							return
						}
						if n, ok := event.Object.(*v1.Node); ok && n.Labels[node.AssignedAddressLabel] == "203-0-113-10" && len(n.Spec.Taints) == 0 {
							cancel()
							return
						}
					}
				}
			}()
			assigner := mocks.NewAssigner(t)
			var assignErr error
			if tc.existing {
				assignErr = address.ErrStaticIPAlreadyAssigned
			}
			assigner.EXPECT().Assign(mock.Anything, "instance", "zone", mock.Anything, "").Run(func(context.Context, string, string, []string, string) {
				n, e := client.CoreV1().Nodes().Get(context.Background(), "node", metav1.GetOptions{})
				require.NoError(t, e)
				require.NotContains(t, n.Labels, node.AssignedAddressLabel)
			}).Return("203.0.113.10", assignErr).Once()
			if tc.release {
				assigner.EXPECT().Unassign(mock.Anything, "instance", "zone").Run(func(c context.Context, _ string, _ string) {
					require.NoError(t, c.Err())
					n, e := client.CoreV1().Nodes().Get(c, "node", metav1.GetOptions{})
					require.NoError(t, e)
					require.NotContains(t, n.Labels, node.AssignedAddressLabel)
				}).Return(nil).Once()
			}
			cfg := &config.Config{LabelAssignedAddress: true, WaitForNodeAddress: true, RetryInterval: time.Millisecond, LeaseDuration: 1, ReleaseOnExit: tc.release}
			if tc.taint {
				cfg.TaintKey = "ready"
			}
			err = runNode(ctx, prepareLogger("error", false), client, assigner, &types.Node{Name: "node", Instance: "instance", Zone: "zone"}, cfg)
			require.NoError(t, err)
			<-done
			n, err := client.CoreV1().Nodes().Get(context.Background(), "node", metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, "keep", n.Labels["custom"])
			if tc.release {
				require.NotContains(t, n.Labels, node.AssignedAddressLabel)
			} else {
				require.Equal(t, "203-0-113-10", n.Labels[node.AssignedAddressLabel])
			}
		})
	}
}

func TestAssignmentLabelFailures(t *testing.T) {
	for _, tc := range []struct {
		name                                                       string
		failClear, failSet, failAssign, failWait, failTaint, empty bool
	}{
		{name: "clear failure prevents assignment", failClear: true},
		{name: "assignment failure leaves no marker", failAssign: true},
		{name: "label failure retains taint", failSet: true},
		{name: "node readiness failure leaves no marker", failWait: true},
		{name: "taint failure clears label before release", failTaint: true},
		{name: "empty address is not ready", empty: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset(&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node", Labels: map[string]string{node.AssignedAddressLabel: "stale", "custom": "keep", "topology.kubernetes.io/region": "region", "topology.kubernetes.io/zone": "zone"}}, Spec: v1.NodeSpec{ProviderID: "aws:///zone/instance", Taints: []v1.Taint{{Key: "ready", Effect: v1.TaintEffectNoSchedule}}}})
			failure := errors.New("test failure")
			client.PrependReactor("patch", "nodes", func(a ktesting.Action) (bool, runtime.Object, error) {
				var patch map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(a.(ktesting.PatchAction).GetPatch(), &patch))
				if _, ok := patch["spec"]; ok && tc.failTaint {
					return true, nil, failure
				}
				if raw, ok := patch["metadata"]; ok {
					var meta struct {
						Labels map[string]*string `json:"labels"`
					}
					require.NoError(t, json.Unmarshal(raw, &meta))
					value := meta.Labels[node.AssignedAddressLabel]
					if (value == nil && tc.failClear) || (value != nil && tc.failSet) {
						return true, nil, failure
					}
				}
				return false, nil, nil
			})
			assigner := mocks.NewAssigner(t)
			if !tc.failClear {
				var err error
				if tc.failAssign {
					err = failure
				}
				ip := "203.0.113.10"
				if tc.empty {
					ip = ""
				}
				assigner.EXPECT().Assign(mock.Anything, "instance", "zone", mock.Anything, "").Return(ip, err).Once()
			}
			if tc.failTaint {
				assigner.EXPECT().Unassign(mock.Anything, "instance", "zone").Run(func(c context.Context, _ string, _ string) {
					n, err := client.CoreV1().Nodes().Get(c, "node", metav1.GetOptions{})
					require.NoError(t, err)
					require.NotContains(t, n.Labels, node.AssignedAddressLabel)
				}).Return(nil).Once()
			}
			cfg := &config.Config{LabelAssignedAddress: true, WaitForNodeAddress: tc.failWait, RetryInterval: time.Millisecond, LeaseDuration: 1, TaintKey: "ready"}
			require.Error(t, runNode(context.Background(), prepareLogger("error", false), client, assigner, &types.Node{Name: "node", Instance: "instance", Zone: "zone"}, cfg))
			n, err := client.CoreV1().Nodes().Get(context.Background(), "node", metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, "keep", n.Labels["custom"])
			require.Len(t, n.Spec.Taints, 1)
			if !tc.failClear {
				require.NotContains(t, n.Labels, node.AssignedAddressLabel)
			}
		})
	}
}

func TestReleaseLabelFailureKeepsAddress(t *testing.T) {
	client := fake.NewSimpleClientset()
	client.PrependReactor("patch", "nodes", func(ktesting.Action) (bool, runtime.Object, error) { return true, nil, errors.New("denied") })
	assigner := mocks.NewAssigner(t)
	require.Error(t, releaseIP(assigner, &types.Node{Name: "node"}, client, true))
	assigner.AssertNotCalled(t, "Unassign", mock.Anything, mock.Anything, mock.Anything)
}

func TestAssignmentLabelDisabledDoesNotPatchNodes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := fake.NewSimpleClientset()
	assigner := mocks.NewAssigner(t)
	assigner.EXPECT().Assign(mock.Anything, "instance", "zone", mock.Anything, "").Run(func(context.Context, string, string, []string, string) { cancel() }).Return("203.0.113.10", nil).Once()
	assigner.EXPECT().Unassign(mock.Anything, "instance", "zone").Return(nil).Once()
	cfg := &config.Config{RetryInterval: time.Millisecond, LeaseDuration: 1, ReleaseOnExit: true}
	require.NoError(t, runNode(ctx, prepareLogger("error", false), client, assigner, &types.Node{Name: "node", Instance: "instance", Zone: "zone"}, cfg))
	for _, a := range client.Actions() {
		require.NotEqual(t, "nodes", a.GetResource().Resource)
	}
}

func TestReleaseFailureLeavesLabelRemoved(t *testing.T) {
	client := fake.NewSimpleClientset(&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node", Labels: map[string]string{node.AssignedAddressLabel: "203-0-113-10", "custom": "keep"}}})
	assigner := mocks.NewAssigner(t)
	assigner.EXPECT().Unassign(mock.Anything, "instance", "zone").Return(errors.New("cloud failure")).Once()
	require.Error(t, releaseIP(assigner, &types.Node{Name: "node", Instance: "instance", Zone: "zone"}, client, true))
	n, err := client.CoreV1().Nodes().Get(context.Background(), "node", metav1.GetOptions{})
	require.NoError(t, err)
	require.NotContains(t, n.Labels, node.AssignedAddressLabel)
	require.Equal(t, "keep", n.Labels["custom"])
}
