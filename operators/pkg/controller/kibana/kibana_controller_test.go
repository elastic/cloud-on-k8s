// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package kibana

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/elastic/k8s-operators/operators/pkg/utils/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	kbtype "github.com/elastic/k8s-operators/operators/pkg/apis/kibana/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c k8s.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}
var depKey = types.NamespacedName{Name: "foo-kibana", Namespace: "default"}

func TestReconcile(t *testing.T) {

	instance := &kbtype.Kibana{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: kbtype.KibanaSpec{
			Version: "6.6.1",
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(test.Config, manager.Options{})
	assert.NoError(t, err)
	c = k8s.WrapClient(mgr.GetClient())

	reconciler := newReconciler(mgr)
	recFn, requests := SetupTestReconcile(reconciler)
	controller, err := add(mgr, recFn)
	assert.NoError(t, err)
	assert.NoError(t, addWatches(controller, reconciler))

	stopMgr, mgrStopped := StartTestManager(mgr, t)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Create the Kibana object and expect the Reconcile and Deployment to be created
	err = c.Create(instance)
	assert.NoError(t, err)
	defer c.Delete(instance)
	test.CheckReconcileCalled(t, requests, expectedRequest)

	// Deployment won't be created until we provide details for the ES backend
	secret := mockCASecret(t, c)

	test.RetryUntilSuccess(t, func() error {
		// refetch Kibana as finalizers have been installed
		require.NoError(t, c.Get(k8s.ExtractNamespacedName(instance), instance))
		if err != nil {
			return err
		}
		instance.Spec.Elasticsearch = kbtype.BackendElasticsearch{
			URL: "http://127.0.0.1:9200",
			Auth: kbtype.ElasticsearchAuth{
				Inline: &kbtype.ElasticsearchInlineAuth{
					Username: "foo",
					Password: "bar",
				},
			},
			CaCertSecret: &secret.Name,
		}
		err := c.Update(instance)
		if errors.IsConflict(err) {
			// delayed cache update might require us to try this more than once
			return err
		}
		// don't tolerate any other err
		require.NoError(t, err)
		return nil
	})

	test.CheckReconcileCalled(t, requests, expectedRequest)

	deploy := &appsv1.Deployment{}
	test.RetryUntilSuccess(t, func() error {
		return c.Get(depKey, deploy)
	})

	// Delete the Deployment and expect Reconcile to be called for Deployment deletion
	assert.NoError(t, c.Delete(deploy))
	test.CheckReconcileCalled(t, requests, expectedRequest)

	test.RetryUntilSuccess(t, func() error {
		return c.Get(depKey, deploy)
	})
	// Manually delete Deployment since GC isn't enabled in the test control plane
	test.DeleteIfExists(t, c, deploy)

}

func mockCASecret(t *testing.T, c k8s.Client) *v1.Secret {
	// The Kibana resource needs a CA secret created by the Elasticsearch controller
	// but the Elasticsearch controller is not running.
	// Here we are creating a dummy secret
	// TODO: This would not be necessary if we would allow embedding the secret

	caSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Data: map[string][]byte{
			certificates.CAFileName: []byte("fake-ca-cert"),
		},
	}
	assert.NoError(t, c.Create(caSecret))

	return caSecret
}
