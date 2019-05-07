// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package elasticsearch

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	estype "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/operator"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	esclient "github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/elastic/k8s-operators/operators/pkg/utils/test"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c k8s.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}
var discoveryServiceKey = types.NamespacedName{Name: "foo-es-discovery", Namespace: "default"}
var externalServiceKey = types.NamespacedName{Name: "foo-es", Namespace: "default"}

func getESPods(t *testing.T) []corev1.Pod {
	esPods := &corev1.PodList{}
	esPodSelector := client.ListOptions{Namespace: "default"}
	err := esPodSelector.SetLabelSelector("common.k8s.elastic.co/type=elasticsearch")
	assert.NoError(t, err)
	test.RetryUntilSuccess(t, func() error {
		return c.List(&esPodSelector, esPods)
	})
	return esPods.Items
}

func checkNumberOfPods(t *testing.T, expected int) {
	test.RetryUntilSuccess(t, func() error {
		nPods := len(getESPods(t))
		if nPods != expected {
			return fmt.Errorf("got %d pods, expected %d", nPods, expected)
		}
		return nil
	})
}

func TestReconcile(t *testing.T) {
	varFalse := false
	instance := &estype.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: estype.ElasticsearchSpec{
			Version:          "7.0.0",
			SetVMMaxMapCount: &varFalse,
			Nodes: []v1alpha1.NodeSpec{
				{
					Config: &estype.Config{
						Data: map[string]interface{}{
							estype.NodeMaster: true,
						},
					},
					NodeCount: 3,
				},
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(test.Config, manager.Options{})

	assert.NoError(t, err)
	c = k8s.WrapClient(mgr.GetClient())

	r, err := newReconciler(mgr, operator.Parameters{OperatorImage: "operator-image-dummy"})
	require.NoError(t, err)
	recFn, requests := SetupTestReconcile(r)
	controller, err := add(mgr, recFn)
	assert.NoError(t, err)
	assert.NoError(t, addWatches(controller, r))

	stopMgr, mgrStopped := StartTestManager(mgr, t)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Pre-create dependent Endpoint which will not be created automatically as only the Elasticsearch controller is running.
	endpoints := &corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "foo-es", Namespace: "default"}}
	err = c.Create(endpoints)
	assert.NoError(t, err)
	// Create the Elasticsearch object and expect the Reconcile and Deployment to be created
	err = c.Create(instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	assert.NoError(t, err)
	defer c.Delete(instance)

	test.CheckReconcileCalled(t, requests, expectedRequest)

	// Elasticsearch pods should be created
	checkNumberOfPods(t, 3)

	// Services should be created
	discoveryService := &corev1.Service{}
	test.RetryUntilSuccess(t, func() error { return c.Get(discoveryServiceKey, discoveryService) })
	externalService := &corev1.Service{}
	test.RetryUntilSuccess(t, func() error { return c.Get(externalServiceKey, externalService) })

	// simulate a cluster observed by observers
	observedCluster := types.NamespacedName{
		Namespace: "ns",
		Name:      "observedCluster",
	}
	esclientGreen := esclient.NewMockClientWithUser(version.MustParse("6.7.0"),
		esclient.UserAuth{},
		func(req *http.Request) *http.Response {
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(bytes.NewBufferString(`{"status": "green"}`)),
				Header:     make(http.Header),
				Request:    req,
			}
		})
	r.esObservers.Observe(observedCluster, nil, esclientGreen)
	// cluster health should have gone from none to green,
	// check reconciliation was called for the observed cluster
	expectedReq := reconcile.Request{NamespacedName: observedCluster}
	test.RetryUntilSuccess(t, func() error {
		select {
		case evt := <-requests:
			if evt != expectedReq {
				return errors.New("not the expected reconciliation")
			}
			// we got one reconciliation!
			return nil
		case <-time.After(test.Timeout):
			return errors.New("no reconciliation after timeout")
		}
	})

	// Delete resources and expect Reconcile to be called and eventually recreate them
	// ES pod
	assert.NoError(t, c.Delete(&getESPods(t)[0]))
	test.CheckReconcileCalled(t, requests, expectedRequest)
	checkNumberOfPods(t, 3)

	// Services
	test.CheckResourceDeletionTriggersReconcile(t, c, requests, externalServiceKey, externalService, expectedRequest)
	test.CheckResourceDeletionTriggersReconcile(t, c, requests, discoveryServiceKey, discoveryService, expectedRequest)

	// Manually delete Deployment and Services since GC might not be enabled in the test control plane
	test.DeleteIfExists(t, c, externalService)
	test.DeleteIfExists(t, c, discoveryService)
	test.DeleteIfExists(t, c, endpoints)

}
