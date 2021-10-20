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
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/compute/metadata"
	cfg "github.com/doitintl/kubeip/pkg/config"
	"github.com/doitintl/kubeip/pkg/types"
	"github.com/doitintl/kubeip/pkg/utils"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v0.beta"
	"google.golang.org/api/container/v1"
)

// ClusterName get GKE cluster name from metadata
func ClusterName() (string, error) {
	return metadata.InstanceAttributeValue("cluster-name")
}

// ProjectName get GCP project name from metadata
func ProjectName() (string, error) {
	return metadata.ProjectID()
}

func getPriorityOrder(address *compute.Address, config *cfg.Config) int {
	var defaultValue int
	if config.OrderByDesc {
		defaultValue = math.MinInt
	} else {
		defaultValue = math.MaxInt
	}

	strVal, ok := address.Labels[config.OrderByLabelKey]
	if ok {
		intVal, err := strconv.Atoi(strVal)
		if err != nil {
			logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "getPriorityOrder"}).Errorf("Address %s has errors. Failed to convert order by label value %s with value %s to integer", address.Name, config.OrderByLabelKey, strVal, err)
			return defaultValue
		}
		return intVal

	}

	return defaultValue
}

// GetAllAddresses retrieves all addresses matching the query.
func GetAllAddresses(projectID string, region string, filterJustReserved bool, config *cfg.Config) (*compute.AddressList, error) {
	return getAllAddresses(projectID, region, config.NodePool, filterJustReserved, config)
}

func getAllAddresses(projectID string, region string, pool string, filterJustReserved bool, config *cfg.Config) (*compute.AddressList, error) {
	hc, err := google.DefaultClient(context.Background(), container.CloudPlatformScope)
	if err != nil {
		logrus.Error(err)
		return nil, err
	}
	computeService, err := compute.New(hc)
	if err != nil {
		logrus.Error(err)
		return nil, err
	}
	var filter string
	if config.AllNodePools || strings.EqualFold(pool, config.NodePool) {
		filter = "(labels." + config.LabelKey + "=" + config.LabelValue + ")" + " AND  (-labels." + config.LabelKey + "-node-pool:*)"
	} else {
		filter = "(labels." + config.LabelKey + "=" + config.LabelValue + ")" + " AND " + "(labels." + config.LabelKey + "-node-pool=" + pool + ")"
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
		logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "getAllAddresses"}).Errorf("Failed to list IP addresses: %q", err)
		return nil, err
	}

	// Right now the SDK does not support filter and order together, so we do it programmatically.
	sort.SliceStable(addresses.Items, func(i, j int) bool {
		address1 := addresses.Items[i]
		address2 := addresses.Items[j]
		val1 := getPriorityOrder(address1, config)
		val2 := getPriorityOrder(address2, config)
		if config.OrderByDesc {
			return val1 > val2
		}
		return val1 < val2
	})

	return addresses, nil
}

func findFreeAddress(projectID string, region string, pool string, config *cfg.Config) (types.IPAddress, error) {
	addresses, err := getAllAddresses(projectID, region, pool, true, config)
	if err != nil {
		logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "findFreeAddress"}).Errorf("Failed to list IP addresses in region %s: %q", region, err)
		return types.IPAddress{IP: "", Labels: map[string]string{}}, err
	}

	if len(addresses.Items) != 0 {
		address := addresses.Items[0]
		return types.IPAddress{IP: address.Address, Labels: address.Labels}, nil
	}
	return types.IPAddress{IP: "", Labels: map[string]string{}}, errors.New("no free address found")

}

// DeleteIP delete current IP on GKE node
func DeleteIP(projectID string, zone string, instance string, config *cfg.Config) error {
	hc, err := google.DefaultClient(context.Background(), container.CloudPlatformScope)
	if err != nil {
		logrus.Fatalf("Could not get authenticated client: %v", err)
	}

	computeService, err := compute.New(hc)
	if err != nil {
		logrus.Infof(err.Error())
		return err
	}
	inst, err := computeService.Instances.Get(projectID, zone, instance).Do()
	if err != nil {
		logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "DeleteIP"}).Errorf("Instance not found %s zone %s: %q", instance, zone, err)
		return err
	}
	if len(inst.NetworkInterfaces) > 0 && len(inst.NetworkInterfaces[0].AccessConfigs) > 0 {
		accessConfigName := inst.NetworkInterfaces[0].AccessConfigs[0].Name
		if config.DryRun {
			logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "DeleteIP"}).Infof("Deleted Access Config for %s zone %s new ip %s", instance, zone, accessConfigName)
		} else {
			op, err := computeService.Instances.DeleteAccessConfig(projectID, zone, instance, accessConfigName, "nic0").Do()
			if err != nil {
				logrus.Errorf("DeleteAccessConfig %q", err)
				return err
			}
			err = waitForCompilation(projectID, zone, op)
			if err != nil {
				return err
			}
		}

	}
	logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "DeleteIP"}).Infof("Deleted IP for %s zone %s", instance, zone)
	// Delete an prior tags.
	utils.TagNode(instance, types.IPAddress{IP: "0.0.0.0", Labels: map[string]string{}}, config)
	return nil
}

func addIP(projectID string, zone string, instance string, pool string, addr types.IPAddress, config *cfg.Config) error {
	hc, err := google.DefaultClient(context.Background(), container.CloudPlatformScope)
	if err != nil {
		logrus.Fatalf("Could not get authenticated client: %v", err)
	}

	computeService, err := compute.New(hc)
	if err != nil {
		logrus.Infof(err.Error())
		return err
	}

	if config.DryRun {
		logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "addIP"}).Infof("Added Access Config for %s zone %s new ip %s", instance, zone, addr.IP)
	} else {
		accessConfig := &compute.AccessConfig{
			Name:  "External NAT",
			Type:  "ONE_TO_ONE_NAT",
			NatIP: addr.IP,
			Kind:  "compute#accessConfig",
		}
		op, err := computeService.Instances.AddAccessConfig(projectID, zone, instance, "nic0", accessConfig).Do()
		if err != nil {
			logrus.Errorf("AddAccessConfig %q", err)
			return err
		}
		err = waitForCompilation(projectID, zone, op)
		if err != nil {
			return err
		}
	}

	logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "addIP"}).Infof("Added IP for %s zone %s new ip %s", instance, zone, addr.IP)
	return nil
}

func replaceIP(projectID string, zone string, instance string, pool string, config *cfg.Config) error {
	region := zone[:len(zone)-2]
	addr, err := findFreeAddress(projectID, region, pool, config)
	// Check if we found address.
	if err != nil {
		logrus.Infof(err.Error())
		return err
	}

	err = DeleteIP(projectID, zone, instance, config)
	if err != nil {
		logrus.Infof(err.Error())
		return err
	}

	err = addIP(projectID, zone, instance, pool, addr, config)
	if err != nil {
		logrus.Infof(err.Error())
		return err
	}

	logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "replaceIP"}).Infof("Replaced IP for %s zone %s new ip %s", instance, zone, addr.IP)
	oldNode, err := utils.GetNodeByIP(addr.IP)
	if err == nil {
		utils.TagNode(oldNode, types.IPAddress{IP: "0.0.0.0", Labels: map[string]string{}}, config)
	}
	utils.TagNode(instance, addr, config)
	return nil

}

func waitForCompilation(projectID string, zone string, operation *compute.Operation) (err error) {
	hc, err := google.DefaultClient(context.Background(), container.CloudPlatformScope)
	if err != nil {
		logrus.Fatalf("Could not get authenticated client: %v", err)
		return err
	}
	computeService, err := compute.New(hc)
	if err != nil {
		logrus.Fatalf("Could not get create compute service: %v", err)
		return err
	}
	for {
		op, err := computeService.ZoneOperations.Get(projectID, zone, operation.Name).Do()
		if err != nil {
			logrus.Errorf("ZoneOperations.Get %q %s", err, operation.Name)
			return err
		}
		if strings.ToLower(op.Status) != "done" {
			time.Sleep(2 * time.Second)
		} else {
			return nil
		}
	}
}

// GetAddressUsedByInstance returns the IP used by this instance or the broadcast address otherwise.
func GetAddressUsedByInstance(projectID string, instance string, zone string, config *cfg.Config) (string, error) {
	hc, err := google.DefaultClient(context.Background(), container.CloudPlatformScope)
	if err != nil {
		logrus.Fatalf("Could not get authenticated client: %v", err)
		return "", err
	}

	computeService, err := compute.New(hc)
	if err != nil {
		logrus.Fatalf("Could not get create compute service: %v", err)
		return "", err
	}

	region := zone[:len(zone)-2]
	filter := "(labels." + config.LabelKey + "=" + config.LabelValue + ")"
	addresses, err := computeService.Addresses.List(projectID, region).Filter(filter).Do()
	if err != nil {
		logrus.Fatalf("Could not list addresses for instance %s: %v", instance, err)
		return "", err
	}

	for _, addr := range addresses.Items {
		if len(addr.Users) > 0 && strings.Contains(addr.Users[0], instance) {
			return addr.Address, nil
		}
	}

	return "0.0.0.0", nil
}

// IsInstanceUsesReservedIP test if GKE node is using reserved IP
func IsInstanceUsesReservedIP(projectID string, instance string, zone string, config *cfg.Config) bool {
	ctx := context.Background()
	hc, err := google.DefaultClient(ctx, container.CloudPlatformScope)
	if err != nil {
		logrus.Error(err)
		return false
	}
	computeService, err := compute.New(hc)
	if err != nil {
		logrus.Error(err)
		return false
	}
	region := zone[:len(zone)-2]
	filter := "(labels." + config.LabelKey + "=" + config.LabelValue + ")"
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
func Kubeip(instance <-chan types.Instance, config *cfg.Config) {
	for {
		inst := <-instance
		logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "Kubeip"}).Infof("Working on %s in zone %s", inst.Name, inst.Zone)
		_ = replaceIP(inst.ProjectID, inst.Zone, inst.Name, inst.Pool, config)
	}
}

func getAddressDetails(ip string, region string, projectID string, config *cfg.Config) (types.IPAddress, error) {
	hc, err := google.DefaultClient(context.Background(), container.CloudPlatformScope)
	if err != nil {
		logrus.Error(err)
		return types.IPAddress{IP: "", Labels: map[string]string{}}, err
	}
	computeService, err := compute.New(hc)
	if err != nil {
		logrus.Error(err)
		return types.IPAddress{IP: "", Labels: map[string]string{}}, err
	}
	filter := "address=" + "\"" + ip + "\""

	addresses, err := computeService.Addresses.List(projectID, region).Filter(filter).Do()
	if err != nil {
		logrus.Error(err)
		return types.IPAddress{IP: "", Labels: map[string]string{}}, err
	}

	if len(addresses.Items) != 1 {
		address := addresses.Items[0]
		logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "getAddressDetails"}).Infof("Node ip is reserved %s %s", ip, fmt.Sprint(address.Labels))
		return types.IPAddress{IP: address.Address, Labels: address.Labels}, nil
	}

	logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "getAddressDetails"}).Errorf("More than one address found %s", ip)
	return types.IPAddress{IP: "", Labels: map[string]string{}}, fmt.Errorf("more than one address found for ip %s", ip)
}

func isAddressReserved(ip string, region string, projectID string, config *cfg.Config) bool {
	hc, err := google.DefaultClient(context.Background(), container.CloudPlatformScope)
	if err != nil {
		logrus.Error(err)
		return false
	}
	computeService, err := compute.New(hc)
	if err != nil {
		logrus.Error(err)
		return false
	}
	filter := "address=" + "\"" + ip + "\""
	addresses, err := computeService.Addresses.List(projectID, region).Filter(filter).Do()
	if err != nil {
		logrus.Error(err)
		return false
	}

	if len(addresses.Items) != 0 {
		logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "isAddressReserved"}).Infof("Node ip is reserved %s", ip)
		return true
	}
	return false
}

// AddTagIfMissing add GKE node tag if missing
func AddTagIfMissing(projectID string, instance string, zone string, config *cfg.Config) {
	hc, err := google.DefaultClient(context.Background(), container.CloudPlatformScope)
	if err != nil {
		logrus.Fatalf("Could not get authenticated client: %v", err)
		return
	}
	computeService, err := compute.New(hc)
	if err != nil {
		return
	}
	inst, err := computeService.Instances.Get(projectID, zone, instance).Do()
	if err != nil {
		return
	}
	var ip string
	for _, config := range inst.NetworkInterfaces[0].AccessConfigs {
		if config.NatIP != "" {
			ip = config.NatIP
		}
	}
	if isAddressReserved(ip, zone[:len(zone)-2], projectID, config) {
		addressDetails, err := getAddressDetails(ip, zone, projectID, config)
		if err != nil {
			return
		}
		logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "AddTagIfMissing"}).Infof("Tagging %s", instance)
		utils.TagNode(instance, addressDetails, config)
	}

}
