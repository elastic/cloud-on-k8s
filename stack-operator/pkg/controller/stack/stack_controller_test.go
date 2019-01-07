// +build integration

package stack

import (
	"testing"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	esv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	kbv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/nodecerts"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c client.Client

var resourceKey = types.NamespacedName{Name: "foo", Namespace: "default"}
var expectedRequest = reconcile.Request{NamespacedName: resourceKey}

func TestReconcile(t *testing.T) {
	instance := &deploymentsv1alpha1.Stack{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: deploymentsv1alpha1.StackSpec{
			Elasticsearch: esv1alpha1.ElasticsearchSpec{
				SetVMMaxMapCount: false,
				Topologies: []esv1alpha1.ElasticsearchTopologySpec{
					{
						NodeCount: 3,
					},
				},
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	assert.NoError(t, err)
	c = mgr.GetClient()

	rec, err := newReconciler(mgr)
	require.NoError(t, err)
	recFn, requests := SetupTestReconcile(rec)
	assert.NoError(t, add(mgr, recFn))

	stopMgr, mgrStopped := StartTestManager(mgr, t)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Pretend secrets created by the Elasticsearch controller are there
	secrets := mockSecrets(t, c)

	// Create the stack resource, that should be reconciled
	err = c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	assert.NoError(t, err)
	defer c.Delete(context.TODO(), instance)
	test.CheckReconcileCalled(t, requests, expectedRequest)

	// Elasticsearch cluster should be created
	es := &esv1alpha1.ElasticsearchCluster{}
	test.RetryUntilSuccess(t, func() error { return c.Get(context.TODO(), resourceKey, es) })

	// Kibana should be created
	kibana := &kbv1alpha1.Kibana{}
	test.RetryUntilSuccess(t, func() error { return c.Get(context.TODO(), resourceKey, kibana) })

	// Delete resources and expect Reconcile to be called and eventually recreate them
	// ES cluster
	test.CheckResourceDeletionTriggersReconcile(t, c, requests, resourceKey, es, expectedRequest)
	// Kibana
	test.CheckResourceDeletionTriggersReconcile(t, c, requests, resourceKey, kibana, expectedRequest)

	// Manually delete Cluster, Deployment and Secret since GC might not be enabled in the test control plane
	test.DeleteIfExists(t, c, es)
	test.DeleteIfExists(t, c, kibana)
	for _, s := range secrets {
		test.DeleteIfExists(t, c, s)
	}
}

func mockSecrets(t *testing.T, c client.Client) []*v1.Secret {
	// The Kibana resource needs some secrets to be created,
	// but the Elasticsearch controller is not running.
	// Here we are creating dummy secrets to pretend they exist.
	// TODO: This would not be necessary if Kibana and Elasticsearch were less coupled.

	userSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      support.ElasticInternalUsersSecretName("foo"),
			Namespace: "default",
		},
		Data: map[string][]byte{
			support.InternalKibanaServerUserName: []byte("blub"),
		},
	}
	assert.NoError(t, c.Create(context.TODO(), userSecret))

	caSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Data: map[string][]byte{
			nodecerts.SecretCAKey: []byte("fake-ca-cert"),
		},
	}
	assert.NoError(t, c.Create(context.TODO(), caSecret))

	return []*v1.Secret{userSecret, caSecret}
}
