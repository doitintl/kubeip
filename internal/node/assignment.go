package node

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typesv1 "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

// AssignedAddressLabel preserves the v1 label spelling.
const AssignedAddressLabel = "kubip_assigned"

// SetAssignedAddress patches only the assignment label. An empty address removes it.
func SetAssignedAddress(ctx context.Context, client kubernetes.Interface, nodeName, address, uid string) error {
	var value *string
	if address != "" {
		ip, err := netip.ParseAddr(address)
		if err != nil {
			return errors.Wrap(err, "invalid assigned address")
		}
		ip = ip.Unmap()
		label := strings.ReplaceAll(ip.String(), ".", "-")
		if ip.Is6() {
			label = fmt.Sprintf("%x", ip.As16())
		}
		value = &label
	}
	metadata := map[string]any{"labels": map[string]*string{AssignedAddressLabel: value}}
	if uid != "" {
		metadata["uid"] = uid
	}
	patch, err := json.Marshal(map[string]any{"metadata": metadata})
	if err != nil {
		return errors.Wrap(err, "encoding assignment label patch")
	}
	_, err = client.CoreV1().Nodes().Patch(ctx, nodeName, typesv1.MergePatchType, patch, metav1.PatchOptions{})
	if address == "" && apierrors.IsNotFound(err) {
		return nil
	}
	return errors.Wrap(err, "patching assignment label")
}
