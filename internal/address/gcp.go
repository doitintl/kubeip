package address

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/doitintl/kubeip/internal/cloud"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/compute/v1"
)

const (
	operationDone               = "DONE" // operation status DONE
	inUseStatus                 = "IN_USE"
	reservedStatus              = "RESERVED" // static IP addresses that are reserved but not currently in use
	defaultTimeout              = 10 * time.Minute
	defaultNetworkName          = "External IP"
	defaultNetworkNameIPv6      = "External IPv6"
	defaultAccessConfigType     = "ONE_TO_ONE_NAT"
	defaultAccessConfigIPv6Type = "DIRECT_IPV6"
	defaultNetworkTier          = "PREMIUM"
	accessConfigKind            = "compute#accessConfig"
	defaultPrefixLength         = 96
	maxRetries                  = 10 // number of retries for assigning ephemeral public IP address
)

type internalAssigner interface {
	CheckAddressAssigned(region, addressName string) (bool, error)
	AddInstanceAddress(ctx context.Context, instance *compute.Instance, zone string, address *compute.Address) error
	DeleteInstanceAddress(ctx context.Context, instance *compute.Instance, zone string) error
}

type gcpAssigner struct {
	lister         cloud.Lister
	waiter         cloud.ZoneWaiter
	addressManager cloud.AddressManager
	instanceGetter cloud.InstanceGetter
	project        string
	region         string
	ipv6           bool
	logger         *logrus.Entry
}

type operationError struct {
	name string
	err  *compute.OperationError
}

func newOperationError(name string, err *compute.OperationError) *operationError {
	return &operationError{name: name, err: err}
}

func isOperationError(err error) bool {
	_, ok := err.(*operationError) //nolint:errorlint
	return ok
}

func joinErrorMessages(operationError *compute.OperationError) string {
	if operationError == nil || len(operationError.Errors) == 0 {
		return ""
	}
	messages := make([]string, 0, len(operationError.Errors))
	for _, errorItem := range operationError.Errors {
		messages = append(messages, errorItem.Message)
	}
	return strings.Join(messages, "; ")
}

func (e *operationError) Error() string {
	if e.err == nil {
		return ""
	}
	return fmt.Sprintf("operation %s failed with error %v", e.name, joinErrorMessages(e.err))
}

func NewGCPAssigner(ctx context.Context, logger *logrus.Entry, project, region string, ipv6 bool) (Assigner, error) {
	// initialize Google Cloud client
	client, err := compute.NewService(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize Google Cloud client")
	}

	// get project ID from metadata server
	if project == "" {
		project, err = metadata.ProjectID()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get project ID from metadata server")
		}
	}

	// get region from metadata server
	if region == "" {
		region, err = metadata.InstanceAttributeValue("cluster-location")
		if err != nil {
			return nil, errors.Wrap(err, "failed to get region from metadata server")
		}
		// if cluster-location is zone, extract region from zone
		if len(region) > 2 && region[len(region)-2] == '-' {
			region = region[:len(region)-2]
		}
	}

	return &gcpAssigner{
		lister:         cloud.NewLister(client),
		waiter:         cloud.NewZoneWaiter(client),
		addressManager: cloud.NewAddressManager(client, ipv6),
		instanceGetter: cloud.NewInstanceGetter(client),
		project:        project,
		region:         region,
		ipv6:           ipv6,
		logger:         logger,
	}, nil
}

func (a *gcpAssigner) waitForOperation(c context.Context, op *compute.Operation, zone string, timeout time.Duration) error {
	if op == nil {
		a.logger.Warn("operation is nil")
		return nil
	}
	// Create a context that will be cancelled with timeout
	ctx, cancel := context.WithTimeout(c, timeout)
	defer cancel()

	var err error
	name := op.Name
	for op.Status != operationDone {
		// Pass the cancellable context to the Wait method
		op, err = a.waiter.Wait(a.project, zone, name).Context(ctx).Do()
		if err != nil {
			// If the context was cancelled, return a timeout error
			if errors.Is(err, context.Canceled) {
				return errors.New("operation timed out")
			}
			return errors.Wrapf(err, "failed to get operation %s", name)
		}
		// If the operation has an error, return it
		if op != nil && op.Error != nil {
			return newOperationError(op.Name, op.Error)
		}
	}
	return nil
}

func (a *gcpAssigner) DeleteInstanceAddress(ctx context.Context, instance *compute.Instance, zone string) error {
	// get instance network interface
	networkInterface, err := getNetworkInterface(instance)
	if err != nil {
		return errors.Wrap(err, "failed to get instance network interface")
	}

	// get instance network interface access config (IPv4 or IPv6)
	accessConfig, err := getAccessConfig(networkInterface, a.ipv6)
	if err != nil {
		return errors.Wrap(err, "failed to get instance network interface access config")
	}

	// delete instance network interface access config
	a.logger.WithField("instance", instance.Name).Infof("deleting public IP address from instance")
	op, err := a.addressManager.DeleteAccessConfig(a.project, zone, instance.Name, accessConfig.Name, networkInterface.Name, networkInterface.Fingerprint)
	if err != nil {
		return errors.Wrapf(err, "failed to delete access config %s from instance %s", accessConfig.Name, instance.Name)
	}
	// wait for operation to complete
	if err = a.waitForOperation(ctx, op, zone, defaultTimeout); err != nil {
		// return error if operation failed
		if isOperationError(err) {
			return err
		}
		// log error and continue (ignore non-operation errors)
		a.logger.WithError(err).Errorf("failed waiting for operation %s", op.Name)
	}
	return nil
}

func (a *gcpAssigner) AddInstanceAddress(ctx context.Context, instance *compute.Instance, zone string, address *compute.Address) error {
	// get instance network interface
	networkInterface, err := getNetworkInterface(instance)
	if err != nil {
		return errors.Wrap(err, "failed to get instance network interface")
	}
	// create access config for the address
	accessConfig := createAccessConfig(address, a.ipv6)
	// add instance network interface access config
	a.logger.WithField("accessConfig", accessConfig).Info("adding public IP address to instance")
	op, err := a.addressManager.AddAccessConfig(a.project, zone, instance.Name, networkInterface.Name, networkInterface.Fingerprint, accessConfig)
	if err != nil {
		return errors.Wrapf(err, "failed to add access config to instance %s", instance.Name)
	}
	// wait for operation to complete
	if err = a.waitForOperation(ctx, op, zone, defaultTimeout); err != nil {
		// return error if operation failed
		if isOperationError(err) {
			return err
		}
		// log error and continue (ignore non-operation errors)
		a.logger.WithError(err).Errorf("failed waiting for operation %s", op.Name)
	}
	return nil
}

func (a *gcpAssigner) CheckAddressAssigned(region, addressName string) (bool, error) {
	address, err := a.addressManager.GetAddress(a.project, region, addressName)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get address %s", addressName)
	}
	return address.Status == inUseStatus, nil
}

func (a *gcpAssigner) Assign(ctx context.Context, instanceID, zone string, filter []string, orderBy string) error {
	// check if instance already has a public static IP address assigned
	instance, err := a.checkStaticIPAssigned(zone, instanceID)
	if err != nil {
		return errors.Wrapf(err, "check if static public IP is already assigned to instance %s", instanceID)
	}

	// get available reserved public IP addresses
	addresses, err := a.listAddresses(filter, orderBy, reservedStatus)
	if err != nil {
		return errors.Wrap(err, "failed to list available addresses")
	}
	if len(addresses) == 0 {
		return errors.Errorf("no available addresses")
	}
	// log available addresses IPs
	ips := make([]string, 0, len(addresses))
	for _, address := range addresses {
		ips = append(ips, address.Address)
	}
	a.logger.WithField("addresses", ips).Debugf("found %d available addresses", len(addresses))

	// delete current ephemeral public IP address
	if err = a.DeleteInstanceAddress(ctx, instance, zone); err != nil {
		return errors.Wrap(err, "failed to delete current public IP address")
	}

	// get instance details again to refresh the network interface fingerprint (required for adding a new ipv6 address)
	instance, err = a.instanceGetter.Get(a.project, zone, instanceID)
	if err != nil {
		return errors.Wrapf(err, "failed refresh network interface fingerprint for instance %s", instanceID)
	}

	// try to assign all available addresses until one succeeds
	// due to concurrency, it is possible that another kubeip instance will assign the same address
	for _, address := range addresses {
		if err = tryAssignAddress(ctx, a, instance, a.region, zone, address); err != nil {
			a.logger.WithError(err).Errorf("failed to assign static public IP address %s", address.Address)
			continue
		}
		// break the loop after successfully assigning an address
		break
	}
	if err != nil {
		return errors.Wrap(err, "failed to assign static public IP address")
	}
	return nil
}

func (a *gcpAssigner) checkStaticIPAssigned(zone, instanceID string) (*compute.Instance, error) {
	instance, err := a.instanceGetter.Get(a.project, zone, instanceID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get instance %s", instanceID)
	}
	assigned, err := a.listAddresses(nil, "", inUseStatus)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list assigned addresses")
	}
	// create a map of users for quick lookup
	users := a.createUserMap(assigned)
	// check if the instance's self link is in the list of users
	if _, ok := users[instance.SelfLink]; ok {
		return nil, ErrStaticIPAlreadyAssigned
	}
	return instance, nil
}

func (a *gcpAssigner) listAddresses(filter []string, orderBy, status string) ([]*compute.Address, error) {
	call := a.lister.List(a.project, a.region)
	// Initialize filters with known filters
	filters := []string{
		fmt.Sprintf("(status=%s)", status),
		"(addressType=EXTERNAL)",
	}
	if a.ipv6 {
		filters = append(filters, "(ipVersion=IPV6)")
	} else {
		filters = append(filters, "(ipVersion!=IPV6)")
	}

	// filter addresses by provided filter: labels.key=value
	for _, f := range filter {
		filters = append(filters, fmt.Sprintf("(%s)", f))
	}
	// set the filter
	call = call.Filter(strings.Join(filters, " "))
	// sort addresses by
	if orderBy != "" {
		call = call.OrderBy(orderBy)
	}
	// get all addresses
	var addresses []*compute.Address
	for {
		list, err := call.Do()
		if err != nil {
			return nil, errors.Wrap(err, "failed to list available addresses")
		}
		addresses = append(addresses, list.Items...)
		if list.NextPageToken == "" {
			return addresses, nil
		}
		call = call.PageToken(list.NextPageToken)
	}
}

func (a *gcpAssigner) Unassign(ctx context.Context, instanceID, zone string) error {
	// get the instance details
	instance, err := a.instanceGetter.Get(a.project, zone, instanceID)
	if err != nil {
		return errors.Wrapf(err, "failed to get instance %s", instanceID)
	}
	// list all assigned addresses
	assigned, err := a.listAddresses(nil, "", inUseStatus)
	if err != nil {
		return errors.Wrap(err, "failed to list assigned addresses")
	}
	if len(assigned) == 0 {
		return ErrNoStaticIPAssigned
	}

	// create a map of users for quick lookup
	users := a.createUserMap(assigned)

	// check if the instance's self link is in the list of users
	if _, ok := users[instance.SelfLink]; ok {
		// release/remove current static public IP address
		if err = a.DeleteInstanceAddress(ctx, instance, zone); err != nil {
			return errors.Wrap(err, "failed to delete current public IP address")
		}
		// get instance details again to refresh the network interface fingerprint (required for adding a new ipv6 address)
		instance, err = a.instanceGetter.Get(a.project, zone, instanceID)
		if err != nil {
			return errors.Wrapf(err, "failed refresh network interface fingerprint for instance %s", instanceID)
		}
		// assign ephemeral public IP address to the instance (pass nil address)
		if err = retryAddEphemeralAddress(ctx, a.logger, a, instance, zone); err != nil {
			return errors.Wrap(err, "failed to assign ephemeral public IP address")
		}
	}
	return nil
}

func getAccessConfig(networkInterface *compute.NetworkInterface, ipv6 bool) (*compute.AccessConfig, error) {
	if ipv6 {
		if len(networkInterface.Ipv6AccessConfigs) == 0 {
			return nil, errors.New("instance network interface has no IPv6 access configs")
		}
		return networkInterface.Ipv6AccessConfigs[0], nil
	}
	if len(networkInterface.AccessConfigs) == 0 {
		return nil, errors.New("instance network interface has no access configs")
	}
	return networkInterface.AccessConfigs[0], nil
}

func getNetworkInterface(instance *compute.Instance) (*compute.NetworkInterface, error) {
	if len(instance.NetworkInterfaces) == 0 {
		return nil, errors.New("instance has no network interfaces")
	}
	return instance.NetworkInterfaces[0], nil
}

func tryAssignAddress(ctx context.Context, as internalAssigner, instance *compute.Instance, region, zone string, address *compute.Address) error {
	// Force check if address is already assigned
	addressAssigned, err := as.CheckAddressAssigned(region, address.Name)
	if err != nil {
		return errors.Wrap(err, "failed to check if address is assigned")
	}
	if addressAssigned {
		return errors.New("address is already assigned")
	}
	// Assign address to the instance
	if err = as.AddInstanceAddress(ctx, instance, zone, address); err != nil {
		return errors.Wrap(err, "failed to assign static public IP address")
	}
	return nil
}

func (a *gcpAssigner) createUserMap(assigned []*compute.Address) map[string]struct{} {
	users := make(map[string]struct{})
	for _, address := range assigned {
		for _, user := range address.Users {
			users[user] = struct{}{}
		}
	}
	return users
}

func retryAddEphemeralAddress(ctx context.Context, logger *logrus.Entry, as internalAssigner, instance *compute.Instance, zone string) error {
	for i := 0; i < maxRetries; i++ {
		if err := as.AddInstanceAddress(ctx, instance, zone, nil); err != nil {
			logger.WithError(err).Error("failed to assign ephemeral public IP address, retrying")
			continue
		}
		return nil
	}
	return errors.New("reached max retries")
}

func createAccessConfig(address *compute.Address, ipv6 bool) *compute.AccessConfig {
	accessConfig := &compute.AccessConfig{
		Name: defaultNetworkName,
		Type: defaultAccessConfigType,
		Kind: accessConfigKind,
	}

	if ipv6 {
		accessConfig.Name = defaultNetworkNameIPv6
		accessConfig.Type = defaultAccessConfigIPv6Type
		accessConfig.ExternalIpv6 = addressAddressOrEmpty(address)
		accessConfig.NetworkTier = addressNetworkTierOrDefault(address)
		accessConfig.ExternalIpv6PrefixLength = addressPrefixLengthOrDefault(address)
	} else {
		accessConfig.NatIP = addressAddressOrEmpty(address)
	}

	return accessConfig
}

func addressAddressOrEmpty(address *compute.Address) string {
	if address == nil {
		return ""
	}
	return address.Address
}

func addressNetworkTierOrDefault(address *compute.Address) string {
	if address == nil {
		return defaultNetworkTier
	}
	return address.NetworkTier
}

func addressPrefixLengthOrDefault(address *compute.Address) int64 {
	if address == nil {
		return defaultPrefixLength
	}
	return address.PrefixLength
}
