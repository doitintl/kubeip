package address

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/pkg/errors"
)

const (
	ipamOwnerTag = "kubeip.com/instance"
	ipamPoolTag  = "kubeip.com/ipam-pool"
)

// IPAMState persists allocation intent across process restarts.
type IPAMState interface {
	Load(context.Context) (string, error)
	CompareAndSwap(ctx context.Context, oldValue, newValue string) error
}

type ipamAPI interface {
	AllocateAddress(context.Context, *ec2.AllocateAddressInput, ...func(*ec2.Options)) (*ec2.AllocateAddressOutput, error)
	DescribeAddresses(context.Context, *ec2.DescribeAddressesInput, ...func(*ec2.Options)) (*ec2.DescribeAddressesOutput, error)
	ReleaseAddress(context.Context, *ec2.ReleaseAddressInput, ...func(*ec2.Options)) (*ec2.ReleaseAddressOutput, error)
}

func (a *awsAssigner) ownedIPAMAddresses(ctx context.Context, instanceID string) ([]types.Address, error) {
	result, err := a.ipamClient.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{Filters: []types.Filter{
		{Name: aws.String("tag:" + ipamOwnerTag), Values: []string{instanceID}},
		{Name: aws.String("tag:" + ipamPoolTag), Values: []string{a.ipamPoolID}},
	}})
	if err != nil {
		return nil, errors.Wrap(err, "finding owned IPAM addresses")
	}
	if len(result.Addresses) > 1 {
		return nil, errors.New("multiple owned IPAM addresses; manual reconciliation required")
	}
	return result.Addresses, nil
}

func (a *awsAssigner) allocateIPAM(ctx context.Context, instanceID, intent string) (*types.Address, error) {
	// AllocateAddress has no client token. Persist intent and disable SDK retries.
	// An ambiguous result is recovered by tags, never by allocating again.
	if err := a.ipamState.CompareAndSwap(ctx, "", intent); err != nil {
		return nil, errors.Wrap(err, "claiming IPAM allocation")
	}
	result, err := a.ipamClient.AllocateAddress(ctx, &ec2.AllocateAddressInput{
		Domain: types.DomainTypeVpc, IpamPoolId: aws.String(a.ipamPoolID),
		TagSpecifications: []types.TagSpecification{{ResourceType: types.ResourceTypeElasticIp, Tags: []types.Tag{
			{Key: aws.String(ipamOwnerTag), Value: aws.String(instanceID)},
			{Key: aws.String(ipamPoolTag), Value: aws.String(a.ipamPoolID)},
		}}},
	}, func(o *ec2.Options) { o.Retryer = aws.NopRetryer{} })
	if err != nil {
		return nil, errors.Wrap(err, "allocating IPAM address; intent retained for recovery")
	}
	if aws.ToString(result.AllocationId) == "" || aws.ToString(result.PublicIp) == "" {
		return nil, errors.New("incomplete IPAM allocation response; intent retained for recovery")
	}
	return &types.Address{AllocationId: result.AllocationId, PublicIp: result.PublicIp}, nil
}

func (a *awsAssigner) assignIPAM(ctx context.Context, instanceID string) (string, error) {
	intent := a.ipamPoolID + "/" + instanceID
	state, err := a.ipamState.Load(ctx)
	if err != nil {
		return "", errors.Wrap(err, "loading IPAM allocation intent")
	}
	if state != "" && state != intent {
		return "", errors.New("IPAM pool or instance differs from persisted intent")
	}
	instance, err := a.instanceGetter.Get(ctx, instanceID, a.region)
	if err != nil {
		return "", errors.Wrap(err, "getting IPAM target instance")
	}
	nic, err := a.getNetworkInterfaceID(instance)
	if err != nil {
		return "", err
	}
	addresses, err := a.ownedIPAMAddresses(ctx, instanceID)
	if err != nil {
		return "", err
	}
	if err := a.checkIPAMPrimary(ctx, instanceID, addresses); err != nil {
		return "", err
	}
	address, err := a.recoverIPAM(ctx, instanceID, intent, state, addresses)
	if err != nil {
		return "", err
	}
	if aws.ToString(address.AssociationId) != "" {
		if aws.ToString(address.InstanceId) == instanceID && aws.ToString(address.NetworkInterfaceId) == nic &&
			aws.ToString(address.PublicIp) == aws.ToString(instance.PublicIpAddress) {
			return aws.ToString(address.PublicIp), nil
		}
		return "", errors.New("owned IPAM address is associated elsewhere; refusing reassociation")
	}
	if err := a.tryAssignAddress(ctx, address, nic, instanceID); err != nil {
		// Keep the allocation for the next retry, including ambiguous association results.
		return "", errors.Wrap(err, "associating IPAM address; allocation retained for retry")
	}
	return aws.ToString(address.PublicIp), nil
}

func (a *awsAssigner) releaseIPAM(ctx context.Context, instanceID string) error {
	intent := a.ipamPoolID + "/" + instanceID
	state, err := a.ipamState.Load(ctx)
	if err != nil {
		return errors.Wrap(err, "loading IPAM release intent")
	}
	if state != intent {
		return errors.New("IPAM release requires matching persisted allocation intent")
	}
	addresses, err := a.ownedIPAMAddresses(ctx, instanceID)
	if err != nil {
		return err
	}
	if len(addresses) == 0 {
		return errors.New("IPAM allocation not found; intent retained for manual reconciliation")
	}
	address := &addresses[0]
	if aws.ToString(address.AssociationId) != "" {
		assigned, err := a.getAssignedElasticIP(ctx, instanceID)
		if err != nil {
			return errors.Wrap(err, "checking IPAM release ownership")
		}
		if aws.ToString(assigned.AllocationId) != aws.ToString(address.AllocationId) {
			return errors.New("owned IPAM address is not the instance primary address; refusing release")
		}
		if err := a.eipAssigner.Unassign(ctx, *address.AssociationId); err != nil {
			return errors.Wrap(err, "disassociating IPAM address")
		}
	}
	if _, err := a.ipamClient.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{AllocationId: address.AllocationId}); err != nil {
		return errors.Wrap(err, "releasing owned IPAM allocation")
	}
	return errors.Wrap(a.ipamState.CompareAndSwap(ctx, intent, ""), "clearing released IPAM intent")
}

func (a *awsAssigner) recoverIPAM(ctx context.Context, instanceID, intent, state string, addresses []types.Address) (*types.Address, error) {
	if len(addresses) != 0 {
		if aws.ToString(addresses[0].AllocationId) == "" || aws.ToString(addresses[0].PublicIp) == "" {
			return nil, errors.New("owned IPAM address is missing its allocation ID or public IP")
		}
		if state == "" {
			if err := a.ipamState.CompareAndSwap(ctx, "", intent); err != nil {
				return nil, errors.Wrap(err, "recovering IPAM allocation intent")
			}
		}
		return &addresses[0], nil
	}
	if state != "" {
		return nil, errors.New("IPAM allocation outcome unresolved; inspect tagged addresses before resetting Node intent")
	}
	return a.allocateIPAM(ctx, instanceID, intent)
}

func (a *awsAssigner) checkIPAMPrimary(ctx context.Context, instanceID string, addresses []types.Address) error {
	assigned, err := a.getAssignedElasticIP(ctx, instanceID)
	if err != nil && !errors.Is(err, ErrNoStaticIPAssigned) {
		return err
	}
	if err == nil && (len(addresses) == 0 || aws.ToString(assigned.AllocationId) != aws.ToString(addresses[0].AllocationId)) {
		return errors.New("primary interface already has an Elastic IP not owned by this IPAM configuration")
	}
	return nil
}
