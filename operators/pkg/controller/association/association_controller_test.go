// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package association

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/associations/v1alpha1"
	esv1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	kbv1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/nodecerts"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/secret"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/elastic/k8s-operators/operators/pkg/utils/test"
	"github.com/pkg/errors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c k8s.Client

var resourceKey = types.NamespacedName{Name: "baz", Namespace: "default"}
var kibanaKey = types.NamespacedName{Name: "bar", Namespace: "default"}
var expectedRequest = reconcile.Request{NamespacedName: resourceKey}

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
	addWatches(controller, rec)

	stopMgr, mgrStopped := StartTestManager(mgr, t)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Assume an Elasticsearch cluster and a Kibana has been created
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
	secrets := mockSecrets(t, c)

	// Create the stack resource, that should be reconciled
	instance := &v1alpha1.KibanaElasticsearchAssociation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceKey.Name,
			Namespace: resourceKey.Namespace,
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

	// Currently no effects on Elasticsearch cluster (TODO decouple user creation)

	// Kibana should be updated
	kibana := &kbv1alpha1.Kibana{}
	test.RetryUntilSuccess(t, func() error {
		err := c.Get(kibanaKey, kibana)
		if err != nil {
			return err
		}
		err = errors.New("Not reconciled yet")

		switch e := kibana.Spec.Elasticsearch; {
		case e.URL == "", e.CaCertSecret == nil, e.Auth.Inline.Username == "", e.Auth.Inline.Password == "":
			return err
		default:
			return nil
		}
	})

	// TODO what should happen on delete of one of the vertices of the association?
	// Manually delete Cluster, Deployment and Secret since GC might not be enabled in the test control plane
	test.DeleteIfExists(t, c, es)
	test.DeleteIfExists(t, c, kibana)
	for _, s := range secrets {
		test.DeleteIfExists(t, c, s)
	}
}

func mockSecrets(t *testing.T, c k8s.Client) []*v1.Secret {
	// The Kibana resource needs some secrets to be created,
	// but the Elasticsearch controller is not running.
	// Here we are creating dummy secrets to pretend they exist.
	// TODO: This would not be necessary if Kibana and Elasticsearch were less coupled.

	userSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.ElasticInternalUsersSecretName("foo"),
			Namespace: "default",
		},
		Data: map[string][]byte{
			secret.InternalKibanaServerUserName: []byte("blub"),
		},
	}
	assert.NoError(t, c.Create(userSecret))

	caSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Data: map[string][]byte{
			nodecerts.SecretCAKey: []byte("fake-ca-cert"),
		},
	}
	assert.NoError(t, c.Create(caSecret))

	return []*v1.Secret{userSecret, caSecret}
}
