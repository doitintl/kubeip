package node

import (
	"context"
	"net"
	"os"
	"strings"

	"github.com/doitintl/kubeip/internal/types"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	minProviderIDTokens = 2
	podInfoDir          = "/etc/podinfo/"
	awsPoolLabel        = "eks.amazonaws.com/nodegroup"
	azurePoolLabel      = "node.kubernetes.io/instancegroup"
	gcpPoolLabel        = "cloud.google.com/gke-nodepool"
	regionLabel         = "topology.kubernetes.io/region"
	zoneLabel           = "topology.kubernetes.io/zone"
)

type Explorer interface {
	GetNode(ctx context.Context, nodeName string) (*types.Node, error)
}

type explorer struct {
	client kubernetes.Interface
}

func getNodeName(file string) (string, error) {
	// get node name from file
	nodeName, err := os.ReadFile(file)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read %s", file)
	}
	return string(nodeName), nil
}

func NewExplorer(client kubernetes.Interface) Explorer {
	return &explorer{
		client: client,
	}
}

func getCloudProvider(providerID string) (types.CloudProvider, error) {
	if strings.HasPrefix(providerID, "aws://") {
		return types.CloudProviderAWS, nil
	}
	if strings.HasPrefix(providerID, "azure://") {
		return types.CloudProviderAzure, nil
	}
	if strings.HasPrefix(providerID, "gce://") {
		return types.CloudProviderGCP, nil
	}
	return "", errors.Errorf("unsupported cloud provider: %s", providerID)
}

func getInstance(providerID string) (string, error) {
	s := strings.Split(providerID, "/")
	if len(s) < minProviderIDTokens {
		return "", errors.Errorf("failed to get instance ID")
	}
	return s[len(s)-1], nil
}

func getNodePool(providerID types.CloudProvider, labels map[string]string) (string, error) {
	var ok bool
	var pool string
	if providerID == types.CloudProviderAWS {
		pool, ok = labels[awsPoolLabel]
	} else if providerID == types.CloudProviderAzure {
		pool, ok = labels[azurePoolLabel]
	} else if providerID == types.CloudProviderGCP {
		pool, ok = labels[gcpPoolLabel]
	} else {
		return "", errors.Errorf("unsupported cloud provider: %s", providerID)
	}
	if !ok {
		return "", errors.Errorf("failed to get node pool")
	}
	return pool, nil
}

func getAddresses(addresses []v1.NodeAddress) ([]net.IP, []net.IP, error) {
	var externalIPs []net.IP
	var internalIPs []net.IP
	for _, address := range addresses {
		if address.Type != v1.NodeExternalIP && address.Type != v1.NodeInternalIP {
			continue
		}
		ip := net.ParseIP(address.Address)
		if ip == nil {
			return nil, nil, errors.Errorf("failed to parse IP address: %s", address.Address)
		}
		if address.Type == v1.NodeExternalIP {
			externalIPs = append(externalIPs, ip)
		} else if address.Type == v1.NodeInternalIP {
			internalIPs = append(internalIPs, ip)
		}
	}
	return externalIPs, internalIPs, nil
}

// GetNode returns the node object
func (d *explorer) GetNode(ctx context.Context, nodeName string) (*types.Node, error) {
	// get node name from downward API if nodeName is empty
	if nodeName == "" {
		var err error
		nodeName, err = getNodeName(podInfoDir + "nodeName")
		if err != nil {
			return nil, errors.Wrap(err, "failed to get node name from downward API")
		}
	}

	// get node object from API server
	n, err := d.client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get kubernetes node")
	}

	// get cloud provider from node spec
	cloudProvider, err := getCloudProvider(n.Spec.ProviderID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get cloud provider")
	}

	// get instance ID from provider ID
	instance, err := getInstance(n.Spec.ProviderID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get instance ID")
	}

	// get node region from node labels
	region, ok := n.Labels[regionLabel]
	if !ok {
		return nil, errors.Errorf("failed to get node region")
	}

	// get node zone from node labels
	zone, ok := n.Labels[zoneLabel]
	if !ok {
		return nil, errors.Errorf("failed to get node zone")
	}

	// get node pool from node labels
	pool, err := getNodePool(cloudProvider, n.Labels)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get node pool")
	}

	// get node addresses
	externalIPs, internalIPs, err := getAddresses(n.Status.Addresses)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get node addresses")
	}

	return &types.Node{
		Name:        nodeName,
		Instance:    instance,
		Cloud:       cloudProvider,
		Region:      region,
		Zone:        zone,
		Pool:        pool,
		ExternalIPs: externalIPs,
		InternalIPs: internalIPs,
	}, nil
}
