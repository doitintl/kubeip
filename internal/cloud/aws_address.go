package cloud

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/pkg/errors"
)

type EipAssigner interface {
	Assign(ctx context.Context, region, instanceID string, address *types.Address) error
}

type eipAssigner struct {
	client *ec2.Client
}

func NewEipAssigner(client *ec2.Client) EipAssigner {
	return &eipAssigner{client: client}
}

func (a *eipAssigner) Assign(ctx context.Context, instanceID, networkInterfaceID string, address *types.Address) error {
	// associate elastic IP with the instance
	input := &ec2.AssociateAddressInput{
		AllocationId:       address.AllocationId,
		InstanceId:         &instanceID,
		NetworkInterfaceId: &networkInterfaceID,
	}

	_, err := a.client.AssociateAddress(ctx, input)
	if err != nil {
		return errors.Wrap(err, "failed to associate elastic IP with the instance")
	}

	return nil
}
