package node

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestIPAMState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node", UID: "uid-1", ResourceVersion: "7", Annotations: map[string]string{"keep": "value"}, Labels: map[string]string{"kubeip.com/public": "true"}}})
	state := NewIPAMState(client, "node", "uid-1")
	client.PrependReactor("patch", "nodes", func(action ktesting.Action) (bool, runtime.Object, error) {
		var patch struct {
			Metadata struct {
				UID         string             `json:"uid"`
				Version     string             `json:"resourceVersion"`
				Annotations map[string]*string `json:"annotations"`
			} `json:"metadata"`
		}
		require.NoError(t, json.Unmarshal(action.(ktesting.PatchAction).GetPatch(), &patch))
		require.Equal(t, "uid-1", patch.Metadata.UID)
		require.Equal(t, "7", patch.Metadata.Version)
		require.Len(t, patch.Metadata.Annotations, 1)
		return false, nil, nil
	})
	require.NoError(t, state.CompareAndSwap(ctx, "", "pool/instance"))
	value, err := state.Load(ctx)
	require.NoError(t, err)
	require.Equal(t, "pool/instance", value)
	require.Error(t, state.CompareAndSwap(ctx, "", "different"))
	require.NoError(t, state.CompareAndSwap(ctx, "pool/instance", ""))
	n, err := client.CoreV1().Nodes().Get(ctx, "node", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"keep": "value"}, n.Annotations)
	require.Equal(t, "true", n.Labels["kubeip.com/public"])
}
func TestIPAMStatePatchConflict(t *testing.T) {
	t.Parallel()
	client := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node"}})
	client.PrependReactor("patch", "nodes", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "nodes"}, "node", nil)
	})
	require.Error(t, NewIPAMState(client, "node", "").CompareAndSwap(context.Background(), "", "pool/instance"))
}

func TestAssignmentStateRefusesReplacementNode(t *testing.T) {
	t.Parallel()
	client := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node", UID: "replacement"}})
	state := NewAssignmentState(client, "node", "original")
	_, err := state.Load(context.Background())
	require.ErrorContains(t, err, "replaced")
	require.ErrorContains(t, state.CompareAndSwap(context.Background(), "", "old-instance"), "replaced")
	for _, action := range client.Actions() {
		require.NotEqual(t, "patch", action.GetVerb())
	}
}
