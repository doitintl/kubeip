// Copyright © 2018 Aviv Laufer <aviv.laufer@gmail.com>
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
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package kipcompute

import (
	"errors"
	"strings"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/Sirupsen/logrus"
	cfg "github.com/doitintl/kubeip/pkg/config"
	"github.com/doitintl/kubeip/pkg/types"
	"github.com/doitintl/kubeip/pkg/utils"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
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

func findFreeAddress(projectID string, region string, pool string, config *cfg.Config) (string, error) {
	ctx := context.Background()
	hc, err := google.DefaultClient(ctx, container.CloudPlatformScope)
	if err != nil {
		logrus.Error(err)
		return "", err
	}
	computeService, err := compute.New(hc)
	if err != nil {
		logrus.Error(err)
		return "", err
	}
	var filter string
	if config.AllNodePools || strings.EqualFold(pool, config.NodePool) {
		filter = "(labels." + config.LabelKey + "=" + config.LabelValue + ")" + " AND  (-labels." + config.LabelKey + "-node-pool:*)"
	} else {
		filter = "(labels." + config.LabelKey + "=" + config.LabelValue + ")" + " AND " + "(labels." + config.LabelKey + "-node-pool=" + pool + ")"
	}
	addresses, err := computeService.Addresses.List(projectID, region).Filter("(status=RESERVED) AND " + filter).Do()
	if err != nil {
		logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "findFreeAddress"}).Errorf("Failed to list IP addresses in region %s: %q", region, err)
		return "", err
	}

	if len(addresses.Items) != 0 {
		return addresses.Items[0].Address, nil
	}
	return "", errors.New("no free address found")

}

func replaceIP(projectID string, zone string, instance string, pool string, config *cfg.Config) error {
	ctx := context.Background()
	hc, err := google.DefaultClient(ctx, container.CloudPlatformScope)
	if err != nil {
		logrus.Fatalf("Could not get authenticated client: %v", err)
	}
	region := zone[:len(zone)-2]
	addr, err := findFreeAddress(projectID, region, pool, config)
	if err != nil {
		logrus.Infof(err.Error())
		return err
	}
	logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "replaceIP"}).Infof("Found reserved address %s", addr)
	computeService, err := compute.New(hc)
	if err != nil {
		logrus.Infof(err.Error())
		return err
	}
	inst, err := computeService.Instances.Get(projectID, zone, instance).Do()
	if err != nil {
		logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "replaceIP"}).Errorf("Instance not found %s zone %s: %q", instance, zone, err)
		return err
	}
	if len(inst.NetworkInterfaces) > 0 && len(inst.NetworkInterfaces[0].AccessConfigs) > 0 {
		accessConfigName := inst.NetworkInterfaces[0].AccessConfigs[0].Name
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
	accessConfig := &compute.AccessConfig{
		Name:  "External NAT",
		Type:  "ONE_TO_ONE_NAT",
		NatIP: addr,
		Kind:  "kipcompute#accessConfig",
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
	logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "replaceIP"}).Infof("Replaced IP for %s zone %s new ip %s", instance, zone, addr)
	oldNode, err := utils.GetNodeByIP(addr)
	if err == nil {
		utils.TagNode(oldNode, "0.0.0.0")
	}
	utils.TagNode(instance, addr)
	return nil

}

func waitForCompilation(projectID string, zone string, operation *compute.Operation) (err error) {
	ctx := context.Background()
	hc, err := google.DefaultClient(ctx, container.CloudPlatformScope)
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

// IsInstanceUsesReservedIP test if GKE node is using reserved IP
func IsInstanceUsesReservedIP(projectID string, instance string, zone string, config *cfg.Config) bool {
	region := zone[:len(zone)-2]
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

func isAddressReserved(ip string, region string, projectID string) bool {
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
func AddTagIfMissing(projectID string, instance string, zone string) {
	ctx := context.Background()
	hc, err := google.DefaultClient(ctx, container.CloudPlatformScope)
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
	if isAddressReserved(ip, zone[:len(zone)-2], projectID) {
		logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "AddTagIfMissing"}).Infof("Tagging %s", instance)
		utils.TagNode(instance, ip)
	}

}
