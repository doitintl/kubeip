package address

import (
	"context"

	"github.com/pkg/errors"
)

// InventoryReader reports existing reservations, not unallocated IPAM pool capacity.
type InventoryReader interface {
	Inventory(ctx context.Context, filter []string) (available, assigned int, err error)
}

func (a *gcpAssigner) Inventory(ctx context.Context, filter []string) (int, int, error) {
	if err := ctx.Err(); err != nil {
		return 0, 0, errors.Wrap(err, "reading address inventory")
	}
	available, err := a.listAddressesContext(ctx, filter, "", reservedStatus)
	if err != nil {
		return 0, 0, err
	}
	assigned, err := a.listAddressesContext(ctx, filter, "", inUseStatus)
	if err != nil {
		return 0, 0, err
	}
	return len(available), len(assigned), nil
}

func (a *awsAssigner) Inventory(ctx context.Context, filter []string) (int, int, error) {
	filters := make(map[string][]string)
	for _, f := range filter {
		name, values, err := parseShorthandFilter(f)
		if err != nil {
			return 0, 0, errors.Wrap(err, "parsing inventory filter")
		}
		filters[name] = values
	}
	if a.ipamPoolID != "" {
		filters["tag:"+ipamPoolTag] = []string{a.ipamPoolID}
	}
	available, err := a.eipLister.List(ctx, filters, false)
	if err != nil {
		return 0, 0, errors.Wrap(err, "listing unused reservations")
	}
	assigned, err := a.eipLister.List(ctx, filters, true)
	if err != nil {
		return 0, 0, errors.Wrap(err, "listing used reservations")
	}
	return len(available), len(assigned), nil
}
