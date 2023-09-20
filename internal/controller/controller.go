package controller

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/doitintl/kubeip/internal/config"
	"github.com/doitintl/kubeip/internal/kipcompute"
	"github.com/doitintl/kubeip/internal/types"
	"github.com/doitintl/kubeip/internal/utils"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"golang.org/x/time/rate"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	nodeResource        = "node"
	createEvent         = "create"
	deleteEvent         = "delete"
	maxInstances        = 100
	rateLimit           = 10
	burstTokens         = 100
	baseDaley           = time.Second
	maxDelay            = 100 * time.Second
	nodeCacheSyncPeriod = 5 * time.Minute
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
	informer    cache.SharedInformer
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
	// Create a new Node informer
	nodeInformer := cache.NewSharedInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (object runtime.Object, err error) {
				return kubeClient.CoreV1().Nodes().List(context.Background(), options) //nolint:wrapcheck
			},
			WatchFunc: func(options metav1.ListOptions) (retWc watch.Interface, err error) {
				return kubeClient.CoreV1().Nodes().Watch(context.Background(), options) //nolint:wrapcheck
			},
		},
		&corev1.Node{},
		nodeCacheSyncPeriod,
	)

	ctrl, err := newResourceController(log, project, cluster, kubeClient, nodeInformer)
	if err != nil {
		return errors.Wrap(err, "creating resource controller")
	}
	ctrl.config = cfg
	ctrl.ticker = time.NewTicker(ctrl.config.Ticker)
	stopCh := make(chan struct{})
	defer close(stopCh)

	instance := make(chan types.Instance, maxInstances)
	ctrl.instance = instance
	go ctrl.Run(stopCh)
	go ctrl.forceAssignment()

	kipcompute.Kubeip(instance, cfg)

	// wait till context is canceled
	<-ctx.Done()
	return nil
}

func newResourceController(log logrus.FieldLogger, project, cluster string, client kubernetes.Interface, informer cache.SharedInformer) (*Controller, error) {
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
		logger:      log.WithField("internal", "kubeip-node"),
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
		c.logger.WithError(err).Errorf("error processing %s (will retry)", newEvent.(Event).key)
		c.queue.AddRateLimited(newEvent)
	} else {
		// err != nil and too many retries
		c.logger.WithError(err).Errorf("error processing %s (giving up)", newEvent.(Event).key)
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
	obj, _, err := c.informer.GetStore().GetByKey(newEvent.key)
	if err != nil {
		return errors.Wrapf(err, "getting object from informer by key %s", newEvent.key)
	}
	// get object's metadata
	objectMeta := utils.GetObjectMetaData(obj)

	// process events based on its type
	switch newEvent.eventType {
	case "delete":
		if strings.HasPrefix(newEvent.key, prefix) {
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
				var options metav1.GetOptions
				options.Kind = "Node"
				options.APIVersion = "1"
				var nodeMeta *corev1.Node
				nodeMeta, err = kubeClient.CoreV1().Nodes().Get(context.Background(), node, options)
				if err != nil {
					return errors.Wrap(err, "failed to get node")
				}

				labels := nodeMeta.Labels
				var pool string
				var ok bool
				if pool, ok = labels["cloud.google.com/gke-nodepool"]; ok {
					if !c.isNodePoolMonitored(pool) {
						return nil
					}
				} else {
					return errors.New("failed to find node pool")
				}
				var inst types.Instance
				if nodeZone, ok := labels["failure-domain.beta.kubernetes.io/zone"]; ok {
					inst.Zone = nodeZone
				} else {
					return errors.New("failed to find zone")
				}
				inst.Name = node
				inst.ProjectID = c.projectID
				inst.Pool = pool
				c.instance <- inst
				return nil
			}
		}
	}
	return nil
}

func isNodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			// If the node is unknown we assume that it is ready, we do not want to do IP changes so rapidly.
			return condition.Status == corev1.ConditionTrue || condition.Status == corev1.ConditionUnknown
		}
	}
	return false
}

func (c *Controller) processAllNodes(shouldCheckOptimalIPAssignment bool) error { //nolint:funlen,gocognit,gocyclo
	kubeClient := utils.GetClient()
	nodelist, err := kubeClient.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to list nodes")
	}
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
			c.logger.Warn("failed to find node pool")
			continue
		}

		if nodeZone, ok := labels["failure-domain.beta.kubernetes.io/zone"]; ok {
			inst.Zone = nodeZone
		} else {
			c.logger.Warn("failed to find zone")
			continue
		}
		inst.ProjectID = c.projectID
		inst.Name = nodelist.Items[node].GetName()

		// If node is not ready we will basically remove the node IP just in case
		if !isNodeReady(&nodelist.Items[node]) {
			c.logger.Debugf("node %s in zone %s is not ready, remove IP for reuse", inst.Name, inst.Zone)
			// Delete the IP we will re-assign this
			err = kipcompute.DeleteIP(c.projectID, inst.Zone, inst.Name, c.config)
			if err != nil {
				return errors.Wrap(err, "failed to delete IP")
			}
			continue
		}
		nodesOfInterest = append(nodesOfInterest, inst)
	}

	// Should we check that the IPs assigned to the current nodes are in fact the best possible IPs to assign?
	if shouldCheckOptimalIPAssignment {
		// Determining the required IP per region
		regionsCount := make(map[string][]types.Instance)
		c.logger.Debugf("collected %d Nodes of interest...calculating number of IPs required", len(nodesOfInterest))
		for _, inst := range nodesOfInterest {
			zone := inst.Zone
			region := zone[:len(zone)-2]
			regionsCount[region] = append(regionsCount[region], inst)
		}

		// Determining the most optimal nodes per region.
		for region, instances := range regionsCount {
			addresses, err := kipcompute.GetAllAddresses(c.projectID, region, false, c.config)
			if err != nil {
				return errors.Wrap(err, "failed to retrieve all addresses")
			}

			var topMostAddresses []string
			for _, address := range addresses.Items[:utils.Min(len(instances), len(addresses.Items))] {
				topMostAddresses = append(topMostAddresses, address.Address)
			}

			// Retrieve all addresses in the region.
			var usedAddresses []AddressInstanceTuple
			for _, instance := range instances {
				address, errGet := kipcompute.GetAddressUsedByInstance(c.projectID, instance.Name, instance.Zone, c.config)
				if errGet != nil {
					return errors.Wrap(errGet, "failed to retrieve address")
				}
				usedAddresses = append(usedAddresses, AddressInstanceTuple{
					address,
					instance,
				})
			}

			// Perform subtraction
			c.logger.Infof("project %s in region %s should use the following IPs %s... Checking that the instances follow these assignments", c.projectID, region, topMostAddresses)
			var toRemove []AddressInstanceTuple
			for _, usedAddress := range usedAddresses {
				if usedAddress.address != "0.0.0.0" && !utils.Contains(topMostAddresses, usedAddress.address) {
					toRemove = append(toRemove, usedAddress)
				}
			}

			if len(toRemove) > 0 {
				for _, remove := range toRemove {
					// Delete the IP we will re-assign this
					err = kipcompute.DeleteIP(c.projectID, remove.instance.Zone, remove.instance.Name, c.config)
					if err != nil {
						return errors.Wrap(err, "failed to delete IP")
					}
				}
			}
		}
	}

	for _, inst := range nodesOfInterest {
		if !kipcompute.IsInstanceUsesReservedIP(c.projectID, inst.Name, inst.Zone, c.config) {
			c.logger.WithFields(logrus.Fields{
				"instance": inst.Name,
				"pool":     inst.Pool,
			}).Debugf("found unassigned node in pool")
			c.instance <- inst
		}
	}
	return nil
}

func (c *Controller) forceAssignmentOnce(shouldCheckOptimalIPAssignment bool) {
	if !c.processing {
		c.processing = true
		if c.config.ForceAssignment {
			err := c.processAllNodes(shouldCheckOptimalIPAssignment)
			if err != nil {
				c.logger.WithError(err).Error("failed to process all nodes")
			}
		}
		c.assignMissingTags()
		c.processing = false
	}
}

func (c *Controller) forceAssignment() {
	c.forceAssignmentOnce(true)
	for range c.ticker.C {
		c.forceAssignmentOnce(false)
	}
}

func (c *Controller) assignMissingTags() {
	nodePools := c.config.AdditionalNodePools
	nodePools = append(nodePools, c.config.NodePool)

	kubeClient := utils.GetClient()

	for _, pool := range nodePools {
		label := fmt.Sprintf("!kubip_assigned,cloud.google.com/gke-nodepool=%s", pool)
		nodelist, err := kubeClient.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{LabelSelector: label})
		if err != nil {
			c.logger.Warn("failed to list nodes")
			continue
		}
		for _, node := range nodelist.Items {
			labels := node.GetLabels()
			if nodeZone, ok := labels["failure-domain.beta.kubernetes.io/zone"]; ok {
				kipcompute.AddTagIfMissing(c.projectID, node.GetName(), nodeZone, c.config)
			}
		}
	}
}
