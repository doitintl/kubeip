package cloud

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/pkg/errors"
)

type EipLister interface {
	List(ctx context.Context, filter map[string][]string, inUse bool) ([]types.Address, error)
}

type eipLister struct {
	client *ec2.Client
}

func NewEipLister(client *ec2.Client) EipLister {
	return &eipLister{client: client}
}

func (l *eipLister) List(ctx context.Context, filter map[string][]string, inUse bool) ([]types.Address, error) {
	// create filter for DescribeAddressesInput
	filters := make([]types.Filter, 0, len(filter)+1)
	for k, v := range filter {
		key := k
		filters = append(filters, types.Filter{
			Name:   &key,
			Values: v,
		})
	}

	// list all elastic IPs in the region matching the filter
	input := &ec2.DescribeAddressesInput{
		Filters: filters,
	}
	list, err := l.client.DescribeAddresses(ctx, input)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list elastic IPs")
	}

	filtered := make([]types.Address, 0, len(list.Addresses))
	// API does not support filtering by association ID equal to nil
	// filter addresses based on whether they are in use or not
	for _, address := range list.Addresses {
		if (inUse && address.AssociationId != nil) || (!inUse && address.AssociationId == nil) {
			filtered = append(filtered, address)
		}
	}

	return filtered, nil
}
