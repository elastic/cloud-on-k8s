package stack

import (
	"fmt"
	"testing"
	"time"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c client.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}
var depKey = types.NamespacedName{Name: "foo-es", Namespace: "default"}
var discoveryServiceKey = types.NamespacedName{Name: "foo-es-discovery", Namespace: "default"}
var publicServiceKey = types.NamespacedName{Name: "foo-es-public", Namespace: "default"}

const timeout = time.Second * 5
const retryInterval = time.Millisecond * 100

func retryUntilSuccess(t *testing.T, timeout time.Duration, retryInterval time.Duration, f func() error) {
	timeoutChan := time.After(timeout)
	for {
		resp := make(chan (error))
		go func() {
			resp <- f()
		}()
		select {
		case <-timeoutChan:
			assert.Fail(t, fmt.Sprintf("%s timeout reached", timeout))
			return
		case fSuccess := <-resp:
			if fSuccess == nil {
				return
			}
			select {
			case <-time.After(retryInterval):
				continue
			case <-timeoutChan:
				assert.Fail(t, fmt.Sprintf("%s timeout reached. Error: %s", timeout, fSuccess.Error()))
				return
			}
		}
	}
}

// eventually is a wrapper around retryUntilSuccess with default values
func eventually(t *testing.T, f func() error) {
	retryUntilSuccess(t, timeout, retryInterval, f)
}

func checkReconcileCalled(t *testing.T, requests chan reconcile.Request) {
	select {
	case req := <-requests:
		assert.Equal(t, req, expectedRequest)
	case <-time.After(timeout):
		assert.Fail(t, fmt.Sprintf("No request received after %s", timeout))
	}
}

func checkResourceDeletionTriggersReconcile(t *testing.T, requests chan reconcile.Request, objKey types.NamespacedName, obj runtime.Object) {
	assert.NoError(t, c.Delete(context.TODO(), obj))
	checkReconcileCalled(t, requests)
	eventually(t, func() error { return c.Get(context.TODO(), objKey, obj) })
}

func TestReconcile(t *testing.T) {
	instance := &deploymentsv1alpha1.Stack{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: deploymentsv1alpha1.StackSpec{
			Elasticsearch: deploymentsv1alpha1.ElasticsearchSpec{
				SetVmMaxMapCount: false,
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	assert.NoError(t, err)
	c = mgr.GetClient()

	recFn, requests := SetupTestReconcile(newReconciler(mgr))
	assert.NoError(t, add(mgr, recFn))

	stopMgr, mgrStopped := StartTestManager(mgr, t)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Create the Stack object and expect the Reconcile and Deployment to be created
	err = c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	assert.NoError(t, err)
	defer c.Delete(context.TODO(), instance)

	checkReconcileCalled(t, requests)

	// Deployment should be created
	deploy := &appsv1.Deployment{}
	eventually(t, func() error { return c.Get(context.TODO(), depKey, deploy) })

	// Services should be created
	discoveryService := &corev1.Service{}
	eventually(t, func() error { return c.Get(context.TODO(), discoveryServiceKey, discoveryService) })
	publicService := &corev1.Service{}
	eventually(t, func() error { return c.Get(context.TODO(), publicServiceKey, publicService) })

	// Delete the Deployment and expect Reconcile to be called for Deployment deletion
	checkResourceDeletionTriggersReconcile(t, requests, depKey, deploy)
	// Same for services
	checkResourceDeletionTriggersReconcile(t, requests, publicServiceKey, publicService)
	checkResourceDeletionTriggersReconcile(t, requests, discoveryServiceKey, discoveryService)

	// Manually delete Deployment and Services since GC might not be enabled in the test control plane
	clean(t, deploy)
	clean(t, publicService)
	clean(t, discoveryService)
}

func clean(t *testing.T, obj runtime.Object) {
	err := c.Delete(context.TODO(), obj)
	// If the resource is already deleted, we don't care, but any other error is important
	if !apierrors.IsNotFound(err) {
		assert.NoError(t, err)
	}
}
