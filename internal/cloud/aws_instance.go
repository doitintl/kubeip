package cloud

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/pkg/errors"
)

type Ec2InstanceGetter interface {
	Get(ctx context.Context, instanceID, region string) (*types.Instance, error)
}

type ec2InstanceGetter struct {
	client *ec2.Client
}

func NewEc2InstanceGetter(client *ec2.Client) Ec2InstanceGetter {
	return &ec2InstanceGetter{client: client}
}

func (g *ec2InstanceGetter) Get(ctx context.Context, instanceID, _ string) (*types.Instance, error) {
	input := &ec2.DescribeInstancesInput{
		InstanceIds: []string{
			instanceID,
		},
	}

	resp, err := g.client.DescribeInstances(ctx, input)
	if err != nil {
		return nil, errors.Wrap(err, "failed to describe instances, %v")
	}

	if len(resp.Reservations) == 0 || len(resp.Reservations[0].Instances) == 0 {
		return nil, errors.Wrap(err, "no instances found for the given id")
	}

	return &resp.Reservations[0].Instances[0], nil
}
