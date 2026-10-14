package address

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/doitintl/kubeip/internal/config"
	nodetypes "github.com/doitintl/kubeip/internal/types"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

type ipamFixture struct {
	t                                                        *testing.T
	state                                                    string
	addresses                                                []types.Address
	instance                                                 *types.Instance
	allocated, associated, disassociated, released           int
	allocateErr, associateErr, releaseErr, listErr, stateErr error
	hidden                                                   bool
}

func newIPAMFixture(t *testing.T) (*awsAssigner, *ipamFixture) {
	f := &ipamFixture{t: t, instance: &types.Instance{
		InstanceId: aws.String("i-test"), PublicIpAddress: aws.String("198.51.100.1"),
		NetworkInterfaces: []types.InstanceNetworkInterface{{NetworkInterfaceId: aws.String("eni-primary"),
			Association: &types.InstanceNetworkInterfaceAssociation{PublicIp: aws.String("198.51.100.1")},
			Attachment:  &types.InstanceNetworkInterfaceAttachment{DeviceIndex: aws.Int32(0)}}},
	}}
	return &awsAssigner{ipamPoolID: "ipam-pool-test", region: "us-east-1", ipamClient: f, ipamState: f,
		instanceGetter: f, eipLister: f, eipAssigner: f, logger: logrus.NewEntry(logrus.New())}, f
}
func (f *ipamFixture) Load(context.Context) (string, error) { return f.state, f.stateErr }
func (f *ipamFixture) CompareAndSwap(_ context.Context, oldValue, newValue string) error {
	if f.stateErr != nil {
		return f.stateErr
	}
	if f.state != oldValue {
		return errors.New("state conflict")
	}
	f.state = newValue
	return nil
}
func (f *ipamFixture) Get(context.Context, string, string) (*types.Instance, error) {
	return f.instance, nil
}
func (f *ipamFixture) DescribeAddresses(_ context.Context, in *ec2.DescribeAddressesInput, _ ...func(*ec2.Options)) (*ec2.DescribeAddressesOutput, error) {
	require.Equal(f.t, []types.Filter{{Name: aws.String("tag:" + ipamOwnerTag), Values: []string{"i-test"}}, {Name: aws.String("tag:" + ipamPoolTag), Values: []string{"ipam-pool-test"}}}, in.Filters)
	if f.hidden {
		return &ec2.DescribeAddressesOutput{}, f.listErr
	}
	return &ec2.DescribeAddressesOutput{Addresses: f.addresses}, f.listErr
}
func (f *ipamFixture) AllocateAddress(_ context.Context, in *ec2.AllocateAddressInput, opts ...func(*ec2.Options)) (*ec2.AllocateAddressOutput, error) {
	require.Equal(f.t, "ipam-pool-test/i-test", f.state)
	require.Equal(f.t, "ipam-pool-test", aws.ToString(in.IpamPoolId))
	require.Equal(f.t, types.DomainTypeVpc, in.Domain)
	require.Equal(f.t, []types.Tag{{Key: aws.String(ipamOwnerTag), Value: aws.String("i-test")}, {Key: aws.String(ipamPoolTag), Value: aws.String("ipam-pool-test")}}, in.TagSpecifications[0].Tags)
	options := ec2.Options{}
	for _, opt := range opts {
		opt(&options)
	}
	require.Equal(f.t, 1, options.Retryer.MaxAttempts())
	f.allocated++
	f.addresses = []types.Address{{AllocationId: aws.String("eipalloc-test"), PublicIp: aws.String("203.0.113.10")}}
	return &ec2.AllocateAddressOutput{AllocationId: f.addresses[0].AllocationId, PublicIp: f.addresses[0].PublicIp}, f.allocateErr
}
func (f *ipamFixture) ReleaseAddress(_ context.Context, in *ec2.ReleaseAddressInput, _ ...func(*ec2.Options)) (*ec2.ReleaseAddressOutput, error) {
	require.Equal(f.t, "eipalloc-test", aws.ToString(in.AllocationId))
	f.released++
	if f.releaseErr == nil {
		f.addresses = nil
	}
	return &ec2.ReleaseAddressOutput{}, f.releaseErr
}
func (f *ipamFixture) List(_ context.Context, filters map[string][]string, inUse bool) ([]types.Address, error) {
	result := []types.Address{}
	for _, a := range f.addresses {
		if (aws.ToString(a.AssociationId) != "") != inUse {
			continue
		}
		if ids, ok := filters["instance-id"]; ok && aws.ToString(a.InstanceId) != ids[0] {
			continue
		}
		if ids, ok := filters["allocation-id"]; ok && aws.ToString(a.AllocationId) != ids[0] {
			continue
		}
		result = append(result, a)
	}
	return result, nil
}
func (f *ipamFixture) Assign(_ context.Context, nic, allocation string) error {
	require.Equal(f.t, "eni-primary", nic)
	require.Equal(f.t, "eipalloc-test", allocation)
	f.associated++
	if f.associateErr != nil {
		return f.associateErr
	}
	f.addresses[0].AssociationId = aws.String("eipassoc-test")
	f.addresses[0].InstanceId = aws.String("i-test")
	f.addresses[0].NetworkInterfaceId = aws.String(nic)
	f.instance.PublicIpAddress = f.addresses[0].PublicIp
	return nil
}
func (f *ipamFixture) Unassign(_ context.Context, association string) error {
	require.Equal(f.t, "eipassoc-test", association)
	f.disassociated++
	f.addresses[0].AssociationId = nil
	return nil
}

func TestIPAMLifecycle(t *testing.T) {
	t.Parallel()
	a, f := newIPAMFixture(t)
	ctx := context.Background()
	ip, err := a.Assign(ctx, "i-test", "", nil, "")
	require.NoError(t, err)
	require.Equal(t, "203.0.113.10", ip)
	// A fresh process recovers through the durable state and cloud tags.
	restarted := *a
	ip, err = restarted.Assign(ctx, "i-test", "", nil, "")
	require.NoError(t, err)
	require.Equal(t, "203.0.113.10", ip)
	require.Equal(t, 1, f.allocated)
	require.Equal(t, 1, f.associated)
	require.NoError(t, restarted.Unassign(ctx, "i-test", ""))
	require.Equal(t, 1, f.disassociated)
	require.Equal(t, 1, f.released)
	require.Empty(t, f.state)
}
func TestIPAMLostAllocationResponse(t *testing.T) {
	t.Parallel()
	a, f := newIPAMFixture(t)
	ctx := context.Background()
	f.allocateErr = errors.New("timeout")
	_, err := a.Assign(ctx, "i-test", "", nil, "")
	require.Error(t, err)
	f.hidden = true
	for range 3 {
		_, err = a.Assign(ctx, "i-test", "", nil, "")
		require.ErrorContains(t, err, "unresolved")
	}
	require.Equal(t, 1, f.allocated)
	require.Zero(t, f.associated)
	f.hidden = false
	_, err = a.Assign(ctx, "i-test", "", nil, "")
	require.NoError(t, err)
	require.Equal(t, 1, f.allocated)
}
func TestIPAMAssociationFailureRetainsAllocation(t *testing.T) {
	t.Parallel()
	a, f := newIPAMFixture(t)
	ctx := context.Background()
	f.associateErr = errors.New("denied")
	_, err := a.Assign(ctx, "i-test", "", nil, "")
	require.Error(t, err)
	require.Zero(t, f.released)
	f.associateErr = nil
	_, err = a.Assign(ctx, "i-test", "", nil, "")
	require.NoError(t, err)
	require.Equal(t, 1, f.allocated)
	require.Equal(t, 2, f.associated)
}
func TestIPAMRefusesOtherAssociation(t *testing.T) {
	t.Parallel()
	a, f := newIPAMFixture(t)
	ctx := context.Background()
	f.state = "ipam-pool-test/i-test"
	f.addresses = []types.Address{{AllocationId: aws.String("eipalloc-test"), PublicIp: aws.String("203.0.113.10"), AssociationId: aws.String("eipassoc-other"), InstanceId: aws.String("i-other")}}
	_, err := a.Assign(ctx, "i-test", "", nil, "")
	require.ErrorContains(t, err, "elsewhere")
	require.Error(t, a.Unassign(ctx, "i-test", ""))
	require.Zero(t, f.allocated)
	require.Zero(t, f.associated)
	require.Zero(t, f.disassociated)
	require.Zero(t, f.released)
}
func TestIPAMFailureBeforeAllocation(t *testing.T) {
	for _, scenario := range []string{"state unavailable", "pool changed", "duplicate allocations", "list denied", "no public interface"} {
		t.Run(scenario, func(t *testing.T) {
			a, f := newIPAMFixture(t)
			switch scenario {
			case "state unavailable":
				f.stateErr = errors.New("forbidden")
			case "pool changed":
				f.state = "other-pool/i-test"
			case "duplicate allocations":
				f.addresses = []types.Address{{}, {}}
			case "list denied":
				f.listErr = errors.New("denied")
			case "no public interface":
				f.instance.NetworkInterfaces = nil
			}
			_, err := a.Assign(context.Background(), "i-test", "", nil, "")
			require.Error(t, err)
			require.Zero(t, f.allocated)
			require.Zero(t, f.associated)
		})
	}
}
func TestIPAMReleaseFailureRetainsIntent(t *testing.T) {
	t.Parallel()
	a, f := newIPAMFixture(t)
	ctx := context.Background()
	_, err := a.Assign(ctx, "i-test", "", nil, "")
	require.NoError(t, err)
	f.releaseErr = errors.New("denied")
	require.Error(t, a.Unassign(ctx, "i-test", ""))
	require.NotEmpty(t, f.state)
	f.releaseErr = nil
	require.NoError(t, a.Unassign(ctx, "i-test", ""))
	require.Empty(t, f.state)
}
func TestIPAMConfiguration(t *testing.T) {
	t.Parallel()
	for _, provider := range []nodetypes.CloudProvider{nodetypes.CloudProviderGCP, nodetypes.CloudProviderOCI, nodetypes.CloudProviderAWS} {
		_, err := NewAssigner(context.Background(), logrus.NewEntry(logrus.New()), provider, &config.Config{AWSIPAMPoolID: "ipam-pool-test"}, nil)
		require.Error(t, err)
	}
	_, f := newIPAMFixture(t)
	for _, cfg := range []*config.Config{{AWSIPAMPoolID: "pool", IPv6: true}, {AWSIPAMPoolID: "pool", Filter: []string{"tag:x=y"}}, {AWSIPAMPoolID: "pool", OrderBy: "AllocationId"}} {
		_, err := NewAssigner(context.Background(), logrus.NewEntry(logrus.New()), nodetypes.CloudProviderAWS, cfg, f)
		require.Error(t, err)
	}
}
