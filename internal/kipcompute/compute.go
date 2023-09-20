// Copyright © 2021 DoiT International
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package kipcompute

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/doitintl/kubeip/internal/config"
	"github.com/doitintl/kubeip/internal/types"
	"github.com/doitintl/kubeip/internal/utils"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/api/compute/v0.beta" //nolint:goimports
)

const (
	waitTime = 2 * time.Second
)

// ClusterName get GKE cluster name from metadata
func ClusterName() (string, error) {
	return metadata.InstanceAttributeValue("cluster-name") //nolint:wrapcheck
}

// ProjectName get GCP project name from metadata
func ProjectName() (string, error) {
	return metadata.ProjectID() //nolint:wrapcheck
}

func getPriorityOrder(address *compute.Address, cfg *config.Config) int {
	var defaultValue int
	if cfg.OrderByDesc {
		defaultValue = math.MinInt
	} else {
		defaultValue = math.MaxInt
	}

	strVal, ok := address.Labels[cfg.OrderByLabelKey]
	if ok {
		intVal, err := strconv.Atoi(strVal)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"internal": "kubeip",
				"function": "getPriorityOrder",
			}).WithError(err).
				Errorf("Address %s has errors. Failed to convert order by label value %s with value %s to integer", address.Name, cfg.OrderByLabelKey, strVal)
			return defaultValue
		}
		return intVal
	}

	return defaultValue
}

// GetAllAddresses retrieves all addresses matching the query.
func GetAllAddresses(projectID, region string, filterJustReserved bool, cfg *config.Config) (*compute.AddressList, error) {
	return getAllAddresses(projectID, region, cfg.NodePool, filterJustReserved, cfg)
}

func getAllAddresses(projectID, region, pool string, filterJustReserved bool, cfg *config.Config) (*compute.AddressList, error) {
	computeService, err := compute.NewService(context.Background())
	if err != nil {
		return nil, errors.Wrap(err, "failed to get create compute service")
	}
	var filter string
	if cfg.AllNodePools || strings.EqualFold(pool, cfg.NodePool) {
		filter = "(labels." + cfg.LabelKey + "=" + cfg.LabelValue + ")" + " AND  (-labels." + cfg.LabelKey + "-node-pool:*)"
	} else {
		filter = "(labels." + cfg.LabelKey + "=" + cfg.LabelValue + ")" + " AND " + "(labels." + cfg.LabelKey + "-node-pool=" + pool + ")"
	}

	var computedFilter string
	if filterJustReserved {
		computedFilter = "(status=RESERVED) AND " + filter
	} else {
		computedFilter = filter
	}

	var addresses *compute.AddressList
	addresses, err = computeService.Addresses.List(projectID, region).Filter(computedFilter).Do()

	if err != nil {
		return nil, errors.Wrap(err, "failed to list addresses")
	}

	// Right now the SDK does not support filter and order together, so we do it programmatically.
	sort.SliceStable(addresses.Items, func(i, j int) bool {
		address1 := addresses.Items[i]
		address2 := addresses.Items[j]
		val1 := getPriorityOrder(address1, cfg)
		val2 := getPriorityOrder(address2, cfg)
		if cfg.OrderByDesc {
			return val1 > val2
		}
		return val1 < val2
	})

	return addresses, nil
}

func findFreeAddress(projectID, region, pool string, cfg *config.Config) (*types.IPAddress, error) {
	addresses, err := getAllAddresses(projectID, region, pool, true, cfg)
	if err != nil {
		return nil, err
	}

	if len(addresses.Items) != 0 {
		address := addresses.Items[0]
		return &types.IPAddress{IP: address.Address, Labels: address.Labels}, nil
	}
	return nil, errors.New("no free address found")
}

// DeleteIP delete current IP on GKE node
func DeleteIP(projectID, zone, instance string, cfg *config.Config) error {
	computeService, err := compute.NewService(context.Background())
	if err != nil {
		return errors.Wrap(err, "failed to get create compute service")
	}
	inst, err := computeService.Instances.Get(projectID, zone, instance).Do()
	if err != nil {
		return errors.Wrapf(err, "failed to get instance %s zone %s", instance, zone)
	}
	if len(inst.NetworkInterfaces) > 0 && len(inst.NetworkInterfaces[0].AccessConfigs) > 0 {
		accessConfigName := inst.NetworkInterfaces[0].AccessConfigs[0].Name
		if cfg.DryRun {
			logrus.WithFields(logrus.Fields{"internal": "kubeip", "function": "DeleteIP"}).Infof("Deleted Access Config for %s zone %s new ip %s", instance, zone, accessConfigName)
		} else {
			op, err := computeService.Instances.DeleteAccessConfig(projectID, zone, instance, accessConfigName, "nic0").Do()
			if err != nil {
				return errors.Wrap(err, "failed to delete access config")
			}
			err = waitForCompilation(projectID, zone, op)
			if err != nil {
				return errors.Wrap(err, "failed to wait for compilation")
			}
		}
	}
	logrus.WithFields(logrus.Fields{"internal": "kubeip", "function": "DeleteIP"}).Infof("Deleted IP for %s zone %s", instance, zone)
	// Delete an prior tags.
	utils.TagNode(instance, &types.IPAddress{IP: "0.0.0.0", Labels: map[string]string{}}, cfg)
	return nil
}

func addIP(projectID, zone, instance string, addr *types.IPAddress, cfg *config.Config) error {
	computeService, err := compute.NewService(context.Background())
	if err != nil {
		return errors.Wrap(err, "failed to get create compute service")
	}
	if cfg.DryRun {
		logrus.WithFields(logrus.Fields{"internal": "kubeip", "function": "addIP"}).Infof("Added Access Config for %s zone %s new ip %s", instance, zone, addr.IP)
	} else {
		accessConfig := &compute.AccessConfig{
			Name:  "External NAT",
			Type:  "ONE_TO_ONE_NAT",
			NatIP: addr.IP,
			Kind:  "compute#accessConfig",
		}
		var op *compute.Operation
		op, err = computeService.Instances.AddAccessConfig(projectID, zone, instance, "nic0", accessConfig).Do()
		if err != nil {
			return errors.Wrap(err, "failed to add access config")
		}
		err = waitForCompilation(projectID, zone, op)
		if err != nil {
			return errors.Wrap(err, "failed to wait for compilation")
		}
	}

	logrus.WithFields(logrus.Fields{"internal": "kubeip", "function": "addIP"}).Infof("Added IP for %s zone %s new ip %s", instance, zone, addr.IP)
	return nil
}

func replaceIP(projectID, zone, instance, pool string, cfg *config.Config) error {
	region := zone[:len(zone)-2]
	addr, err := findFreeAddress(projectID, region, pool, cfg)
	// Check if we found address.
	if err != nil {
		return errors.Wrap(err, "failed to find free address")
	}

	err = DeleteIP(projectID, zone, instance, cfg)
	if err != nil {
		return errors.Wrap(err, "failed to delete IP")
	}

	err = addIP(projectID, zone, instance, addr, cfg)
	if err != nil {
		return errors.Wrap(err, "failed to add IP")
	}

	logrus.WithFields(logrus.Fields{"internal": "kubeip", "function": "replaceIP"}).Infof("Replaced IP for %s zone %s new ip %s", instance, zone, addr.IP)
	oldNode, err := utils.GetNodeByIP(addr.IP)
	if err == nil {
		utils.TagNode(oldNode, &types.IPAddress{IP: "0.0.0.0", Labels: map[string]string{}}, cfg)
	}
	utils.TagNode(instance, addr, cfg)
	return nil
}

func waitForCompilation(projectID, zone string, operation *compute.Operation) (err error) {
	computeService, err := compute.NewService(context.Background())
	if err != nil {
		return errors.Wrap(err, "failed to get create compute service")
	}
	for {
		var op *compute.Operation
		op, err = computeService.ZoneOperations.Get(projectID, zone, operation.Name).Do()
		if err != nil {
			return errors.Wrapf(err, "failed to get zone operations resource: %s", operation.Name)
		}
		if !strings.EqualFold(op.Status, "done") {
			time.Sleep(waitTime)
		} else {
			return nil
		}
	}
}

// GetAddressUsedByInstance returns the IP used by this instance or the broadcast address otherwise.
func GetAddressUsedByInstance(projectID, instance, zone string, cfg *config.Config) (string, error) {
	computeService, err := compute.NewService(context.Background())
	if err != nil {
		return "", errors.Wrap(err, "failed to get create compute service")
	}

	region := zone[:len(zone)-2]
	filter := "(labels." + cfg.LabelKey + "=" + cfg.LabelValue + ")"
	addresses, err := computeService.Addresses.List(projectID, region).Filter(filter).Do()
	if err != nil {
		return "", errors.Wrap(err, "failed to list addresses")
	}

	for _, addr := range addresses.Items {
		if len(addr.Users) > 0 && strings.Contains(addr.Users[0], instance) {
			return addr.Address, nil
		}
	}

	return "0.0.0.0", nil
}

// IsInstanceUsesReservedIP test if GKE node is using reserved IP
func IsInstanceUsesReservedIP(projectID, instance, zone string, cfg *config.Config) bool {
	computeService, err := compute.NewService(context.Background())
	if err != nil {
		logrus.WithError(err).Error("failed to get create compute service")
		return false
	}
	region := zone[:len(zone)-2]
	filter := "(labels." + cfg.LabelKey + "=" + cfg.LabelValue + ")"
	addresses, err := computeService.Addresses.List(projectID, region).Filter("(status=IN_USE) AND " + filter).Do()
	if err != nil {
		logrus.Error(err)
		return false
	}

	for _, addr := range addresses.Items {
		if strings.Contains(addr.Users[0], instance) {
			return true
		}
	}
	return false
}

// Kubeip replace GKE node IP
func Kubeip(instance <-chan types.Instance, cfg *config.Config) {
	for {
		inst := <-instance
		logrus.WithFields(logrus.Fields{"internal": "kubeip", "function": "Kubeip"}).Infof("Working on %s in zone %s", inst.Name, inst.Zone)
		_ = replaceIP(inst.ProjectID, inst.Zone, inst.Name, inst.Pool, cfg)
	}
}

func getAddressDetails(ip, region, projectID string) (*types.IPAddress, error) {
	computeService, err := compute.NewService(context.Background())
	if err != nil {
		return nil, errors.Wrap(err, "failed to get create compute service")
	}
	filter := "address=" + "\"" + ip + "\""

	addresses, err := computeService.Addresses.List(projectID, region).Filter(filter).Do()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list addresses")
	}

	if len(addresses.Items) != 1 {
		address := addresses.Items[0]
		logrus.WithFields(logrus.Fields{"internal": "kubeip", "function": "getAddressDetails"}).Infof("Node ip is reserved %s %s", ip, fmt.Sprint(address.Labels))
		return &types.IPAddress{IP: address.Address, Labels: address.Labels}, nil
	}

	return nil, errors.New("more than one address found")
}

func isAddressReserved(ip, region, projectID string) bool {
	computeService, err := compute.NewService(context.Background())
	if err != nil {
		logrus.WithError(err).Error("failed to get create compute service")
		return false
	}
	filter := "address=" + "\"" + ip + "\""
	addresses, err := computeService.Addresses.List(projectID, region).Filter(filter).Do()
	if err != nil {
		logrus.Error(err)
		return false
	}

	if len(addresses.Items) != 0 {
		logrus.WithFields(logrus.Fields{"internal": "kubeip", "function": "isAddressReserved"}).Infof("Node ip is reserved %s", ip)
		return true
	}
	return false
}

// AddTagIfMissing add GKE node tag if missing
func AddTagIfMissing(projectID, instance, zone string, cfg *config.Config) {
	computeService, err := compute.NewService(context.Background())
	if err != nil {
		logrus.WithError(err).Error("failed to get create compute service")
		return
	}
	inst, err := computeService.Instances.Get(projectID, zone, instance).Do()
	if err != nil {
		return
	}
	var ip string
	for _, c := range inst.NetworkInterfaces[0].AccessConfigs {
		if c.NatIP != "" {
			ip = c.NatIP
		}
	}
	if isAddressReserved(ip, zone[:len(zone)-2], projectID) {
		addressDetails, err := getAddressDetails(ip, zone, projectID)
		if err != nil {
			return
		}
		logrus.WithFields(logrus.Fields{"internal": "kubeip", "function": "AddTagIfMissing"}).Infof("Tagging %s", instance)
		utils.TagNode(instance, addressDetails, cfg)
	}
}
