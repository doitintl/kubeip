package node

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typesv1 "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

// IPAMAllocationAnnotation records an allocation intent before the cloud request.
const IPAMAllocationAnnotation = "kubeip.com/ipam-allocation"

type IPAMState struct {
	client   kubernetes.Interface
	nodeName string
}

func NewIPAMState(client kubernetes.Interface, nodeName string) *IPAMState {
	return &IPAMState{client: client, nodeName: nodeName}
}

func (s *IPAMState) Load(ctx context.Context) (string, error) {
	n, err := s.client.CoreV1().Nodes().Get(ctx, s.nodeName, metav1.GetOptions{})
	if err != nil {
		return "", errors.Wrap(err, "reading IPAM allocation intent")
	}
	return n.Annotations[IPAMAllocationAnnotation], nil
}

// CompareAndSwap prevents two agents from issuing the same non-idempotent allocation.
func (s *IPAMState) CompareAndSwap(ctx context.Context, oldValue, newValue string) error {
	n, err := s.client.CoreV1().Nodes().Get(ctx, s.nodeName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "reading IPAM allocation intent")
	}
	if n.Annotations[IPAMAllocationAnnotation] != oldValue {
		return errors.New("IPAM allocation intent changed")
	}
	var value *string
	if newValue != "" {
		value = &newValue
	}
	patch, err := json.Marshal(map[string]any{"metadata": map[string]any{
		"uid": n.UID, "resourceVersion": n.ResourceVersion,
		"annotations": map[string]*string{IPAMAllocationAnnotation: value},
	}})
	if err != nil {
		return errors.Wrap(err, "encoding IPAM allocation intent")
	}
	_, err = s.client.CoreV1().Nodes().Patch(ctx, s.nodeName, typesv1.MergePatchType, patch, metav1.PatchOptions{})
	return errors.Wrap(err, "saving IPAM allocation intent")
}
