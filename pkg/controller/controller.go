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
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package controller

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/time/rate"

	cfg "github.com/doitintl/kubeip/pkg/config"
	"github.com/doitintl/kubeip/pkg/kipcompute"
	"github.com/doitintl/kubeip/pkg/types"
	"github.com/doitintl/kubeip/pkg/utils"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	api_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// AddressInstanceTuple object
type AddressInstanceTuple struct {
	address  string
	instance types.Instance
}

// Controller object
type Controller struct {
	logger      *logrus.Entry
	clientset   kubernetes.Interface
	queue       workqueue.RateLimitingInterface
	informer    cache.SharedIndexInformer
	instance    chan<- types.Instance
	projectID   string
	clusterName string
	config      *cfg.Config
	ticker      *time.Ticker
	processing  bool
}

// Event indicate the informerEvent
type Event struct {
	key          string
	eventType    string
	resourceType string
}

var serverStartTime time.Time

const maxRetries = 5

const prefix = "kube-system/kube-proxy-"

// Start kubeip controller
func Start(config *cfg.Config) error {
	var kubeClient kubernetes.Interface
	_, err := rest.InClusterConfig()
	if err != nil {
		return errors.Wrap(err, "Can not get kubernetes config")
	}
	kubeClient, err = utils.GetClient()
	if err != nil {
		return errors.Wrap(err, "Can not get kubernetes API")
	}
	informer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return kubeClient.CoreV1().Pods(meta_v1.NamespaceAll).List(context.Background(), options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return kubeClient.CoreV1().Pods(meta_v1.NamespaceAll).Watch(context.Background(), options)
			},
		},
		&api_v1.Pod{},
		0, // Skip resync
		cache.Indexers{},
	)

	c := newResourceController(kubeClient, informer, "node")
	c.projectID, err = kipcompute.ProjectName()
	if err != nil {
		return errors.Wrap(err, "Can not get project name")
	}
	c.clusterName, err = kipcompute.ClusterName()
	if err != nil {
		return errors.Wrap(err, "Can not get cluster name")
	}
	c.config = config
	c.ticker = time.NewTicker(c.config.Ticker * time.Minute)
	stopCh := make(chan struct{})
	defer close(stopCh)
	// TODO Set size
	instance := make(chan types.Instance, 100)
	c.instance = instance
	go c.Run(stopCh)
	go c.forceAssignment()
	kipcompute.Kubeip(instance, c.config)
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGTERM)
	signal.Notify(sigterm, syscall.SIGINT)
	<-sigterm

	return nil
}

func newResourceController(client kubernetes.Interface, informer cache.SharedIndexInformer, resourceType string) *Controller {
	queue := workqueue.NewRateLimitingQueue(workqueue.NewMaxOfRateLimiter(
		workqueue.NewItemExponentialFailureRateLimiter(1*time.Second, 100*time.Second),
		// 10 qps, 100 bucket size.  This is only for retry speed, and it's only the overall factor (not per item)
		&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
	))
	var newEvent Event
	var err error
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			newEvent.key, err = cache.MetaNamespaceKeyFunc(obj)
			newEvent.eventType = "create"
			newEvent.resourceType = resourceType
			if err == nil {
				queue.Add(newEvent)
			}
		},
		DeleteFunc: func(obj interface{}) {
			newEvent.key, err = cache.MetaNamespaceKeyFunc(obj)
			newEvent.eventType = "delete"
			newEvent.resourceType = resourceType
			if err == nil {
				queue.Add(newEvent)
			}
		},
	})

	return &Controller{
		logger:     logrus.WithField("pkg", "kubeip-"+resourceType),
		clientset:  client,
		informer:   informer,
		queue:      queue,
		processing: false,
	}
}

// Run starts the kubeip controller
func (c *Controller) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	c.logger.Info("Starting kubeip controller")
	serverStartTime = time.Now().Local()

	go c.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, c.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}

	c.logger.Info("kubeip controller synced and ready")

	wait.Until(c.runWorker, time.Second, stopCh)
}

// HasSynced is required for the cache.Controller interface.
func (c *Controller) HasSynced() bool {
	return c.informer.HasSynced()
}

// LastSyncResourceVersion is required for the cache.Controller interface.
func (c *Controller) LastSyncResourceVersion() string {
	return c.informer.LastSyncResourceVersion()
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
		// continue looping
	}
}

func (c *Controller) processNextItem() bool {
	newEvent, quit := c.queue.Get()

	if quit {
		return false
	}
	defer c.queue.Done(newEvent)
	err := c.processItem(newEvent.(Event))
	if err == nil {
		// No error, reset the ratelimit counters
		c.queue.Forget(newEvent)
	} else if c.queue.NumRequeues(newEvent) < maxRetries {
		c.logger.Errorf("Error processing %s (will retry): %v", newEvent.(Event).key, err)
		c.queue.AddRateLimited(newEvent)
	} else {
		// err != nil and too many retries
		c.logger.Errorf("Error processing %s (giving up): %v", newEvent.(Event).key, err)
		c.queue.Forget(newEvent)
		utilruntime.HandleError(err)
	}

	return true
}

func (c *Controller) isNodePoolMonitored(pool string) bool {
	if c.config.AllNodePools {
		return true
	}
	if strings.EqualFold(pool, c.config.NodePool) {
		return true
	}
	for _, ns := range c.config.AdditionalNodePools {
		if strings.EqualFold(pool, ns) {
			return true
		}
	}
	return false
}
func (c *Controller) processItem(newEvent Event) error {
	obj, _, err := c.informer.GetIndexer().GetByKey(newEvent.key)
	if err != nil {
		return fmt.Errorf("error fetching object with key %s from store: %v", newEvent.key, err)
	}
	// get object's metadata
	objectMeta := utils.GetObjectMetaData(obj)

	// process events based on its type
	switch newEvent.eventType {
	case "delete":
		if strings.HasPrefix(newEvent.key, prefix) {
			node := newEvent.key[len(prefix):]
			logrus.WithFields(logrus.Fields{"pkg": "kubeip-" + newEvent.resourceType, "function": "processItem"}).Infof("Processing removal to %v: %s ", newEvent.resourceType, node)
			// A node has been deleted... we need to check whether the assignment is still optimal
			c.forceAssignmentOnce(true)
			return nil
		}
	case "create":
		// compare CreationTimestamp and serverStartTime and alert only on latest events
		// Could be Replaced by using Delta or DeltaFIFO
		if objectMeta.CreationTimestamp.Sub(serverStartTime).Seconds() > 0 {
			if strings.HasPrefix(newEvent.key, prefix) {
				kubeClient, err := utils.GetClient()
				if err != nil {
					return errors.Wrap(err, "Can not get kubernetes API")
				}
				node := newEvent.key[len(prefix):]
				var options meta_v1.GetOptions
				options.Kind = "Node"
				options.APIVersion = "1"
				nodeMeta, err := kubeClient.CoreV1().Nodes().Get(context.Background(), node, options)
				if err != nil {
					return errors.Wrap(err, "Can not get node")
				}

				labels := nodeMeta.Labels
				var pool string
				var ok bool
				if pool, ok = labels["cloud.google.com/gke-nodepool"]; ok {
					logrus.Infof("Node pool found %s", pool)
					if !c.isNodePoolMonitored(pool) {
						return nil
					}
				} else {
					return errors.New("Did not find node pool.  ")
				}
				var inst types.Instance
				if nodeZone, ok := labels["failure-domain.beta.kubernetes.io/zone"]; ok {
					logrus.Infof("Zone pool found %s", nodeZone)
					inst.Zone = nodeZone
				} else {
					return errors.New("did not find zone")
				}
				logrus.WithFields(logrus.Fields{"pkg": "kubeip-" + newEvent.resourceType, "function": "processItem"}).Infof("Processing add to %v: %s ", newEvent.resourceType, node)
				inst.Name = node
				inst.ProjectID = c.projectID
				inst.Pool = pool
				c.instance <- inst
				logrus.WithFields(logrus.Fields{"pkg": "kubeip-" + newEvent.resourceType, "function": "processItem"}).Infof("Processing node %s of cluster %s in zone %s", node, c.clusterName, inst.Zone)
				return nil
			}
		}

	}
	return nil
}

func isNodeReady(node api_v1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == api_v1.NodeReady {
			// If the node is unknown we assume that it is ready, we do not want to do IP changes so rapidly.
			return condition.Status == api_v1.ConditionTrue || condition.Status == api_v1.ConditionUnknown
		}
	}
	return false
}

func (c *Controller) processAllNodes(shouldCheckOptimalIPAssignment bool) error {
	kubeClient, err := utils.GetClient()
	if err != nil {
		return errors.Wrap(err, "Can not get kubernetes API")
	}
	logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).Info("Collecting Node List...")
	nodelist, _ := kubeClient.CoreV1().Nodes().List(context.Background(), meta_v1.ListOptions{})
	var pool string
	var ok bool
	var nodesOfInterest []types.Instance

	for _, node := range nodelist.Items {
		labels := node.GetLabels()
		if pool, ok = labels["cloud.google.com/gke-nodepool"]; ok {
			if !c.isNodePoolMonitored(pool) {
				continue
			}
		} else {
			logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).Info("Did not found node pool")
			continue
		}
		var inst types.Instance
		if nodeZone, ok := labels["failure-domain.beta.kubernetes.io/zone"]; ok {
			inst.Zone = nodeZone
		} else {
			logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).Info("Did not find zone")
			continue
		}

		inst.ProjectID = c.projectID
		inst.Name = node.GetName()
		inst.Pool = pool

		// If node is not ready we will basically remove the node IP just in case
		if !isNodeReady(node) {
			logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).Infof("Node %s in zone %s is not ready, removing IP so we can reuse it. ", inst.Name, inst.Zone)
			// Delete the IP we will re-assign this
			err = kipcompute.DeleteIP(c.projectID, inst.Zone, inst.Name, c.config)
			if err != nil {
				return errors.Wrap(err, "Can not delete IP")
			}
			continue
		}

		nodesOfInterest = append(nodesOfInterest, inst)
	}

	// Should we check that the IPs assigned to the current nodes are in fact the best possible IPs to assign?
	if shouldCheckOptimalIPAssignment {
		// Determining the required IP per region
		regionsCount := make(map[string][]types.Instance)
		logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).Infof("Collected %d Nodes of interest...calculating number of IPs required", len(nodesOfInterest))
		for _, inst := range nodesOfInterest {
			zone := inst.Zone
			region := zone[:len(zone)-2]
			regionsCount[region] = append(regionsCount[region], inst)
		}

		// Determining the most optimal nodes per region.
		for region, instances := range regionsCount {
			logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).Infof("Collected %d Nodes of interest...processing %d nodes instances within region %s", len(nodesOfInterest), len(instances), region)

			addresses, err := kipcompute.GetAllAddresses(c.projectID, region, false, c.config)
			if err != nil {
				return errors.Wrap(err, "Can not get all addresses")
			}

			var topMostAddresses []string
			for _, address := range addresses.Items[:utils.Min(len(instances), len(addresses.Items))] {
				topMostAddresses = append(topMostAddresses, address.Address)
			}

			// Retrieve all addresses in the region.
			var usedAddresses []AddressInstanceTuple
			logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).Infof("Retrieving addresses used in project %s in region %s", c.projectID, region)
			for _, instance := range instances {
				address, err := kipcompute.GetAddressUsedByInstance(c.projectID, instance.Name, instance.Zone, c.config)
				if err != nil {
					return errors.Wrap(err, "Can not get address used by instance")
				}
				usedAddresses = append(usedAddresses, AddressInstanceTuple{
					address,
					instance,
				})
			}

			// Perform subtraction
			logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).Infof("Project %s in region %s should use the following IPs %s... Checking that the instances follow these assignments", c.projectID, region, topMostAddresses)
			var toRemove []AddressInstanceTuple
			for _, usedAddress := range usedAddresses {
				if usedAddress.address != "0.0.0.0" && !utils.Contains(topMostAddresses, usedAddress.address) {
					toRemove = append(toRemove, usedAddress)
				}
			}

			logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).Infof("Found %d Addresses to remove project %s in region %s.  Addresses %s", len(toRemove), c.projectID, region, toRemove)
			if len(toRemove) > 0 {
				logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).Infof("Found %d ips %s in region %s which are not part of the top most addresses %s", len(toRemove), toRemove, region, topMostAddresses)
				for _, remove := range toRemove {

					logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).Infof("Instance %s in project %s in region %s uses suboptimal IP %s... Removing so we reassign", remove.instance.Name, c.projectID, region, toRemove)
					// Delete the IP we will re-assign this
					err = kipcompute.DeleteIP(c.projectID, remove.instance.Zone, remove.instance.Name, c.config)
					if err != nil {
						return errors.Wrap(err, "Can not delete IP")
					}
				}
			}
		}
	}

	for _, inst := range nodesOfInterest {
		if !kipcompute.IsInstanceUsesReservedIP(c.projectID, inst.Name, inst.Zone, c.config) {
			logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).Infof("Found unassigned node %s in pool %s", inst.Name, inst.Pool)
			c.instance <- inst
		}
	}
	return nil
}

func (c *Controller) forceAssignmentOnce(shouldCheckOptimalIPAssignment bool) {
	if !c.processing {
		c.processing = true
		if c.config.ForceAssignment {
			logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "forceAssignmentOnce"}).Info("Starting forceAssignmentOnce")
			c.processAllNodes(shouldCheckOptimalIPAssignment)
		}
		c.assignMissingTags()
		c.processing = false
	} else {
		logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "forceAssignmentOnce"}).Info("Skipping forceAssignmentOnce ... already in progress")
	}
}

func (c *Controller) forceAssignment() {
	logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "forceAssignment"}).Info("Processing initial force assignment check")
	c.forceAssignmentOnce(true)
	for range c.ticker.C {
		logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "forceAssignment"}).Info("Tick received for force assignment check")
		c.forceAssignmentOnce(false)
	}
}

func (c *Controller) assignMissingTags() error {
	nodePools := append(c.config.AdditionalNodePools, c.config.NodePool)

	kubeClient, err := utils.GetClient()
	if err != nil {
		return errors.Wrap(err, "Can not get kubernetes API")
	}

	for _, pool := range nodePools {
		label := fmt.Sprintf("!kubip_assigned,cloud.google.com/gke-nodepool=%s", pool)
		nodelist, err := kubeClient.CoreV1().Nodes().List(context.Background(), meta_v1.ListOptions{
			LabelSelector: label,
		})
		if err != nil {
			logrus.Error(err)
			continue

		}
		for _, node := range nodelist.Items {
			labels := node.GetLabels()
			if nodeZone, ok := labels["failure-domain.beta.kubernetes.io/zone"]; ok {
				if err != nil {
					logrus.WithError(err).Error("Could not get node zone")
					continue
				}
				logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "assignMissingTags"}).Infof("Found node without tag %s", node.GetName())
				kipcompute.AddTagIfMissing(c.projectID, node.GetName(), nodeZone, c.config)

			} else {
				continue
			}
		}
	}
	return nil
}
