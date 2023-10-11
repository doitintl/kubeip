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

	// initialize AWS elastic IP assigner
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

// sortAddressesByField sorts addresses by the given field
// if sortBy is Tag:<key>, sort addresses by tag value
func sortAddressesByField(addresses []types.Address, sortBy string) {
	// if sortBy is Tag:<key>, sort addresses by tag value
	if strings.HasPrefix(sortBy, "Tag:") {
		key := strings.TrimPrefix(sortBy, "Tag:")
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

func (a *awsAssigner) Assign(ctx context.Context, instanceID, zone string, filter []string, orderBy string) error {
	// get elastic IP attached to the instance
	filters := make(map[string][]string)
	filters["instance-id"] = []string{instanceID}
	addresses, err := a.eipLister.List(ctx, filters, true)
	if err != nil {
		return errors.Wrapf(err, "failed to list elastic IPs attached to instance %s", instanceID)
	}
	if len(addresses) > 0 {
		a.logger.Infof("elastic IP %s is already attached to instance %s", *addresses[0].PublicIp, instanceID)
		return nil
	}

	// get available elastic IPs
	filters = make(map[string][]string)
	for _, f := range filter {
		name, values, err2 := parseShorthandFilter(f)
		if err2 != nil {
			return errors.Wrapf(err2, "failed to parse filter %s", f)
		}
		filters[name] = values
	}
	addresses, err = a.eipLister.List(context.Background(), filters, false)
	if err != nil {
		return errors.Wrap(err, "failed to list available elastic IPs")
	}

	// if no available elastic IPs, return error
	if len(addresses) == 0 {
		return errors.Errorf("no available elastic IPs")
	}

	// get EC2 instance
	instance, err := a.instanceGetter.Get(ctx, instanceID, a.region)
	if err != nil {
		return errors.Wrapf(err, "failed to get instance %s", instanceID)
	}
	// get network interface ID
	if instance.NetworkInterfaces == nil || len(instance.NetworkInterfaces) == 0 {
		return errors.Errorf("no network interfaces found for instance %s", instanceID)
	}
	// get network interface ID of network interface with public IP address
	networkInterfaceID := ""
	for _, ni := range instance.NetworkInterfaces {
		if ni.Association != nil && ni.Association.PublicIp != nil {
			networkInterfaceID = *ni.NetworkInterfaceId
		}
	}
	if networkInterfaceID == "" {
		return errors.Errorf("no network interfaces with public IP address found for instance %s", instanceID)
	}

	// sort addresses by orderBy field
	sortAddressesByField(addresses, orderBy)

	// assign the first available elastic IP to the instance
	address := addresses[0]
	if err = a.eipAssigner.Assign(ctx, instanceID, networkInterfaceID, &address); err != nil {
		return errors.Wrap(err, "failed to assign elastic IP")
	}
	a.logger.WithFields(logrus.Fields{
		"instance":      instanceID,
		"address":       *address.PublicIp,
		"allocation_id": *address.AllocationId,
	}).Info("elastic IP assigned to the instance")

	return nil
}
