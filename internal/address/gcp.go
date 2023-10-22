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
	operationDone           = "DONE" // operation status DONE
	inUseStatus             = "IN_USE"
	reservedStatus          = "RESERVED" // static IP addresses that are reserved but not currently in use
	defaultTimeout          = 10 * time.Minute
	defaultNetworkInterface = "nic0"
	defaultNetworkName      = "External NAT"
	accessConfigType        = "ONE_TO_ONE_NAT"
	accessConfigKind        = "compute#accessConfig"
)

type gcpAssigner struct {
	lister         cloud.Lister
	waiter         cloud.ZoneWaiter
	addressManager cloud.AddressManager
	instanceGetter cloud.InstanceGetter
	project        string
	region         string
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

func NewGCPAssigner(ctx context.Context, logger *logrus.Entry, project, region string) (Assigner, error) {
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
		if len(region) > 3 && region[len(region)-3] == '-' {
			region = region[:len(region)-3]
		}
	}

	return &gcpAssigner{
		lister:         cloud.NewLister(client),
		waiter:         cloud.NewZoneWaiter(client),
		addressManager: cloud.NewAddressManager(client),
		instanceGetter: cloud.NewInstanceGetter(client),
		project:        project,
		region:         region,
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

func (a *gcpAssigner) deleteInstanceAddress(ctx context.Context, instance *compute.Instance, zone string) error {
	// Check if the instance has at least one network interface
	if len(instance.NetworkInterfaces) == 0 {
		a.logger.WithField("instance", instance.Name).Info("instance has no network interfaces")
		return nil
	}
	// get instance network interface
	networkInterface := instance.NetworkInterfaces[0]
	// get instance network interface access config
	if len(networkInterface.AccessConfigs) == 0 {
		a.logger.WithField("instance", instance.Name).Info("instance network interface has no access configs")
		return nil
	}
	accessConfig := networkInterface.AccessConfigs[0]
	// get instance network interface access config name
	accessConfigName := accessConfig.Name
	// delete instance network interface access config
	a.logger.WithFields(logrus.Fields{
		"instance": instance.Name,
		"address":  accessConfig.NatIP,
	}).Infof("deleting public IP address from instance")
	op, err := a.addressManager.DeleteAccessConfig(a.project, zone, instance.Name, accessConfigName, networkInterface.Name)
	if err != nil {
		return errors.Wrapf(err, "failed to delete access config %s from instance %s", accessConfigName, instance.Name)
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

func (a *gcpAssigner) addInstanceAddress(ctx context.Context, instance *compute.Instance, zone string, address *compute.Address) error {
	// empty address means ephemeral public IP address
	natIP := ""
	name := defaultNetworkName
	kind := "ephemeral"
	if address != nil {
		natIP = address.Address
		name = address.Name
		kind = "static"
	}
	// add instance network interface access config
	a.logger.WithFields(logrus.Fields{
		"instance":     instance.Name,
		"address-name": name,
		"address-ip":   natIP,
		"kind":         kind,
	}).Info("adding public IP address to instance")
	op, err := a.addressManager.AddAccessConfig(a.project, zone, instance.Name, defaultNetworkInterface, &compute.AccessConfig{
		Name:  name,
		Type:  accessConfigType,
		Kind:  accessConfigKind,
		NatIP: natIP,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to add access config %s to instance %s", name, instance.Name)
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

func (a *gcpAssigner) forceCheckAddressAssigned(region, addressName string) (bool, error) {
	address, err := a.addressManager.GetAddress(a.project, region, addressName)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get address %s", addressName)
	}
	return address.Status == inUseStatus, nil
}

//nolint:gocyclo
func (a *gcpAssigner) Assign(ctx context.Context, instanceID, zone string, filter []string, orderBy string) error {
	// check if instance already has a public static IP address assigned
	instance, err := a.instanceGetter.Get(a.project, zone, instanceID)
	if err != nil {
		return errors.Wrapf(err, "failed to get instance %s", instanceID)
	}
	assigned, err := a.listAddresses(nil, "", inUseStatus)
	if err != nil {
		return errors.Wrap(err, "failed to list assigned addresses")
	}
	if len(assigned) > 0 {
		for _, address := range assigned {
			for _, user := range address.Users {
				if user == instance.SelfLink {
					a.logger.WithFields(logrus.Fields{
						"instance": instanceID,
						"address":  address.Address,
					}).Infof("instance already has a static public IP address assigned")
					return nil
				}
			}
		}
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
	if err = a.deleteInstanceAddress(ctx, instance, zone); err != nil {
		return errors.Wrap(err, "failed to delete current public IP address")
	}

	// try to assign all available addresses until one succeeds
	// due to concurrency, it is possible that another kubeip instance will assign the same address
	for _, address := range addresses {
		// force check if address is already assigned (reduce the chance of assigning the same address by multiple kubeip instances)
		var addressAssigned bool
		addressAssigned, err = a.forceCheckAddressAssigned(a.region, address.Name)
		if err != nil {
			a.logger.WithError(err).Errorf("failed to check if address %s is assigned", address.Address)
			a.logger.Debug("trying next address")
			continue
		}
		if addressAssigned {
			a.logger.WithField("address", address.Address).Debug("address is already assigned")
			a.logger.Debug("trying next address")
			continue
		}
		// assign address to the instance and try the next address if it fails
		if err = a.addInstanceAddress(ctx, instance, zone, address); err != nil {
			a.logger.WithError(err).Errorf("failed to assign static public IP address %s", address.Address)
			a.logger.Debug("trying next address")
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

func (a *gcpAssigner) listAddresses(filter []string, orderBy, status string) ([]*compute.Address, error) {
	call := a.lister.List(a.project, a.region)
	// Initialize filters with known filters
	filters := []string{
		fmt.Sprintf("(status=%s)", status),
		"(addressType=EXTERNAL)",
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
	// if there are assigned addresses, check if they are assigned to the instance
	if len(assigned) > 0 {
		for _, address := range assigned {
			for _, user := range address.Users {
				if user == instance.SelfLink {
					// release/remove current static public IP address
					if err = a.deleteInstanceAddress(ctx, instance, zone); err != nil {
						return errors.Wrap(err, "failed to delete current public IP address")
					}
					// assign ephemeral public IP address to the instance (pass nil address)
					for {
						// retry until ephemeral public IP address is assigned
						// sometime this operation fails and needs to be retried
						if err = a.addInstanceAddress(ctx, instance, zone, nil); err != nil {
							a.logger.WithError(err).Error("failed to assign ephemeral public IP address, retrying")
							continue
						}
						return nil
					}
				}
			}
		}
	}
	return nil
}
