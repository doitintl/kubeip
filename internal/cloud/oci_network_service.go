package cloud

import (
	"context"

	"github.com/doitintl/kubeip/internal/types"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/pkg/errors"
)

// OCINetworkService is the interface for all network related operations in OCI (Virtual Network).
type OCINetworkService interface {
	ListPublicIps(ctx context.Context, request *core.ListPublicIpsRequest, filters *types.OCIFilters) ([]core.PublicIp, error)
	GetPublicIP(ctx context.Context, publicIPOCID string) (*core.PublicIp, error)
	UpdatePublicIP(ctx context.Context, publicIPOCID, privateIPOCID string) error
	DeletePublicIP(ctx context.Context, publicIPOCID string) error
	GetPrimaryPrivateIPOfVnic(ctx context.Context, vnicOCID string) (*core.PrivateIp, error)
	GetPrimaryVnic(ctx context.Context, vnicAttachments []core.VnicAttachment) (*core.Vnic, error)
}

// ociNetworkService is the implementation of OCINetworkService.
type ociNetworkService struct {
	client core.VirtualNetworkClient
}

// NewOCINetworkService creates a new instance of OCINetworkService.
func NewOCINetworkService() (OCINetworkService, error) {
	client, err := core.NewVirtualNetworkClientWithConfigurationProvider(common.DefaultConfigProvider())
	if err != nil {
		return nil, errors.Wrap(err, "failed to create OCI Virtual Network client")
	}

	return &ociNetworkService{client: client}, nil
}

// ListPublicIps lists all public IPs for the given request and applies the given filters.
func (svc *ociNetworkService) ListPublicIps(ctx context.Context, request *core.ListPublicIpsRequest, filters *types.OCIFilters) ([]core.PublicIp, error) {
	response, err := svc.client.ListPublicIps(ctx, *request)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list public IPs")
	}

	if response.Items == nil {
		return nil, errors.New("no public IPs found")
	}

	// Apply filters
	if filters != nil {
		list := []core.PublicIp{}
		for _, ip := range response.Items {
			if filters.CheckFreeformTagFilter(ip.FreeformTags) && filters.CheckDefinedTagFilter(ip.DefinedTags) {
				list = append(list, ip)
			}
		}
		return list, nil
	}

	return response.Items, nil
}

// GetPublicIP returns the public IP with the given OCID.
func (svc *ociNetworkService) GetPublicIP(ctx context.Context, publicIPOCID string) (*core.PublicIp, error) {
	request := core.GetPublicIpRequest{
		PublicIpId: common.String(publicIPOCID),
	}
	response, err := svc.client.GetPublicIp(ctx, request)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get details of public IP OCID: %s"+publicIPOCID)
	}

	if response.PublicIp.Id == nil {
		return nil, errors.Errorf("no public IP found with OCID %s", publicIPOCID)
	}

	return &response.PublicIp, nil
}

// UpdatePublicIP updates the public IP with the given OCID.
func (svc *ociNetworkService) UpdatePublicIP(ctx context.Context, publicIPOCID, privateIPOCID string) error {
	request := core.UpdatePublicIpRequest{
		PublicIpId: common.String(publicIPOCID),
		UpdatePublicIpDetails: core.UpdatePublicIpDetails{
			PrivateIpId: common.String(privateIPOCID),
		},
	}
	if _, err := svc.client.UpdatePublicIp(ctx, request); err != nil {
		return errors.Wrap(err, "failed to update public IP")
	}

	return nil
}

// DeletePublicIP deletes the public IP with the given OCID.
func (svc *ociNetworkService) DeletePublicIP(ctx context.Context, publicIPOCID string) error {
	request := core.DeletePublicIpRequest{
		PublicIpId: common.String(publicIPOCID),
	}
	if _, err := svc.client.DeletePublicIp(ctx, request); err != nil {
		return errors.Wrap(err, "failed to delete public IP")
	}

	return nil
}

// GetPrimaryPrivateIPOfVnic returns the primary private IP of the given VNIC.
func (svc *ociNetworkService) GetPrimaryPrivateIPOfVnic(ctx context.Context, vnicOCID string) (*core.PrivateIp, error) {
	request := core.ListPrivateIpsRequest{
		VnicId: common.String(vnicOCID),
	}
	response, err := svc.client.ListPrivateIps(ctx, request)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list private IPs for the VNIC %s"+vnicOCID)
	}

	if response.Items == nil {
		return nil, errors.New("no private IPs found for the VNIC %s" + vnicOCID)
	}

	// Loop through the private IPs and return the primary one
	for _, privateIP := range response.Items {
		if *privateIP.IsPrimary {
			return &privateIP, nil
		}
	}

	return nil, errors.New("no primary private IP found for the VNIC %s" + vnicOCID)
}

// GetPrimaryVnic returns the primary VNIC from the given VNIC attachments.
func (svc *ociNetworkService) GetPrimaryVnic(ctx context.Context, vnicAttachments []core.VnicAttachment) (*core.Vnic, error) {
	for _, vnicAttachment := range vnicAttachments {
		vnic, err := svc.client.GetVnic(ctx, core.GetVnicRequest{VnicId: vnicAttachment.VnicId})
		if err != nil {
			return nil, errors.Wrap(err, "failed to get VNIC details with OCID %s"+*vnicAttachment.VnicId)
		}

		if vnic.IsPrimary != nil && *vnic.IsPrimary {
			return &vnic.Vnic, nil
		}
	}

	return nil, errors.New("no primary VNIC found from the given VNIC attachments")
}
