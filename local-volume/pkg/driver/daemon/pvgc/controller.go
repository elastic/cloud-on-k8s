// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pvgc

import (
	"context"
	"fmt"
	"time"

	"github.com/elastic/cloud-on-k8s/local-volume/pkg/driver/daemon/drivers"
	"github.com/elastic/cloud-on-k8s/local-volume/pkg/provider"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// reconcileVolumeArgs captures arguments and some state from the reconcile part of the controller
type reconcileVolumeArgs struct {
	key    string
	exists bool
	err    error
}

// onReconcileHandler is a method used to verify reconcile arguments
type onReconcileHandler func(reconcileVolumeArgs)

// Controller handles purging local volumes from the driver once their PV API resource object is deleted.
type Controller struct {
	driver drivers.Driver

	indexer  cache.Indexer
	queue    workqueue.RateLimitingInterface
	informer cache.Controller

	// testOnReconcile is a method that if set, will be called on every internal reconcile
	testOnReconcile onReconcileHandler
}

type ControllerParams struct {
	Client   kubernetes.Interface
	NodeName string
	Driver   drivers.Driver

	// testWatcher is used instead of using the client, which is useful for the tests because the fake client
	// implementation does not work for watchers.
	testWatcher cache.ListerWatcher

	// testOnReconcile is a method that if set, will be called on every internal reconcile, used in tests to verify
	// that specific keys are being reconciled.
	testOnReconcile onReconcileHandler
}

func NewController(p ControllerParams) (*Controller, error) {
	var watcher = p.testWatcher
	if watcher == nil {
		// persistent volume watcher
		watcher = cache.NewFilteredListWatchFromClient(
			p.Client.CoreV1().RESTClient(),
			"persistentvolumes",
			corev1.NamespaceAll,
			func(options *metav1.ListOptions) {
				options.LabelSelector = fmt.Sprintf("%s=%s", provider.NodeAffinityLabel, p.NodeName)
			},
		)
	}

	// work queue
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// Bind the workqueue to a cache with the help of an informer. This way we make sure that
	// whenever the cache is updated, the pv key is added to the workqueue.
	// Note that when we finally process the item from the workqueue, we might see a newer version
	// of the pv than the version which was responsible for triggering the update.
	indexer, informer := cache.NewIndexerInformer(watcher, &v1.PersistentVolume{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(key)
			}
		},
		UpdateFunc: func(old interface{}, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err == nil {
				queue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			// IndexerInformer uses a delta queue, therefore for deletes we have to use this
			// key function.
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(key)
			}
		},
	}, cache.Indexers{})

	// We can now warm up the cache for initial synchronization.
	// Let's suppose that we knew about a PV "mypv" on our last run, therefore add it to the cache.
	// If this PV is not there anymore, the controller will be notified about the removal after the
	// cache has synchronized.

	currentVolumeNames, err := p.Driver.ListVolumes()
	if err != nil {
		return nil, err
	}
	for _, volumeName := range currentVolumeNames {
		log.Infof("Warming cache for known PV: %s", volumeName)
		if err := indexer.Add(&v1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: volumeName,
			},
		}); err != nil {
			return nil, err
		}
	}

	return &Controller{
		driver: p.Driver,

		indexer:  indexer,
		informer: informer,
		queue:    queue,

		testOnReconcile: p.testOnReconcile,
	}, nil
}

func (c *Controller) Run(ctx context.Context) error {
	// Let the workers stop when we are done
	defer c.queue.ShutDown()

	log.Info("Starting PV Controller")
	go c.informer.Run(ctx.Done())

	// Wait for all involved caches to be synced, before processing items from the queue is started
	if !cache.WaitForCacheSync(ctx.Done(), c.informer.HasSynced) {
		return fmt.Errorf("timed out waiting for caches to sync")
	}

	// we start a single worker for now. if the worker finishes, we start it again after a second.
	go wait.Until(c.runWorker, time.Second, ctx.Done())

	// wait until our context is done
	<-ctx.Done()

	log.Info("Stopping PV Controller")

	return nil
}

// runWorker processes all remaining items in the queue before returning.
func (c *Controller) runWorker() {
	for c.processNextItem() {
	}
}

// processNextItem processes a single work item from the queue
func (c *Controller) processNextItem() bool {
	// Wait until there is a new item in the working queue
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	// Tell the queue that we are done with processing this key. This unblocks the key for other workers
	// This allows safe parallel processing because two pvs with the same key are never processed in
	// parallel.
	defer c.queue.Done(key)

	// Invoke the method containing the business logic
	err := c.reconcileForKey(key.(string))
	// Handle the error if something went wrong during the execution of the business logic
	c.handleErr(err, key)
	return true
}

// handleErr checks if an error happened and makes sure we will retry later.
func (c *Controller) handleErr(err error, key interface{}) {
	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		c.queue.Forget(key)
		return
	}

	// This Controller retries 5 times if something goes wrong. After that, it stops trying.
	if c.queue.NumRequeues(key) < 5 {
		log.Infof("Error syncing pv %v: %v", key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		c.queue.AddRateLimited(key)
		return
	}

	c.queue.Forget(key)
	// Report to an external entity that, even after several retries, we could not successfully process this key
	runtime.HandleError(err)
	log.Infof("Dropping pv %q out of the queue: %v", key, err)
}

// reconcileForKey is the business logic of the Controller. In this Controller it simply prints
// information about the pv to stdout. In case an error happened, it has to simply return the error.
// The retry logic should not be part of the business logic.
func (c *Controller) reconcileForKey(key string) error {
	obj, exists, err := c.indexer.GetByKey(key)

	// notify our testOnReconcile handler if it is set in order to allow unit tests to verify reconcile has been
	// called with some specific arguments.
	if c.testOnReconcile != nil {
		c.testOnReconcile(reconcileVolumeArgs{key: key, exists: exists, err: err})
	}

	if err != nil {
		log.Infof("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if exists {
		// Note that you also have to check the uid if you have a local controlled resource, which
		// is dependent on the actual instance, to detect that a PV was recreated with the same name
		log.Infof("Sync/Add/Update for PV %s", obj.(*v1.PersistentVolume).GetName())
		return nil
	}

	log.Infof("PV %s does not exist anymore, purging", key)
	if err := c.driver.PurgeVolume(key); err != nil {
		return err
	}
	log.Infof("Successfully purged PV %s", key)
	return nil
}
