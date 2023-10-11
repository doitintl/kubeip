package address

import (
	"context"

	"github.com/doitintl/kubeip/internal/config"
	"github.com/doitintl/kubeip/internal/types"
	"github.com/sirupsen/logrus"
)

type Assigner interface {
	Assign(ctx context.Context, instanceID, zone string, filter []string, orderBy string) error
}

type assigner struct {
}

func NewAssigner(ctx context.Context, logger *logrus.Entry, provider types.CloudProvider, cfg *config.Config) (Assigner, error) {
	if provider == types.CloudProviderAWS {
		return NewAwsAssigner(ctx, logger, cfg.Region)
	} else if provider == types.CloudProviderAzure {
		return &azureAssigner{}, nil
	} else if provider == types.CloudProviderGCP {
		return NewGCPAssigner(ctx, logger, cfg.Project, cfg.Region)
	}
	return &assigner{}, nil
}

func (a *assigner) Assign(_ context.Context, _, _ string, _ []string, _ string) error {
	return nil
}
