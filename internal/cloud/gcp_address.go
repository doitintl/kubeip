package cloud

import (
	"google.golang.org/api/compute/v1"
)

const (
	ipv4Only = "IPV4_ONLY"
	ipv4ipv6 = "IPV4_IPV6"
)

type AddressManager interface {
	AddAccessConfig(project string, zone string, instance string, networkInterface string, fingerprint string, accessconfig *compute.AccessConfig) (*compute.Operation, error)
	DeleteAccessConfig(project string, zone string, instance string, accessConfig string, networkInterface string, fingerprint string) (*compute.Operation, error)
	GetAddress(project, region, name string) (*compute.Address, error)
}

type addressManager struct {
	client *compute.Service
	ipv6   bool
}

func NewAddressManager(client *compute.Service, ipv6 bool) AddressManager {
	return &addressManager{client: client, ipv6: ipv6}
}

func (m *addressManager) AddAccessConfig(project, zone, instance, networkInterface, fingerprint string, accessconfig *compute.AccessConfig) (*compute.Operation, error) {
	if m.ipv6 {
		// Add the IPv6 address configuration by updating the network interface with the IPv6 stack type and Ipv6AccessConfigs struct
		return m.client.Instances.UpdateNetworkInterface(project, zone, instance, networkInterface, &compute.NetworkInterface{ //nolint:wrapcheck
			Fingerprint: fingerprint, // Required to update network interface
			StackType:   ipv4ipv6,
			Ipv6AccessConfigs: []*compute.AccessConfig{
				accessconfig,
			},
		}).Do()
	}
	return m.client.Instances.AddAccessConfig(project, zone, instance, networkInterface, accessconfig).Do() //nolint:wrapcheck
}

func (m *addressManager) DeleteAccessConfig(project, zone, instance, accessConfig, networkInterface, fingerprint string) (*compute.Operation, error) {
	if m.ipv6 {
		// Remove the existing IPv6 address configuration by updating the network interface with the IPv4 only stack type.
		return m.client.Instances.UpdateNetworkInterface(project, zone, instance, networkInterface, &compute.NetworkInterface{ //nolint:wrapcheck
			Fingerprint: fingerprint, // Required to update network interface
			StackType:   ipv4Only,
		}).Do()
	}
	return m.client.Instances.DeleteAccessConfig(project, zone, instance, accessConfig, networkInterface).Do() //nolint:wrapcheck
}

func (m *addressManager) GetAddress(project, region, name string) (*compute.Address, error) {
	return m.client.Addresses.Get(project, region, name).Do() //nolint:wrapcheck
}
