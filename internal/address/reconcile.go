package address

import (
	"context"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/pkg/errors"
	"google.golang.org/api/compute/v1"
)

// Observer reads the public address without changing cloud resources.
type Observer interface {
	CurrentAddress(context.Context, string, string) (string, error)
}

// Restorer restores the persisted reservation without selecting a replacement.
type Restorer interface {
	Observer
	RestoreAddress(ctx context.Context, instance, zone, expected string, filter []string) error
}

func (a *gcpAssigner) getInstanceContext(ctx context.Context, instance, zone string) (*compute.Instance, error) {
	if getter, ok := a.instanceGetter.(interface {
		GetContext(context.Context, string, string, string) (*compute.Instance, error)
	}); ok {
		return getter.GetContext(ctx, a.project, zone, instance) //nolint:wrapcheck
	}
	return a.instanceGetter.Get(a.project, zone, instance) //nolint:wrapcheck
}

func (a *gcpAssigner) CurrentAddress(ctx context.Context, instance, zone string) (string, error) {
	vm, err := a.getInstanceContext(ctx, instance, zone)
	if err != nil {
		return "", errors.Wrap(err, "observing instance address")
	}
	nic, err := getNetworkInterface(vm)
	if err != nil {
		return "", err
	}
	access, err := getAccessConfig(nic, a.ipv6)
	if errors.Is(err, ErrNoPublicIPAssigned) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if a.ipv6 {
		return access.ExternalIpv6, nil
	}
	return access.NatIP, nil
}

func (a *awsAssigner) CurrentAddress(ctx context.Context, instance, _ string) (string, error) {
	vm, err := a.instanceGetter.Get(ctx, instance, a.region)
	if err != nil {
		return "", errors.Wrap(err, "observing instance address")
	}
	return aws.ToString(vm.PublicIpAddress), nil
}

func (a *gcpAssigner) RestoreAddress(ctx context.Context, instance, zone, expected string, filter []string) error {
	current, err := a.CurrentAddress(ctx, instance, zone)
	if err != nil {
		return err
	}
	if current == expected {
		return nil
	}
	filters := append(append([]string{}, filter...), "address="+strconv.Quote(expected))
	addresses, err := a.listAddressesContext(ctx, filters, "", reservedStatus)
	if err != nil {
		return err
	}
	if len(addresses) != 1 || addresses[0].Address != expected || len(addresses[0].Users) != 0 {
		return errors.New("persisted address is not an available reservation matching the filter; refusing replacement")
	}
	busy, err := a.CheckAddressAssigned(a.region, addresses[0].Name)
	if err != nil {
		return err
	}
	if busy {
		return errors.New("persisted address is already in use; refusing reassignment")
	}
	vm, err := a.getInstanceContext(ctx, instance, zone)
	if err != nil {
		return errors.Wrap(err, "reading instance before repair")
	}
	accessConfigName := getAccessConfigName(vm, a.ipv6)
	if err := a.DeleteInstanceAddress(ctx, vm, zone); err != nil && !errors.Is(err, ErrNoPublicIPAssigned) {
		return err
	}
	vm, err = a.getInstanceContext(ctx, instance, zone)
	if err != nil {
		return errors.Wrap(err, "refreshing instance before repair")
	}
	return tryAssignAddress(ctx, a, vm, a.region, zone, addresses[0], accessConfigName)
}
