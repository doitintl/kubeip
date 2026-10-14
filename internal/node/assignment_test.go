package node

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestSetAssignedAddress(t *testing.T) {
	for _, tc := range []struct{ name, address, want string }{
		{"IPv4 compatibility", "203.0.113.10", "203-0-113-10"},
		{"IPv6", "2001:db8::1", "20010db8000000000000000000000001"},
		{"IPv4 mapped", "::ffff:203.0.113.10", "203-0-113-10"},
		{"remove", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client := fake.NewSimpleClientset(&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node", Labels: map[string]string{AssignedAddressLabel: "old", "custom": "keep", "kubeip.com/public": "true"}, Annotations: map[string]string{"owner": "keep"}}})
			require.NoError(t, SetAssignedAddress(ctx, client, "node", tc.address, ""))
			n, err := client.CoreV1().Nodes().Get(ctx, "node", metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, tc.want, n.Labels[AssignedAddressLabel])
			require.Equal(t, "keep", n.Labels["custom"])
			require.Equal(t, "true", n.Labels["kubeip.com/public"])
			require.Equal(t, "keep", n.Annotations["owner"])
			if tc.address == "" {
				require.NotContains(t, n.Labels, AssignedAddressLabel)
			}
			var patch map[string]map[string]map[string]*string
			require.NoError(t, json.Unmarshal(client.Actions()[0].(ktesting.PatchAction).GetPatch(), &patch))
			require.Len(t, patch["metadata"]["labels"], 1)
		})
	}
}

func TestSetAssignedAddressErrors(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	require.Error(t, SetAssignedAddress(ctx, client, "node", "not-an-IP", ""))
	require.Empty(t, client.Actions())
	require.NoError(t, SetAssignedAddress(ctx, client, "deleted-node", "", ""))
	require.Error(t, SetAssignedAddress(ctx, client, "deleted-node", "203.0.113.10", ""))
	denied := apierrors.NewForbidden(schema.GroupResource{Resource: "nodes"}, "node", nil)
	client.PrependReactor("patch", "nodes", func(ktesting.Action) (bool, runtime.Object, error) { return true, nil, denied })
	require.ErrorIs(t, SetAssignedAddress(ctx, client, "node", "", ""), denied)
	require.ErrorIs(t, SetAssignedAddress(ctx, client, "node", "203.0.113.10", ""), denied)
}

func TestAssignedAddressPatchIncludesNodeIdentity(t *testing.T) {
	t.Parallel()
	client := fake.NewSimpleClientset()
	client.PrependReactor("patch", "nodes", func(action ktesting.Action) (bool, runtime.Object, error) {
		var patch struct {
			Metadata struct {
				UID string `json:"uid"`
			} `json:"metadata"`
		}
		require.NoError(t, json.Unmarshal(action.(ktesting.PatchAction).GetPatch(), &patch))
		require.Equal(t, "original-node", patch.Metadata.UID)
		return true, &v1.Node{}, nil
	})
	require.NoError(t, SetAssignedAddress(context.Background(), client, "node", "203.0.113.10", "original-node"))
}
