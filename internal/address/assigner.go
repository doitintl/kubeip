package address

import (
	"context"
	"errors"

	"github.com/doitintl/kubeip/internal/config"
	"github.com/doitintl/kubeip/internal/types"
	"github.com/sirupsen/logrus"
)

var (
	ErrInvalidIPAMConfiguration = errors.New("AWS IPAM requires AWS IPv4, a Node state store, and empty FILTER and ORDER_BY")
	ErrUnknownCloudProvider     = errors.New("unknown cloud provider")
	ErrStaticIPAlreadyAssigned  = errors.New("static public IP already assigned")
	ErrNoStaticIPAssigned       = errors.New("no static public IP assigned")
)

type Assigner interface {
	Assign(ctx context.Context, instanceID, zone string, filter []string, orderBy string) (string, error)
	Unassign(ctx context.Context, instanceID, zone string) error
}

func NewAssigner(ctx context.Context, logger *logrus.Entry, provider types.CloudProvider, cfg *config.Config, state IPAMState) (Assigner, error) {
	if cfg.AWSIPAMPoolID != "" {
		if provider != types.CloudProviderAWS || cfg.IPv6 || len(cfg.Filter) != 0 || cfg.OrderBy != "" || state == nil {
			return nil, ErrInvalidIPAMConfiguration
		}
	}
	if provider == types.CloudProviderAWS {
		return NewAwsAssigner(ctx, logger, cfg.Region, cfg.AWSIPAMPoolID, state)
	} else if provider == types.CloudProviderAzure {
		return &azureAssigner{}, nil
	} else if provider == types.CloudProviderGCP {
		return NewGCPAssigner(ctx, logger, cfg.Project, cfg.Region, cfg.IPv6)
	} else if provider == types.CloudProviderOCI {
		return NewOCIAssigner(ctx, logger, cfg)
	}
	return nil, ErrUnknownCloudProvider
}
