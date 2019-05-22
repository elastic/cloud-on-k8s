// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package license

import (
	"fmt"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/chrono"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/test"
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

func TestMain(m *testing.M) {
	test.RunWithK8s(m, filepath.Join("..", "..", "..", "config", "crds"))
}

func TestReconcile(t *testing.T) {
	// start the test manager & controller
	c, stop := test.StartTestController(t, Add, operator.Parameters{})
	defer stop()

	thirtyDays := 30 * 24 * time.Hour
	now := time.Now()
	startDate := now.Add(-thirtyDays)
	expiryDate := now.Add(thirtyDays)

	enterpriseLicense := &v1alpha1.EnterpriseLicense{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "elastic-system"},
		Spec: v1alpha1.EnterpriseLicenseSpec{
			LicenseMeta: v1alpha1.LicenseMeta{
				UID:                "test",
				ExpiryDateInMillis: chrono.ToMillis(expiryDate),
			},
			Type:         "enterprise",
			SignatureRef: corev1.SecretKeySelector{},
			ClusterLicenseSpecs: []v1alpha1.ClusterLicenseSpec{
				{
					LicenseMeta: v1alpha1.LicenseMeta{
						ExpiryDateInMillis: chrono.ToMillis(expiryDate),
						StartDateInMillis:  chrono.ToMillis(startDate),
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

	// Create the EnterpriseLicense object
	assert.NoError(t, c.Create(enterpriseLicense))
	defer c.Delete(enterpriseLicense)

	// Create the linked secret
	assert.NoError(t, c.Create(controllerSecret))
	defer c.Delete(controllerSecret)

	varFalse := false
	cluster := &v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: v1alpha1.ElasticsearchSpec{
			Version:          "7.0.0",
			SetVMMaxMapCount: &varFalse,
			Nodes: []v1alpha1.NodeSpec{
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
