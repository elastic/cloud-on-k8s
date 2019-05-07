// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package apmserverelasticsearchassociation

import (
	"fmt"
	"testing"

	apmv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	esv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/nodecerts"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/test"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	c               k8s.Client
	apmKey          = types.NamespacedName{Name: "bar", Namespace: "default"}
	expectedRequest = reconcile.Request{NamespacedName: apmKey}
)

func TestReconcile(t *testing.T) {

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(test.Config, manager.Options{})
	assert.NoError(t, err)
	c = k8s.WrapClient(mgr.GetClient())

	rec, err := newReconciler(mgr)
	require.NoError(t, err)
	recFn, requests := SetupTestReconcile(rec)
	controller, err := add(mgr, recFn)
	assert.NoError(t, err)
	assert.NoError(t, addWatches(controller, rec))

	stopMgr, mgrStopped := StartTestManager(mgr, t)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Assume an Elasticsearch cluster and a Apm Server have been created
	es := &esv1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
	}
	assert.NoError(t, c.Create(es))
	as := apmv1alpha1.ApmServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apmKey.Name,
			Namespace: apmKey.Namespace,
		},
		Spec: apmv1alpha1.ApmServerSpec{
			Output: apmv1alpha1.Output{
				Elasticsearch: apmv1alpha1.ElasticsearchOutput{
					Ref: &commonv1alpha1.ObjectSelector{
						Name:      "foo",
						Namespace: "default",
					},
				},
			},
		},
	}
	assert.NoError(t, c.Create(&as))
	// Pretend secrets created by the Elasticsearch controller are there
	caSecret := mockCaSecret(t, c, *es)

	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	assert.NoError(t, err)
	defer c.Delete(&as)
	test.CheckReconcileCalled(t, requests, expectedRequest)
	// let's wait until the Apm Server update triggers another reconcile iteration
	test.CheckReconcileCalled(t, requests, expectedRequest)

	// Currently no effects on Elasticsearch cluster (TODO decouple user creation)

	// ApmServer should be updated
	apmServer := &apmv1alpha1.ApmServer{}
	test.RetryUntilSuccess(t, func() error {
		err := c.Get(apmKey, apmServer)
		if err != nil {
			return err
		}
		switch {
		case !apmServer.Spec.Output.Elasticsearch.IsConfigured():
			return errors.New("Not reconciled yet")
		default:
			assert.Equal(t, commonv1alpha1.AssociationEstablished, apmServer.Status.Association)
			return nil
		}
	})

	// Manually delete Cluster, Deployment and Secret since GC might not be enabled in the test control plane
	test.DeleteIfExists(t, c, es)
	test.DeleteIfExists(t, c, caSecret)

	// Ensure association goes back to pending if one of the vertices is deleted
	test.CheckReconcileCalled(t, requests, expectedRequest)
	test.RetryUntilSuccess(t, func() error {
		fetched := apmv1alpha1.ApmServer{}
		err := c.Get(apmKey, &fetched)
		if err != nil {
			return err
		}
		if commonv1alpha1.AssociationPending != fetched.Status.Association {
			return fmt.Errorf("expected %v, found %v", commonv1alpha1.AssociationPending, fetched.Status.Association)
		}
		return nil
	})

	// Delete ApmServer as well
	test.DeleteIfExists(t, c, apmServer)
}

func mockCaSecret(t *testing.T, c k8s.Client, es esv1alpha1.Elasticsearch) *corev1.Secret {
	// The ApmServer resource needs a CA cert  secrets to be created,
	// but the Elasticsearch controller is not running.
	// Here we are creating a dummy CA secret to pretend they exist.
	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodecerts.CACertSecretName(es.Name),
			Namespace: es.Namespace,
		},
		Data: map[string][]byte{
			certificates.CAFileName: []byte("fake-ca-cert"),
		},
	}
	assert.NoError(t, c.Create(caSecret))
	return caSecret
}
