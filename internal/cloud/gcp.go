package cloud

import "google.golang.org/api/compute/v1"

type ListCall interface {
	Filter(filter string) ListCall
	OrderBy(orderBy string) ListCall
	PageToken(pageToken string) ListCall
	Do() (*compute.AddressList, error)
}

type Lister interface {
	List(projectID, region string) ListCall
}

func NewLister(client *compute.Service) Lister {
	return &gcpLister{client: client}
}

type gcpLister struct {
	client *compute.Service
}

type gcpListCall struct {
	call *compute.AddressesListCall
}

func (l *gcpLister) List(projectID, region string) ListCall {
	return &gcpListCall{l.client.Addresses.List(projectID, region)}
}

func (c *gcpListCall) Filter(filter string) ListCall {
	return &gcpListCall{c.call.Filter(filter)}
}

func (c *gcpListCall) OrderBy(orderBy string) ListCall {
	return &gcpListCall{c.call.OrderBy(orderBy)}
}

func (c *gcpListCall) PageToken(pageToken string) ListCall {
	return &gcpListCall{c.call.PageToken(pageToken)}
}

func (c *gcpListCall) Do() (*compute.AddressList, error) {
	return c.call.Do() //nolint:wrapcheck
}
