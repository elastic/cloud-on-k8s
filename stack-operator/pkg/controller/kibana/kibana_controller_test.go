// +build integration

package kibana

import (
	"testing"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/test"
	"github.com/stretchr/testify/assert"

	kibanav1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/kibana/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c k8s.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}
var depKey = types.NamespacedName{Name: "foo-kibana", Namespace: "default"}

const timeout = time.Second * 5

func TestReconcile(t *testing.T) {

	instance := &kibanav1alpha1.Kibana{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"}}

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

	// Create the Kibana object and expect the Reconcile and Deployment to be created
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
