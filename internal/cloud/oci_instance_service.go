package cloud

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/pkg/errors"
)

// OCIInstanceService is the interface for all instance related operations in OCI.
type OCIInstanceService interface {
	ListVnicAttachments(ctx context.Context, compartmentOCID, instanceOCID string) ([]core.VnicAttachment, error)
}

// ociInstanceService is the implementation of OCIInstanceService.
type ociInstanceService struct {
	client core.ComputeClient
}

// NewOCIInstanceService creates a new instance of OCIInstanceService.
func NewOCIInstanceService() (OCIInstanceService, error) {
	client, err := core.NewComputeClientWithConfigurationProvider(common.DefaultConfigProvider())
	if err != nil {
		return nil, errors.Wrap(err, "failed to create OCI Compute client")
	}

	return &ociInstanceService{client: client}, nil
}

// ListVnicAttachments lists all VNIC attachments for the given compartment and instance OCID.
func (svc *ociInstanceService) ListVnicAttachments(ctx context.Context, compartmentOCID, instanceOCID string) ([]core.VnicAttachment, error) {
	request := core.ListVnicAttachmentsRequest{
		CompartmentId: common.String(compartmentOCID),
		InstanceId:    common.String(instanceOCID),
	}
	response, err := svc.client.ListVnicAttachments(ctx, request)
	if err != nil {
		return nil, errors.Wrap(err, "error while listing VNIC attachments")
	}

	if response.Items == nil {
		return nil, errors.Errorf("no VNIC attachments found, compartmentOCID: %s, instanceOCID: %s", compartmentOCID, instanceOCID)
	}

	return response.Items, nil
}
