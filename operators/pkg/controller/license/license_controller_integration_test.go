// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package license

import (
	"fmt"
	"testing"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/elastic/k8s-operators/operators/pkg/utils/test"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c k8s.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}

func listClusterLicenses(t *testing.T, c k8s.Client) []v1alpha1.ClusterLicense {
	clusterLicenses := v1alpha1.ClusterLicenseList{}
	assert.NoError(t, c.List(&client.ListOptions{}, &clusterLicenses))
	return clusterLicenses.Items
}

func validateOwnerRef(obj runtime.Object, cluster metav1.ObjectMeta) error {
	metaObj, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	owners := metaObj.GetOwnerReferences()
	if len(owners) != 1 {
		return fmt.Errorf("expected exactly 1 owner, got %d", len(owners))
	}

	ownerName := owners[0].Name
	ownerKind := owners[0].Kind
	expectedKind := "Elasticsearch"
	if ownerName != cluster.Name || ownerKind != expectedKind {
		return fmt.Errorf("expected owner %s (%s), got %s (%s)", cluster.Name, expectedKind, ownerName, ownerKind)
	}
	return nil
}

func TestReconcile(t *testing.T) {

	thirtyDays := 30 * 24 * time.Hour
	now := time.Now()
	startDate := now.Add(-thirtyDays)
	expiryDate := now.Add(thirtyDays)

	enterpriseLicense := &v1alpha1.EnterpriseLicense{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "elastic-system"},
		Spec: v1alpha1.EnterpriseLicenseSpec{
			LicenseMeta: v1alpha1.LicenseMeta{
				UID:                "test",
				ExpiryDateInMillis: test.ToMillis(expiryDate),
			},
			Type:         "enterprise",
			SignatureRef: corev1.SecretKeySelector{},
			ClusterLicenseSpecs: []v1alpha1.ClusterLicenseSpec{
				{
					LicenseMeta: v1alpha1.LicenseMeta{
						ExpiryDateInMillis: test.ToMillis(expiryDate),
						StartDateInMillis:  test.ToMillis(startDate),
					},
					Type: v1alpha1.LicenseTypePlatinum,
					SignatureRef: corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "ctrl-secret",
						},
						Key: "sig",
					},
				},
			},
		},
	}
	controllerSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ctrl-secret",
			Namespace: "elastic-system",
		},
		Data: map[string][]byte{
			"sig": []byte("blah"),
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(test.Config, manager.Options{})
	assert.NoError(t, err)
	c = k8s.WrapClient(mgr.GetClient())

	recFn, requests := SetupTestReconcile(newReconciler(mgr))
	assert.NoError(t, add(mgr, recFn))

	stopMgr, mgrStopped := StartTestManager(mgr, t)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Create the EnterpriseLicense object
	assert.NoError(t, c.Create(enterpriseLicense))
	defer c.Delete(enterpriseLicense)

	// Create the linked secret
	assert.NoError(t, c.Create(controllerSecret))
	defer c.Delete(controllerSecret)

	cluster := &v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: v1alpha1.ElasticsearchSpec{
			Version:          "7.0.0",
			LicenseType:      "platinum",
			SetVMMaxMapCount: false,
			Topology: []v1alpha1.TopologyElementSpec{
				{
					NodeCount: 3,
				},
			},
		},
	}
	assert.NoError(t, c.Create(cluster))
	test.CheckReconcileCalled(t, requests, expectedRequest)

	// test license assignment and ownership being triggered on cluster create
	test.RetryUntilSuccess(t, func() error {
		licenses := listClusterLicenses(t, c)
		numLicenses := len(licenses)
		if numLicenses != 1 {
			return fmt.Errorf("expected exactly 1 cluster license got %d", numLicenses)
		}
		validateOwnerRef(&licenses[0], cluster.ObjectMeta)
		return nil
	})

	test.RetryUntilSuccess(t, func() error {
		var secret corev1.Secret
		err := c.Get(types.NamespacedName{Name: "foo-license", Namespace: "default"}, &secret)
		if err != nil {
			return err
		}
		validateOwnerRef(&secret, cluster.ObjectMeta)
		return nil
	})

	// Delete the cluster and expect Reconcile to be called for cluster deletion
	test.DeleteIfExists(t, c, cluster)
	test.CheckReconcileCalled(t, requests, expectedRequest)
	// ClusterLicense should be GC'ed but can't be tested here
}

// purpose of this test is mostly to understand and document the delaying queue behaviour
// can be removed or skipped when it causes trouble in CI because these tests are non-deterministic
func TestDelayingQueueInvariants(t *testing.T) {
	item := types.NamespacedName{Name: "foo", Namespace: "bar"}
	tests := []struct {
		name                 string
		adds                 func(workqueue.DelayingInterface)
		expectedObservations int
		timeout              time.Duration
	}{
		{
			name: "single add",
			adds: func(q workqueue.DelayingInterface) {
				q.Add(item)
			},
			expectedObservations: 1,
			timeout:              10 * time.Millisecond,
		},
		{
			name: "deduplication",
			adds: func(q workqueue.DelayingInterface) {
				q.Add(item)
				q.Add(item)
			},
			expectedObservations: 1,
			timeout:              500 * time.Millisecond,
		},
		{
			name: "no dedup'ing when delaying",
			adds: func(q workqueue.DelayingInterface) {
				q.Add(item)
				q.AddAfter(item, 1*time.Millisecond)
			},
			expectedObservations: 2,
			timeout:              10 * time.Millisecond,
		},
		{
			name: "but dedup's and updates item within the wait queue",
			adds: func(q workqueue.DelayingInterface) {
				q.AddAfter(item, 1*time.Hour)        // schedule for an hour from now
				q.AddAfter(item, 1*time.Millisecond) // update scheduled item for a millisecond from now
			},
			expectedObservations: 1,
			timeout:              10 * time.Millisecond,
		},
		{
			name: "direct add and delayed add are independent",
			adds: func(q workqueue.DelayingInterface) {
				q.AddAfter(item, 10*time.Millisecond)
				q.Add(item) // should work despite one item in the work queue
			},
			expectedObservations: 2,
			timeout:              20 * time.Millisecond,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := workqueue.NewDelayingQueue()
			tt.adds(q)
			results := make(chan int)
			var seen int
			go func() {
				for {
					item, _ := q.Get()
					results <- 1
					q.Done(item)
				}
			}()
			collect := func() {
				for {
					select {
					case r := <-results:
						seen += r
					case <-time.After(tt.timeout):
						return
					}
				}

			}
			collect()
			assert.Equal(t, tt.expectedObservations, seen)
		})
	}

}
