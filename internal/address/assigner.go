package address

import (
	"context"
	"errors"

	"github.com/doitintl/kubeip/internal/config"
	"github.com/doitintl/kubeip/internal/types"
	"github.com/sirupsen/logrus"
)

type Assigner interface {
	Assign(ctx context.Context, instanceID, zone string, filter []string, orderBy string) error
	Unassign(ctx context.Context, instanceID, zone string) error
}

func NewAssigner(ctx context.Context, logger *logrus.Entry, provider types.CloudProvider, cfg *config.Config) (Assigner, error) {
	if provider == types.CloudProviderAWS {
		return NewAwsAssigner(ctx, logger, cfg.Region)
	} else if provider == types.CloudProviderAzure {
		return &azureAssigner{}, nil
	} else if provider == types.CloudProviderGCP {
		return NewGCPAssigner(ctx, logger, cfg.Project, cfg.Region)
	}
	return nil, errors.New("unknown cloud provider")
}
