package cloud

import "google.golang.org/api/compute/v1"

type InstanceGetter interface {
	Get(projectID, zone, instance string) (*compute.Instance, error)
}

type instanceGetter struct {
	client *compute.Service
}

func NewInstanceGetter(client *compute.Service) InstanceGetter {
	return &instanceGetter{client: client}
}

func (g *instanceGetter) Get(projectID, zone, instance string) (*compute.Instance, error) {
	return g.client.Instances.Get(projectID, zone, instance).Do() //nolint:wrapcheck
}
