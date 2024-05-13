package node

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/doitintl/kubeip/internal/types"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typesv1 "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

type Tainter interface {
	RemoveTaintKey(ctx context.Context, node *types.Node, taintKey string) (bool, error)
}

type tainter struct {
	client kubernetes.Interface
}

func deleteTaintsByKey(taints []v1.Taint, taintKey string) ([]v1.Taint, bool) {
	newTaints := []v1.Taint{}
	didDelete := false

	for i := range taints {
		if taintKey == taints[i].Key {
			didDelete = true
			continue
		}
		newTaints = append(newTaints, taints[i])
	}

	return newTaints, didDelete
}

func NewTainter(client kubernetes.Interface) Tainter {
	return &tainter{
		client: client,
	}
}

func (t *tainter) RemoveTaintKey(ctx context.Context, node *types.Node, taintKey string) (bool, error) {
	// get node object from API server
	n, err := t.client.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
	if err != nil {
		return false, errors.Wrap(err, "failed to get kubernetes node")
	}

	// Remove taint from the node representation
	newTaints, didDelete := deleteTaintsByKey(n.Spec.Taints, taintKey)
	if !didDelete {
		return false, nil
	}

	// Marshal the remaining taints of the node into json format for patching.
	// The remaining taints may be empty, and that will result in an empty json array "[]"
	newTaintsMarshaled, err := json.Marshal(newTaints)
	if err != nil {
		return false, errors.Wrap(err, "failed to marshal new taints")
	}

	// Patch the node with only the remaining taints
	patch := fmt.Sprintf(`{"spec":{"taints":%v}}`, string(newTaintsMarshaled))
	_, err = t.client.CoreV1().Nodes().Patch(ctx, node.Name, typesv1.MergePatchType, []byte(patch), metav1.PatchOptions{})
	if err != nil {
		return false, errors.Wrap(err, "failed to patch node taints")
	}

	return true, nil
}
