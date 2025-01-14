package address

import (
	"context"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/doitintl/kubeip/internal/cloud"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	shorthandFilterTokens = 2
)

type awsAssigner struct {
	region         string
	logger         *logrus.Entry
	instanceGetter cloud.Ec2InstanceGetter
	eipLister      cloud.EipLister
	eipAssigner    cloud.EipAssigner
}

func NewAwsAssigner(ctx context.Context, logger *logrus.Entry, region string) (Assigner, error) {
	// initialize AWS client
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, errors.Wrap(err, "failed to load AWS config")
	}

	// create AWS client for EC2 service in the given region with default config and credentials
	client := ec2.NewFromConfig(cfg)

	// initialize AWS instance getter
	instanceGetter := cloud.NewEc2InstanceGetter(client)

	// initialize AWS elastic IP lister
	eipLister := cloud.NewEipLister(client)

	// initialize AWS elastic IP internalAssigner
	eipAssigner := cloud.NewEipAssigner(client)

	return &awsAssigner{
		region:         region,
		logger:         logger,
		instanceGetter: instanceGetter,
		eipLister:      eipLister,
		eipAssigner:    eipAssigner,
	}, nil
}

// parseShorthandFilter parses shorthand filter string into filter name and values
// shorthand filter format: Name=string,Values=string,string ...
// https://awscli.amazonaws.com/v2/documentation/api/latest/reference/ec2/describe-addresses.html#options
func parseShorthandFilter(filter string) (string, []string, error) {
	// split filter by the first ","
	exp := strings.SplitN(filter, ",", shorthandFilterTokens)
	if len(exp) != shorthandFilterTokens {
		return "", nil, errors.New("invalid filter format; supported format Name=string,Values=string,string,")
	}
	// get filter name
	name := strings.Split(exp[0], "=")
	if len(name) != 2 || name[0] != "Name" {
		return "", nil, errors.New("invalid filter Name")
	}
	// get filter values
	values := strings.Split(exp[1], "=")
	if len(values) != 2 || values[0] != "Values" {
		return "", nil, errors.New("invalid filter Values list")
	}
	listValues := strings.Split(values[1], ",")
	return name[1], listValues, nil
}

func sortAddressesByTag(addresses []types.Address, key string) {
	sort.Slice(addresses, func(i, j int) bool {
		if addresses[i].Tags == nil {
			return false
		}
		if addresses[j].Tags == nil {
			return true
		}
		for _, tag := range addresses[i].Tags {
			if *tag.Key == key {
				for _, tag2 := range addresses[j].Tags {
					if *tag2.Key == key {
						return *tag.Value < *tag2.Value
					}
				}
			}
		}
		return false
	})
}

// sortAddressesByField sorts addresses by the given field
// if sortBy is Tag:<key>, sort addresses by tag value
func sortAddressesByField(addresses []types.Address, sortBy string) {
	// if sortBy is Tag:<key>, sort addresses by tag value
	if strings.HasPrefix(sortBy, "Tag:") {
		key := strings.TrimPrefix(sortBy, "Tag:")
		sortAddressesByTag(addresses, key)
		return // return if sortBy is Tag:<key>
	}
	// sort addresses by orderBy field
	switch sortBy {
	case "AllocationId":
		sort.Slice(addresses, func(i, j int) bool {
			return *addresses[i].AllocationId < *addresses[j].AllocationId
		})
	case "AssociationId":
		sort.Slice(addresses, func(i, j int) bool {
			return *addresses[i].AssociationId < *addresses[j].AssociationId
		})
	case "Domain":
		sort.Slice(addresses, func(i, j int) bool {
			return addresses[i].Domain < addresses[j].Domain
		})
	case "InstanceId":
		sort.Slice(addresses, func(i, j int) bool {
			return *addresses[i].InstanceId < *addresses[j].InstanceId
		})
	case "NetworkInterfaceId":
		sort.Slice(addresses, func(i, j int) bool {
			return *addresses[i].NetworkInterfaceId < *addresses[j].NetworkInterfaceId
		})
	case "NetworkInterfaceOwnerId":
		sort.Slice(addresses, func(i, j int) bool {
			return *addresses[i].NetworkInterfaceOwnerId < *addresses[j].NetworkInterfaceOwnerId
		})
	case "PrivateIpAddress":
		sort.Slice(addresses, func(i, j int) bool {
			return *addresses[i].PrivateIpAddress < *addresses[j].PrivateIpAddress
		})
	case "PublicIp":
		sort.Slice(addresses, func(i, j int) bool {
			return *addresses[i].PublicIp < *addresses[j].PublicIp
		})
	case "PublicIpv4Pool":
		sort.Slice(addresses, func(i, j int) bool {
			return *addresses[i].PublicIpv4Pool < *addresses[j].PublicIpv4Pool
		})
	}
}

func (a *awsAssigner) forceCheckAddressAssigned(ctx context.Context, allocationID string) (bool, error) {
	// get elastic IP attached to the allocation ID
	filters := make(map[string][]string)
	filters["allocation-id"] = []string{allocationID}
	addresses, err := a.eipLister.List(ctx, filters, true)
	if err != nil {
		return false, errors.Wrapf(err, "failed to list elastic IPs by allocation-id %s", allocationID)
	}
	if len(addresses) == 0 {
		return false, nil
	}
	// check if the first address (and the only) is assigned
	if addresses[0].AssociationId != nil {
		return true, nil
	}
	return false, nil
}

func (a *awsAssigner) Assign(ctx context.Context, instanceID, _ string, filter []string, orderBy string) (string, error) {
	// get elastic IP attached to the instance
	err := a.checkElasticIPAssigned(ctx, instanceID)
	if err != nil {
		return "", errors.Wrapf(err, "check if elastic IP is already assigned to instance %s", instanceID)
	}

	// get available elastic IPs based on filter and orderBy
	addresses, err := a.getAvailableElasticIPs(ctx, filter, orderBy)
	if err != nil {
		return "", errors.Wrap(err, "failed to get available elastic IPs")
	}

	// get EC2 instance
	instance, err := a.instanceGetter.Get(ctx, instanceID, a.region)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get instance %s", instanceID)
	}
	// get primary network interface ID with public IP address (DeviceIndex == 0)
	networkInterfaceID, err := a.getNetworkInterfaceID(instance)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get network interface ID for instance %s", instanceID)
	}

	// try to assign available addresses until succeeds
	// due to concurrency, it is possible that another kubeip instance will assign the same address
	var assignedAddress string
	for i := range addresses {
		a.logger.WithFields(logrus.Fields{
			"instance":           instanceID,
			"address":            *addresses[i].PublicIp,
			"allocation_id":      *addresses[i].AllocationId,
			"networkInterfaceID": networkInterfaceID,
		}).Debug("assigning elastic IP to the instance")
		err = a.tryAssignAddress(ctx, &addresses[i], networkInterfaceID, instanceID)
		if err != nil {
			a.logger.WithError(err).Warn("failed to assign elastic IP address")
			a.logger.Debug("retrying with another address")
		} else {
			a.logger.WithFields(logrus.Fields{
				"instance":      instanceID,
				"address":       *addresses[i].PublicIp,
				"allocation_id": *addresses[i].AllocationId,
			}).Info("elastic IP assigned to the instance")
			assignedAddress = *addresses[i].PublicIp
			break // break if address assigned successfully
		}
	}
	if err != nil {
		return "", errors.Wrap(err, "failed to assign elastic IP address")
	}
	return assignedAddress, nil
}

func (a *awsAssigner) tryAssignAddress(ctx context.Context, address *types.Address, networkInterfaceID, instanceID string) error {
	// force check if address is already assigned (reduce the chance of assigning the same address by multiple kubeip instances)
	addressAssigned, err := a.forceCheckAddressAssigned(ctx, *address.AllocationId)
	if err != nil {
		return errors.Wrapf(err, "failed to check if address %s is assigned", *address.PublicIp)
	}
	if addressAssigned {
		return errors.Errorf("address %s is already assigned", *address.PublicIp)
	}
	if err = a.eipAssigner.Assign(ctx, networkInterfaceID, *address.AllocationId); err != nil {
		return errors.Wrapf(err, "failed to assign elastic IP %s to the instance %s", *address.PublicIp, instanceID)
	}
	return nil
}

func (a *awsAssigner) getNetworkInterfaceID(instance *types.Instance) (string, error) {
	// get network interface ID
	if len(instance.NetworkInterfaces) == 0 {
		return "", errors.Errorf("no network interfaces found for instance %s", *instance.InstanceId)
	}
	// get primary network interface ID with public IP address (DeviceIndex == 0)
	networkInterfaceID := ""
	for _, ni := range instance.NetworkInterfaces {
		if ni.Association != nil && ni.Association.PublicIp != nil &&
			ni.Attachment != nil && ni.Attachment.DeviceIndex != nil && *ni.Attachment.DeviceIndex == 0 {
			networkInterfaceID = *ni.NetworkInterfaceId
			break
		}
	}
	if networkInterfaceID == "" {
		return "", errors.Errorf("no network interfaces with public IP address found for instance %s", *instance.InstanceId)
	}
	return networkInterfaceID, nil
}

func (a *awsAssigner) checkElasticIPAssigned(ctx context.Context, instanceID string) error {
	filters := make(map[string][]string)
	filters["instance-id"] = []string{instanceID}
	addresses, err := a.eipLister.List(ctx, filters, true)
	if err != nil {
		return errors.Wrapf(err, "failed to list elastic IPs attached to instance %s", instanceID)
	}
	if len(addresses) > 0 {
		return ErrStaticIPAlreadyAssigned
	}
	return nil
}

func (a *awsAssigner) getAssignedElasticIP(ctx context.Context, instanceID string) (*types.Address, error) {
	// get elastic IP attached to the instance
	filters := make(map[string][]string)
	filters["instance-id"] = []string{instanceID}
	addresses, err := a.eipLister.List(ctx, filters, true)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list elastic IPs attached to instance %s", instanceID)
	}
	if len(addresses) == 0 {
		return nil, ErrNoStaticIPAssigned
	}
	return &addresses[0], nil
}

func (a *awsAssigner) getAvailableElasticIPs(ctx context.Context, filter []string, orderBy string) ([]types.Address, error) {
	filters := make(map[string][]string)
	for _, f := range filter {
		name, values, err := parseShorthandFilter(f)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse filter %s", f)
		}
		filters[name] = values
	}
	addresses, err := a.eipLister.List(ctx, filters, false)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list available elastic IPs")
	}
	if len(addresses) == 0 {
		return nil, errors.Errorf("no available elastic IPs")
	}
	// sort addresses by orderBy field
	sortAddressesByField(addresses, orderBy)
	// log available addresses IPs
	ips := make([]string, 0, len(addresses))
	for _, address := range addresses {
		ips = append(ips, *address.PublicIp)
	}
	a.logger.WithField("addresses", ips).Debugf("Found %d available addresses", len(addresses))

	return addresses, nil
}

func (a *awsAssigner) Unassign(ctx context.Context, instanceID, _ string) error {
	// get elastic IP attached to the instance
	address, err := a.getAssignedElasticIP(ctx, instanceID)
	if err != nil {
		return errors.Wrapf(err, "check if elastic IP is assigned to instance %s", instanceID)
	}
	// unassign elastic IP from the instance
	if err = a.eipAssigner.Unassign(ctx, *address.AssociationId); err != nil {
		return errors.Wrap(err, "failed to unassign elastic IP")
	}
	a.logger.WithFields(logrus.Fields{
		"instance":      instanceID,
		"address":       *address.PublicIp,
		"allocation_id": *address.AllocationId,
		"associationId": *address.AssociationId,
	}).Info("elastic IP unassigned from the instance")

	return nil
}
