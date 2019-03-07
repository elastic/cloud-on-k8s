// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package association

import (
	"fmt"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/associations/v1alpha1"
	esv1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	kbv1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/elastic/k8s-operators/operators/pkg/utils/test"
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
	associationKey  = types.NamespacedName{Name: "baz", Namespace: "default"}
	kibanaKey       = types.NamespacedName{Name: "bar", Namespace: "default"}
	expectedRequest = reconcile.Request{NamespacedName: associationKey}
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

	// Assume an Elasticsearch cluster and a Kibana have been created
	es := &esv1alpha1.ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
	}
	assert.NoError(t, c.Create(es))
	kb := kbv1alpha1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kibanaKey.Name,
			Namespace: kibanaKey.Namespace,
		},
	}
	assert.NoError(t, c.Create(&kb))
	// Pretend secrets created by the Elasticsearch controller are there
	caSecret := mockCaSecret(t, c, *es)

	// Create the association resource, that should be reconciled
	instance := &v1alpha1.KibanaElasticsearchAssociation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      associationKey.Name,
			Namespace: associationKey.Namespace,
		},
		Spec: v1alpha1.KibanaElasticsearchAssociationSpec{
			Elasticsearch: v1alpha1.ObjectSelector{
				Name:      "foo",
				Namespace: "default",
			},
			Kibana: v1alpha1.ObjectSelector{
				Name:      kibanaKey.Name,
				Namespace: kibanaKey.Namespace,
			},
		},
	}
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
	// let's wait until the Kibana update triggers another reconcile iteration
	test.CheckReconcileCalled(t, requests, expectedRequest)

	// Currently no effects on Elasticsearch cluster (TODO decouple user creation)

	// Kibana should be updated
	kibana := &kbv1alpha1.Kibana{}
	test.RetryUntilSuccess(t, func() error {
		err := c.Get(kibanaKey, kibana)
		if err != nil {
			return err
		}
		switch {
		case !kibana.Spec.Elasticsearch.IsConfigured():
			return errors.New("Not reconciled yet")
		default:
			return nil
		}
	})

	// Manually delete Cluster, Deployment and Secret since GC might not be enabled in the test control plane
	test.DeleteIfExists(t, c, es)
	test.DeleteIfExists(t, c, caSecret)

	// Ensure association goes back to pending if one of the vertices is deleted
	test.CheckReconcileCalled(t, requests, expectedRequest)
	test.RetryUntilSuccess(t, func() error {
		fetched := v1alpha1.KibanaElasticsearchAssociation{}
		err := c.Get(associationKey, &fetched)
		if err != nil {
			return err
		}
		if v1alpha1.AssociationPending != fetched.Status.AssociationStatus {
			return fmt.Errorf("expected %v, found %v", v1alpha1.AssociationPending, fetched.Status.AssociationStatus)
		}
		return nil
	})

	// Delete Kibana as well
	test.DeleteIfExists(t, c, kibana)

}

func mockCaSecret(t *testing.T, c k8s.Client, es esv1alpha1.ElasticsearchCluster) *corev1.Secret {
	// The Kibana resource needs a CA cert  secrets to be created,
	// but the Elasticsearch controller is not running.
	// Here we are creating a dummy CA secret to pretend they exist.
	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodecerts.CASecretNameForCluster(es.Name),
			Namespace: es.Namespace,
		},
		Data: map[string][]byte{
			certificates.CAFileName: []byte("fake-ca-cert"),
		},
	}
	assert.NoError(t, c.Create(caSecret))
	return caSecret
}
