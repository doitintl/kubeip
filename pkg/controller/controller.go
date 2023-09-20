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

	"github.com/doitintl/kubeip/pkg/config"
	"github.com/doitintl/kubeip/pkg/kipcompute"
	"github.com/doitintl/kubeip/pkg/types"
	"github.com/doitintl/kubeip/pkg/utils"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/time/rate"
	api_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
)

const (
	nodeResource = "node"
	createEvent  = "create"
	deleteEvent  = "delete"
	maxInstances = 100
	rateLimit    = 10
	burstTokens  = 100
	baseDaley    = time.Second
	maxDelay     = 100 * time.Second
)

var (
	errTimeout = errors.New("timeout error")
)

// AddressInstanceTuple object
type AddressInstanceTuple struct {
	address  string
	instance types.Instance
}

// Controller object
type Controller struct {
	logger      logrus.FieldLogger
	clientset   kubernetes.Interface
	queue       workqueue.RateLimitingInterface
	informer    cache.SharedIndexInformer
	instance    chan<- types.Instance
	projectID   string
	clusterName string
	config      *config.Config
	ticker      *time.Ticker
	processing  bool
}

// Event indicate the informerEvent
type Event struct {
	key          string
	eventType    string
	resourceType string
}

var (
	serverStartTime time.Time
	errEmptyPath    = errors.New("empty path")
)

const (
	maxRetries = 5
	prefix     = "kube-system/kube-proxy-"
)

func kubeConfigFromPath(kubepath string) (*rest.Config, error) {
	if kubepath == "" {
		return nil, errEmptyPath
	}

	data, err := os.ReadFile(kubepath)
	if err != nil {
		return nil, errors.Wrapf(err, "reading kubeconfig at %s", kubepath)
	}

	cfg, err := clientcmd.RESTConfigFromKubeConfig(data)
	if err != nil {
		return nil, errors.Wrapf(err, "building rest config from kubeconfig at %s", kubepath)
	}

	return cfg, nil
}

func retrieveKubeConfig(log logrus.FieldLogger, cfg *config.Config) (*rest.Config, error) {
	kubeconfig, err := kubeConfigFromPath(cfg.KubeConfigPath)
	if err != nil && !errors.Is(err, errEmptyPath) {
		return nil, errors.Wrap(err, "retrieving kube config from path")
	}

	if kubeconfig != nil {
		log.Debug("using kube config from env variables")
		return kubeconfig, nil
	}

	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, errors.Wrap(err, "retrieving in cluster kube config")
	}
	log.Debug("using in cluster kube config")
	return inClusterConfig, nil
}

// Start kubeip controller
func Start(ctx context.Context, log logrus.FieldLogger, project, cluster string, cfg *config.Config) error {
	kubeConfig, err := retrieveKubeConfig(log, cfg)
	if err != nil {
		return errors.Wrap(err, "retrieving kube config")
	}
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return errors.Wrap(err, "initializing kubernetes client")
	}
	informer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return kubeClient.CoreV1().Pods(meta_v1.NamespaceAll).List(context.Background(), options) //nolint:wrapcheck
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return kubeClient.CoreV1().Pods(meta_v1.NamespaceAll).Watch(context.Background(), options) //nolint:wrapcheck
			},
		},
		&api_v1.Pod{},
		0, // Skip resync
		cache.Indexers{},
	)

	ctrl, err := newResourceController(log, project, cluster, kubeClient, informer)
	if err != nil {
		return errors.Wrap(err, "creating resource controller")
	}
	ctrl.config = cfg
	ctrl.ticker = time.NewTicker(ctrl.config.Ticker)
	stopCh := make(chan struct{})
	defer close(stopCh)
	// TODO Set size
	instance := make(chan types.Instance, maxInstances)
	ctrl.instance = instance
	go ctrl.Run(stopCh)
	go ctrl.forceAssignment()
	kipcompute.Kubeip(instance, cfg)
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGTERM)
	signal.Notify(sigterm, syscall.SIGINT)
	<-sigterm

	return nil
}

func newResourceController(log logrus.FieldLogger, project, cluster string, client kubernetes.Interface, informer cache.SharedIndexInformer) (*Controller, error) {
	queue := workqueue.NewRateLimitingQueue(workqueue.NewMaxOfRateLimiter(
		workqueue.NewItemExponentialFailureRateLimiter(baseDaley, maxDelay),
		// 10 qps, 100 bucket size.  This is only for retry speed, and it's only the overall factor (not per item)
		&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(rateLimit), burstTokens)},
	))

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(&Event{key, createEvent, nodeResource})
			}
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(&Event{key, deleteEvent, nodeResource})
			}
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "adding event handler")
	}

	return &Controller{
		logger:      log.WithField("pkg", "kubeip-node"),
		projectID:   project,
		clusterName: cluster,
		clientset:   client,
		informer:    informer,
		queue:       queue,
		processing:  false,
	}, nil
}

// Run starts the kubeip controller
func (c *Controller) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	c.logger.Info("Starting kubeip controller")
	serverStartTime = time.Now().Local()

	go c.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, c.HasSynced) {
		utilruntime.HandleError(errTimeout)
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
		return errors.Wrapf(err, "getting object from informer by key %s", newEvent.key)
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
				kubeClient := utils.GetClient()
				node := newEvent.key[len(prefix):]
				var options meta_v1.GetOptions
				options.Kind = "Node"
				options.APIVersion = "1"
				nodeMeta, err := kubeClient.CoreV1().Nodes().Get(context.Background(), node, options)
				if err != nil {
					logrus.Infof(err.Error())
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
					logrus.Infof("Did not found node pool.  These are the labels present %s. ", labels)
					return errors.New("Did not find node pool.  ")
				}
				var inst types.Instance
				if nodeZone, ok := labels["failure-domain.beta.kubernetes.io/zone"]; ok {
					logrus.Infof("Zone pool found %s", nodeZone)
					inst.Zone = nodeZone
				} else {
					logrus.Info("Did not find zone")
					return errors.New("Did not find zone.  ")
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

func isNodeReady(node *api_v1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == api_v1.NodeReady {
			// If the node is unknown we assume that it is ready, we do not want to do IP changes so rapidly.
			return condition.Status == api_v1.ConditionTrue || condition.Status == api_v1.ConditionUnknown
		}
	}
	return false
}

func (c *Controller) processAllNodes(shouldCheckOptimalIPAssignment bool) { //nolint:funlen,gocognit,gocyclo
	kubeClient := utils.GetClient()
	logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).Info("Collecting Node List...")
	nodelist, _ := kubeClient.CoreV1().Nodes().List(context.Background(), meta_v1.ListOptions{})
	nodesOfInterest := make([]types.Instance, 0, len(nodelist.Items))

	for node := range nodelist.Items {
		var inst types.Instance
		labels := nodelist.Items[node].GetLabels()
		if pool, ok := labels["cloud.google.com/gke-nodepool"]; ok {
			if !c.isNodePoolMonitored(pool) {
				continue
			}
			inst.Pool = pool
		} else {
			logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).Info("Did not found node pool")
			continue
		}

		if nodeZone, ok := labels["failure-domain.beta.kubernetes.io/zone"]; ok {
			inst.Zone = nodeZone
		} else {
			logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).Info("Did not find zone")
			continue
		}
		inst.ProjectID = c.projectID
		inst.Name = nodelist.Items[node].GetName()

		// If node is not ready we will basically remove the node IP just in case
		if !isNodeReady(&nodelist.Items[node]) {
			logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).Infof("Node %s in zone %s is not ready, removing IP so we can reuse it. ", inst.Name, inst.Zone)
			// Delete the IP we will re-assign this
			err := kipcompute.DeleteIP(c.projectID, inst.Zone, inst.Name, c.config)
			if err != nil {
				logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).
					Errorf("Could not delete IP used by instance %s in zone %s. Aborting.", inst.Name, inst.Zone)
				return
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
				logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).Errorf("Could not retrieve addresses for project %s region %s. Aborting.", c.projectID, region)
				return
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
					logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).
						Errorf("Could not retrieve address for project %s region %s instance %s. Aborting.", c.projectID, region, instance.Name)
					return
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
					err := kipcompute.DeleteIP(c.projectID, remove.instance.Zone, remove.instance.Name, c.config)
					if err != nil {
						logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).
							Errorf("Could not delete IP %s used by instance %s which is suboptimal. Aborting.", remove.address, remove.instance.Name)
						return
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

func (c *Controller) assignMissingTags() {
	nodePools := c.config.AdditionalNodePools
	nodePools = append(nodePools, c.config.NodePool)

	kubeClient := utils.GetClient()

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
					logrus.Fatalf("Could not get authenticated client: %v", err)
					continue
				}
				logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "assignMissingTags"}).Infof("Found node without tag %s", node.GetName())
				kipcompute.AddTagIfMissing(c.projectID, node.GetName(), nodeZone, c.config)
			} else {
				continue
			}
		}
	}
}
