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

package controller

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"

	cfg "github.com/doitintl/kubeip/pkg/config"
	"github.com/doitintl/kubeip/pkg/kipcompute"
	"github.com/doitintl/kubeip/pkg/types"
	"github.com/doitintl/kubeip/pkg/utils"
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
}

// Event indicate the informerEvent
type Event struct {
	key          string
	eventType    string
	namespace    string
	resourceType string
	name         string
}

var serverStartTime time.Time

const maxRetries = 5

const prefix = "kube-system/kube-proxy-"

func Start(config *cfg.Config) error {
	var kubeClient kubernetes.Interface
	_, err := rest.InClusterConfig()
	if err != nil {
		logrus.Fatal(err)
	} else {
		kubeClient = utils.GetClient()
	}
	informer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return kubeClient.CoreV1().Pods(meta_v1.NamespaceAll).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return kubeClient.CoreV1().Pods(meta_v1.NamespaceAll).Watch(options)
			},
		},
		&api_v1.Pod{},
		0, //Skubeip resync
		cache.Indexers{},
	)

	c := newResourceController(kubeClient, informer, "node")
	c.projectID, err = kipcompute.ProjectName()
	if err != nil {
		logrus.Fatal(err)
		return err
	}
	c.clusterName, err = kipcompute.ClusterName()
	if err != nil {
		logrus.Fatal(err)
		return err
	}
	c.config = config
	c.ticker = time.NewTicker(c.config.Ticker * time.Minute)
	stopCh := make(chan struct{})
	defer close(stopCh)
	//TODO Set size
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
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
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
	})

	return &Controller{
		logger:    logrus.WithField("pkg", "kubeip-"+resourceType),
		clientset: client,
		informer:  informer,
		queue:     queue,
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

func (c *Controller) isNodePollMonitored(pool string) bool {
	if strings.ToLower(pool) == strings.ToLower(c.config.NodePool) {
		return true
	}
	for _, ns := range c.config.AdditionalNodePools{
		if strings.ToLower(pool) == strings.ToLower(ns) {
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
	// get object's metedata
	objectMeta := utils.GetObjectMetaData(obj)

	// process events based on its type
	switch newEvent.eventType {
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
				nodeMeta, err := kubeClient.CoreV1().Nodes().Get(node, options)
				if err != nil {
					logrus.Infof(err.Error())
				}
				labels := nodeMeta.Labels
				var pool string
				var ok bool
				if pool, ok = labels["cloud.google.com/gke-nodepool"]; ok {
					logrus.Infof("Node pool found %s", pool)
					if !c.isNodePollMonitored(pool) {
						return nil
					}
				} else {
					logrus.Info("Did not found node pool")
					return nil
				}
				var inst types.Instance
				if nodeZone, ok := labels["failure-domain.beta.kubernetes.io/zone"]; ok {
					logrus.Infof("Zone pool found %s", nodeZone)
					inst.Zone = nodeZone
				} else {
					logrus.Info("Did not find zone")
					return nil
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

func (c *Controller) processAllNodes() {
	kubeClient := utils.GetClient()
	logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).Info("Collecting Node List...")
	nodelist, _ := kubeClient.CoreV1().Nodes().List(meta_v1.ListOptions{})
	var pool string
	var ok bool
	for _, node := range nodelist.Items {
		labels := node.GetLabels()
		if pool, ok = labels["cloud.google.com/gke-nodepool"]; ok {
			if !c.isNodePollMonitored(pool) {
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
		if !kipcompute.IsInstanceUsesReservedIP(c.projectID, inst.Name, inst.Zone, c.config) {
			logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).Infof("Found un assigned node %s in pool", inst.Name, inst.Pool)
			c.instance <- inst
		}

	}

}

func (c *Controller) forceAssignment() {
	if c.config.ForceAssignment {
		logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "forceAssignment"}).Info("Starting forceAssignment")
		c.processAllNodes()
	}
	c.assignMissingTags()
	for _ = range c.ticker.C {
		if c.config.ForceAssignment {
			logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "processAllNodes"}).Info("On Ticker")
			c.processAllNodes()
		}
		c.assignMissingTags()
	}
}

func (c *Controller) assignMissingTags() {
	kubeClient := utils.GetClient()
	label := fmt.Sprintf("!kubip_assigned,cloud.google.com/gke-nodepool=%s", c.config.NodePool)
	nodelist, err := kubeClient.CoreV1().Nodes().List(meta_v1.ListOptions{
		LabelSelector: label,
	})
	if err != nil {
		logrus.Error(err)
		return

	}
	for _, node := range nodelist.Items {
		labels := node.GetLabels()
		if nodeZone, ok := labels["failure-domain.beta.kubernetes.io/zone"]; ok {
			if err != nil {
				logrus.Fatalf("Could not get authenticated client: %v", err)
				continue
			}
			logrus.WithFields(logrus.Fields{"pkg": "kubeip", "function": "assignMissingTags"}).Infof("Found node without tag %s", node.GetName())
			kipcompute.AddTagIfMissing(c.projectID, node.GetName(), nodeZone)

		} else {
			continue
		}
	}

}
