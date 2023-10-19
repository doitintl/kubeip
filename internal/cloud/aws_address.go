package cloud

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/pkg/errors"
)

type EipAssigner interface {
	Assign(ctx context.Context, networkInterfaceID, allocationID string) error
	Unassign(ctx context.Context, associationID string) error
}

type eipAssigner struct {
	client *ec2.Client
}

func NewEipAssigner(client *ec2.Client) EipAssigner {
	return &eipAssigner{client: client}
}

func (a *eipAssigner) Assign(ctx context.Context, networkInterfaceID, allocationID string) error {
	// associate elastic IP with the instance
	input := &ec2.AssociateAddressInput{
		AllocationId:       &allocationID,
		NetworkInterfaceId: &networkInterfaceID,
		AllowReassociation: aws.Bool(false), // do not allow reassociation of the elastic IP
	}

	_, err := a.client.AssociateAddress(ctx, input)
	if err != nil {
		return errors.Wrap(err, "failed to associate elastic IP with the instance")
	}

	return nil
}

func (a *eipAssigner) Unassign(ctx context.Context, associationID string) error {
	// disassociate elastic IP from the instance
	input := &ec2.DisassociateAddressInput{
		AssociationId: &associationID,
	}

	_, err := a.client.DisassociateAddress(ctx, input)
	if err != nil {
		return errors.Wrap(err, "failed to disassociate elastic IP from the instance")
	}

	return nil
}
