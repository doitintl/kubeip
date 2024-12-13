package address

import (
	"context"
	"strings"

	"github.com/doitintl/kubeip/internal/cloud"
	"github.com/doitintl/kubeip/internal/config"
	"github.com/doitintl/kubeip/internal/types"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ociAssigner is an Assigner implementation for Oracle Cloud Infrastructure.
type ociAssigner struct {
	logger          *logrus.Entry
	filters         *types.OCIFilters
	compartmentOCID string
	instanceSvc     cloud.OCIInstanceService
	networkSvc      cloud.OCINetworkService
}

// NewOCIAssigner creates a new Assigner for Oracle Cloud Infrastructure.
func NewOCIAssigner(_ context.Context, logger *logrus.Entry, cfg *config.Config) (Assigner, error) {
	logger.WithFields(
		logrus.Fields{
			"compartmentOCID": cfg.Project,
			"filters":         cfg.Filter,
		},
	).Info("creating new OCI assigner with given config")

	// Parse the filters
	filters, err := parseOCIFilters(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse OCI filters")
	}
	if filters == nil {
		logger.Warn("no filters provided, any ip from the list of all public IPs present in the project can be used")
	}

	// Create a new instance svc
	computeSvc, err := cloud.NewOCIInstanceService()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create compute service for OCI")
	}

	// Create a new network svc
	networkSvc, err := cloud.NewOCINetworkService()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create network service for OCI")
	}

	return &ociAssigner{
		logger:          logger,
		filters:         filters,
		instanceSvc:     computeSvc,
		networkSvc:      networkSvc,
		compartmentOCID: cfg.Project,
	}, nil
}

// Assign assigns reserved Public IP to the instance.
// If the instance already has a public IP assigned, and it is from the reserved list, it returns the same IP.
// Else it assigns a new public IP from the reserved list.
func (a *ociAssigner) Assign(ctx context.Context, instanceOCID, _ string, _ []string, _ string) (string, error) {
	a.logger.WithField("instanceOCID", instanceOCID).Debug("starting process to assign reserved public IP to instance")

	// Get the primary VNIC
	vnic, err := a.getPrimaryVnicOfInstance(ctx, instanceOCID)
	if err != nil {
		return "", err
	}
	a.logger.WithField("primaryVnicOCID", *vnic.Id).Debugf("got primary VNIC of the instance %s", instanceOCID)

	// Handle already assigned public IP case
	alreadyAssigned, err := a.handlePublicIPAlreadyAssignedCase(ctx, vnic)
	if err != nil {
		return "", errors.Wrap(err, "failed to check if public ip is already assigned or not")
	}
	if alreadyAssigned {
		a.logger.WithField("alreadyAssignedIP", *vnic.PublicIp).Infof("reserved public IP already assigned on instance %s", instanceOCID)
		return *vnic.PublicIp, ErrStaticIPAlreadyAssigned
	}

	// Get primary VNIC private IP
	privateIP, err := a.networkSvc.GetPrimaryPrivateIPOfVnic(ctx, *vnic.Id)
	if err != nil {
		return "", errors.Wrap(err, "failed to get primary VNIC private IP")
	}
	a.logger.WithField("privateIPOCID", *privateIP.Id).Debugf("got primary VNIC private IP of the instance %s", instanceOCID)

	// Fetch all available reserved Public IPs that will be used for assignment
	reservedPublicIPList, err := a.fetchPublicIps(ctx, true, false)
	if err != nil {
		return "", errors.Wrap(err, "failed to get list of reserved public IPs")
	}
	if len(reservedPublicIPList) == 0 {
		return "", errors.New("no reserved public IPs available")
	}
	a.logger.WithField("reservedPublicIpList", reservedPublicIPList).Debug("got list of available reserved public IPs")

	// Try to assign an IP from the reserved public IP list
	for _, publicIP := range reservedPublicIPList {
		if err = a.tryAssignAddress(ctx, *privateIP.Id, *publicIP.Id); err == nil {
			a.logger.WithField("assignedIP", *publicIP.IpAddress).Infof("assigned IP %s to instance %s", *publicIP.IpAddress, instanceOCID)
			return *publicIP.IpAddress, nil
		}
		a.logger.Warnf("Failed to assign IP %s to instance %s: %v", *publicIP.IpAddress, instanceOCID, err)
	}

	return "", errors.New("failed to assign any IP")
}

// Unassign unassigns the public IP from the instance.
// If assigned public IP is from the reserved public IP list, it unassigns the public IP.
// Else it does nothing.
func (a *ociAssigner) Unassign(ctx context.Context, instanceOCID, _ string) error {
	a.logger.WithField("instanceOCID", instanceOCID).Debug("starting process to unassign public IP from the instance")

	// Get the primary VNIC
	vnic, err := a.getPrimaryVnicOfInstance(ctx, instanceOCID)
	if err != nil {
		return err
	}

	// If no public IP is assigned, return
	if vnic.PublicIp == nil {
		a.logger.Infof("no public ip assigned to the instance %s", instanceOCID)
		return ErrNoPublicIPAssigned
	}
	publicIP := vnic.PublicIp

	// Fetch assigned public IPs
	reservedPublicIPList, err := a.fetchPublicIps(ctx, true, true)
	if err != nil {
		return errors.Wrap(err, "failed to get list of reserved public IPs")
	}
	a.logger.WithField("reservedPublicIPList", reservedPublicIPList).Debug("got list of reserved public IPs")

	// Check if assigned public ip is from the reserved public IP list
	for _, ip := range reservedPublicIPList {
		if *ip.IpAddress == *publicIP && ip.LifecycleState == core.PublicIpLifecycleStateAssigned {
			// Unassign the public IP
			if err := a.networkSvc.UpdatePublicIP(ctx, *ip.Id, ""); err != nil {
				return errors.Wrap(err, "failed to unassign public IP assigned to private IP")
			}
			return nil
		}
	}

	return errors.New("public IP not assigned from reserved list")
}

// getPrimaryVnicOfInstance returns the primary VNIC of the instance from the VNIC attachment.
func (a *ociAssigner) getPrimaryVnicOfInstance(ctx context.Context, instanceOCID string) (*core.Vnic, error) {
	// Get VNIC attachment of the instance
	vnicAttachment, err := a.instanceSvc.ListVnicAttachments(ctx, a.compartmentOCID, instanceOCID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list VNIC attachments")
	}

	// Get the primary VNIC
	vnic, err := a.networkSvc.GetPrimaryVnic(ctx, vnicAttachment)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get primary VNIC")
	}

	return vnic, nil
}

// handlePublicIPAlreadyAssignedCase handles the case when the public IP is already assigned to the instance.
// It returns true if the public IP is already assigned to the instance from the reserved IP list. In this case, do nothing.
// It returns false in all other cases with error(if any). In this case, if err is nil, try to assign a new public IP.
// Following are the cases and actions for each case:
//   - Case1: Public IP is already assigned to the instance from the reserved IP list: Do nothing
//   - Case2: Public IP is assigned to the instance but not from the reserved IP list: Unassign the public IP
//   - Case3: Public IP is assigned to the instance, but it is ephemeral: Delete the ephemeral public IP
//   - Case4: Unhandled case: Return error
//
//nolint:gocognit
func (a *ociAssigner) handlePublicIPAlreadyAssignedCase(ctx context.Context, vnic *core.Vnic) (bool, error) {
	if vnic == nil {
		return false, nil
	}
	publicIP := vnic.PublicIp
	if publicIP != nil {
		// Case1
		// Fetch all reserved public IPs that are assigned to the private IPs
		list, err := a.fetchPublicIps(ctx, true, true)
		if err != nil {
			return false, errors.Wrap(err, "failed to list reserved public IPs assigned to private IP")
		}
		for _, ip := range list {
			if *ip.IpAddress == *publicIP {
				return true, nil
			}
		}

		// Case2
		// Fetch all public IPs that are assigned to the private IPs
		list, err = a.fetchPublicIps(ctx, false, true)
		if err != nil {
			return false, errors.Wrap(err, "failed to list public IPs assigned to private IP")
		}
		for _, ip := range list {
			if *ip.IpAddress == *publicIP {
				// Unassign the public IP
				if err = a.networkSvc.UpdatePublicIP(ctx, *ip.Id, ""); err != nil {
					return false, errors.Wrap(err, "failed to unassign public IP assigned to private IP")
				}
				return false, nil
			}
		}

		// Case3
		// Fetch ephemeral public IPs assigned to private IPs
		if vnic.AvailabilityDomain == nil {
			return false, errors.New("availability domain not found")
		}
		list, err = a.fetchEphemeralPublicIPs(ctx, *vnic.AvailabilityDomain)
		if err != nil {
			return false, errors.Wrap(err, "failed to list ephemeral public IPs assigned to private IP")
		}
		for _, ip := range list {
			if *ip.IpAddress == *publicIP {
				// Delete the ephemeral public IP
				if err := a.networkSvc.DeletePublicIP(ctx, *ip.Id); err != nil {
					return false, errors.Wrap(err, "failed to delete ephemeral public IP assigned to private IP")
				}
				return false, nil
			}
		}

		// Case4
		// Unhandled case
		return false, errors.New("unhandled case: public IP is assigned to the instance but not from the reserved IP list")
	}

	return false, nil
}

// fetchPublicIps returns the list of public IPs.
// If useFilter is set to true, it applies the filters.
// It returns only available public IPs if inUse is set to false.
// It returns only assigned public IPs if inUse is set to true.
func (a *ociAssigner) fetchPublicIps(ctx context.Context, useFilter, inUse bool) ([]core.PublicIp, error) {
	filters := a.filters
	// If useFilter is set to false, do not apply the filters
	if !useFilter {
		filters = nil
	}
	list, err := a.networkSvc.ListPublicIps(ctx, &core.ListPublicIpsRequest{
		Scope:         core.ListPublicIpsScopeRegion,
		CompartmentId: common.String(a.compartmentOCID),
	}, filters)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list public IPs")
	}

	lifecycleState := core.PublicIpLifecycleStateAvailable
	// If inUse is set to true, only return assigned public IPs
	if inUse {
		lifecycleState = core.PublicIpLifecycleStateAssigned
	}

	// Return IPs that match the given lifecycleState.
	var updatedList []core.PublicIp
	for _, ip := range list {
		if ip.LifecycleState == lifecycleState {
			updatedList = append(updatedList, ip)
		}
	}
	return updatedList, nil
}

// fetchEphemeralPublicIPs returns the list of ephemeral public IPs assigned to the private IPs in the availability domain.
func (a *ociAssigner) fetchEphemeralPublicIPs(ctx context.Context, availabilityDomain string) ([]core.PublicIp, error) {
	list, err := a.networkSvc.ListPublicIps(ctx, &core.ListPublicIpsRequest{
		Scope:              core.ListPublicIpsScopeAvailabilityDomain,
		AvailabilityDomain: common.String(availabilityDomain),
		CompartmentId:      common.String(a.compartmentOCID),
	}, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list ephemeral public IPs")
	}

	return list, nil
}

// tryAssignAddress tries to assign the public IP to the private IP.
// If the public IP is not available, it returns an error.
func (a *ociAssigner) tryAssignAddress(ctx context.Context, privateIPOCID, publicIPOCID string) error {
	// Fetch public IP details to check if it is available
	publicIP, err := a.networkSvc.GetPublicIP(ctx, publicIPOCID)
	if err != nil {
		return errors.Wrap(err, "failed to get public IP details")
	}
	if publicIP == nil {
		return errors.New("public IP not found")
	}

	// If public IP is not available, return
	if publicIP.LifecycleState != core.PublicIpLifecycleStateAvailable {
		return errors.New("public IP is not available")
	}

	// Assign the public IP to the private IP
	if err := a.networkSvc.UpdatePublicIP(ctx, *publicIP.Id, privateIPOCID); err != nil {
		return errors.Wrap(err, "failed to assign public IP")
	}

	return nil
}

// ParseOCIFilters parses the filters for OCI from the config.
// All filters of freeformTags are combined with AND condition.
// All filters of definedTags are combined with AND condition.
// Filter should be in following format:
//   - "freeformTags.key1=value1"
//   - "definedTags.Namespace.key1=value1"
func parseOCIFilters(cfg *config.Config) (*types.OCIFilters, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}

	freeformTags := make(map[string]string)

	for _, filter := range cfg.Filter {
		if strings.HasPrefix(filter, "freeformTags.") {
			key, value, err := types.ParseFreeformTagFilter(filter)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse freeform tag filter")
			}
			freeformTags[key] = value
		} else {
			return nil, errors.New("invalid filter format for OCI, should be in format freeformTags.key=value or definedTags.Namespace.key=value, found: " + filter)
		}
	}

	return &types.OCIFilters{
		FreeformTags: freeformTags,
	}, nil
}
