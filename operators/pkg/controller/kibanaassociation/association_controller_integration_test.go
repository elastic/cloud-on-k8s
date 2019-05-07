// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package kibanaassociation

import (
	"errors"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/common/v1alpha1"
	commonv1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/common/v1alpha1"
	esv1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	kbv1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/certificates"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/elastic/k8s-operators/operators/pkg/utils/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	c               k8s.Client
	kibanaKey       = types.NamespacedName{Name: "bar", Namespace: "default"}
	expectedRequest = reconcile.Request{NamespacedName: kibanaKey}
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

	// consume req requests in background, to let reconciliations go through
	go func() {
		for {
			select {
			case <-requests:
			case <-stopMgr:
				return
			}
		}
	}()

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// create an Elasticsearch cluster
	es := &esv1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
	}
	require.NoError(t, c.Create(es))
	// Pretend secrets created by the Elasticsearch controller are there
	_ = mockCaSecret(t, c, *es)

	// create a Kibana instance referencing that cluster
	kb := kbv1alpha1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kibanaKey.Name,
			Namespace: kibanaKey.Namespace,
		},
		Spec: kbv1alpha1.KibanaSpec{
			ElasticsearchRef: commonv1alpha1.ObjectSelector{
				Name:      es.Name,
				Namespace: es.Namespace,
			},
		},
	}
	require.NoError(t, c.Create(&kb))

	test.RetryUntilSuccess(t, func() error {
		if err := c.Get(kibanaKey, &kb); err != nil {
			return err
		}
		// Kibana should be updated with ES connection details
		if !kb.Spec.Elasticsearch.IsConfigured() {
			return errors.New("kibana not configured yet")
		}
		// association status should be established
		if kb.Status.AssociationStatus != v1alpha1.AssociationEstablished {
			return errors.New("association status not updated yet")
		}
		return nil
	})

	// delete user secret: it should be recreated
	kibanaUserSecretKey := KibanaUserSecretKey(kibanaKey)
	checkResourceRecreated(t, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kibanaUserSecretKey.Name,
			Namespace: kibanaUserSecretKey.Namespace,
		},
	})
	// delete user: it should be recreated
	kibanaUserKey := KibanaUserKey(kb, es.Namespace)
	checkResourceRecreated(t, &esv1alpha1.User{
		ObjectMeta: metav1.ObjectMeta{
			Name:      kibanaUserKey.Name,
			Namespace: kibanaUserKey.Namespace,
		},
	})

	// delete ES cluster
	require.NoError(t, c.Delete(es))

	test.RetryUntilSuccess(t, func() error {
		if err := c.Get(kibanaKey, &kb); err != nil {
			return err
		}
		// association status should be updated to pending
		if kb.Status.AssociationStatus != v1alpha1.AssociationPending {
			return errors.New("association status not updated yet")
		}
		// connection details should be removed
		if kb.Spec.Elasticsearch.IsConfigured() {
			return errors.New("kibana connection details still configured while ES doesn't exist")
		}
		return nil
	})
}

func mockCaSecret(t *testing.T, c k8s.Client, es esv1alpha1.Elasticsearch) *corev1.Secret {
	// The Kibana resource needs a CA cert  secrets to be created,
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

func checkResourceRecreated(t *testing.T, object runtime.Object) {
	metaObj, err := meta.Accessor(object)
	require.NoError(t, err)
	objKey := k8s.ExtractNamespacedName(metaObj)
	// retrieve the object and its uid
	require.NoError(t, c.Get(objKey, object))
	uid := metaObj.GetUID()

	// delete the object
	err = c.Delete(object)
	require.NoError(t, err)

	// should eventually be re-created (with a different uid)
	test.RetryUntilSuccess(t, func() error {
		err := c.Get(objKey, object)
		if err != nil {
			return err
		}
		metaObj, err := meta.Accessor(object)
		if err != nil {
			return err
		}
		newUid := metaObj.GetUID()
		if newUid == uid {
			return errors.New("same uid")
		}
		return nil
	})
}
