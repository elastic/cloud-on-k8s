// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package apmserver

import (
	"errors"
	"testing"
	"time"

	apmv1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/apm/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/elastic/k8s-operators/operators/pkg/utils/test"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c k8s.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}
var depKey = types.NamespacedName{Name: "foo-apm-server", Namespace: "default"}

const timeout = time.Second * 5

func TestReconcile(t *testing.T) {
	instance := &apmv1alpha1.ApmServer{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"}}

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

	// Create the ApmServer object and expect the Reconcile and Deployment to be created
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

	// verify that the status is updated with the service name so we can safely update the instance
	test.RetryUntilSuccess(t, func() error {
		if err := c.Get(k8s.ExtractNamespacedName(instance), instance); err != nil {
			return err
		}

		if instance.Status.ExternalService == "" {
			return errors.New("waiting for external service name in status")
		}

		return nil
	})

	// Deployment won't be created until we provide details for the ES backend
	secret := mockCaSecret(t, c)
	instance.Spec.Output.Elasticsearch = apmv1alpha1.ElasticsearchOutput{
		Hosts: []string{"http://127.0.0.1:9200"},
		Auth: apmv1alpha1.ElasticsearchAuth{
			Inline: &apmv1alpha1.ElasticsearchInlineAuth{
				Username: "foo",
				Password: "bar",
			},
		},
		SSL: apmv1alpha1.ElasticsearchOutputSSL{
			CertificateAuthoritiesSecret: &secret.Name,
		},
	}

	assert.NoError(t, c.Update(instance))
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

func mockCaSecret(t *testing.T, c k8s.Client) *v1.Secret {
	// The ApmServer resource needs a CA secret created by the Elasticsearch controller
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
