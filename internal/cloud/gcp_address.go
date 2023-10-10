package cloud

import (
	"google.golang.org/api/compute/v1"
)

type AddressManager interface {
	AddAccessConfig(project string, zone string, instance string, networkInterface string, accessconfig *compute.AccessConfig) (*compute.Operation, error)
	DeleteAccessConfig(project string, zone string, instance string, accessConfig string, networkInterface string) (*compute.Operation, error)
}

type addressManager struct {
	client *compute.Service
}

func NewAddressManager(client *compute.Service) AddressManager {
	return &addressManager{client: client}
}

func (m *addressManager) AddAccessConfig(project, zone, instance, networkInterface string, accessconfig *compute.AccessConfig) (*compute.Operation, error) {
	return m.client.Instances.AddAccessConfig(project, zone, instance, networkInterface, accessconfig).Do() //nolint:wrapcheck
}

func (m *addressManager) DeleteAccessConfig(project, zone, instance, accessConfig, networkInterface string) (*compute.Operation, error) {
	return m.client.Instances.DeleteAccessConfig(project, zone, instance, accessConfig, networkInterface).Do() //nolint:wrapcheck
}
