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

type AnnotationState struct {
	client   kubernetes.Interface
	nodeName string
	key      string
	uid      string
}

func NewIPAMState(client kubernetes.Interface, nodeName, uid string) *AnnotationState {
	return &AnnotationState{client: client, nodeName: nodeName, key: IPAMAllocationAnnotation, uid: uid}
}

func (s *AnnotationState) Load(ctx context.Context) (string, error) {
	n, err := s.client.CoreV1().Nodes().Get(ctx, s.nodeName, metav1.GetOptions{})
	if err != nil {
		return "", errors.Wrap(err, "reading assignment state")
	}
	if string(n.UID) != s.uid {
		return "", errors.New("node was replaced; refusing to read assignment state")
	}
	return n.Annotations[s.key], nil
}

// CompareAndSwap prevents two agents from issuing the same non-idempotent allocation.
func (s *AnnotationState) CompareAndSwap(ctx context.Context, oldValue, newValue string) error {
	n, err := s.client.CoreV1().Nodes().Get(ctx, s.nodeName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "reading assignment state")
	}
	if string(n.UID) != s.uid {
		return errors.New("node was replaced; refusing to change assignment state")
	}
	if n.Annotations[s.key] != oldValue {
		return errors.New("assignment state changed")
	}
	var value *string
	if newValue != "" {
		value = &newValue
	}
	patch, err := json.Marshal(map[string]any{"metadata": map[string]any{
		"uid": n.UID, "resourceVersion": n.ResourceVersion,
		"annotations": map[string]*string{s.key: value},
	}})
	if err != nil {
		return errors.Wrap(err, "encoding assignment state")
	}
	_, err = s.client.CoreV1().Nodes().Patch(ctx, s.nodeName, typesv1.MergePatchType, patch, metav1.PatchOptions{})
	return errors.Wrap(err, "saving assignment state")
}

// AssignmentStateAnnotation persists drift targets and terminal completion results.
const AssignmentStateAnnotation = "kubeip.com/assignment-state"

func NewAssignmentState(client kubernetes.Interface, nodeName, uid string) *AnnotationState {
	return &AnnotationState{client: client, nodeName: nodeName, key: AssignmentStateAnnotation, uid: uid}
}
