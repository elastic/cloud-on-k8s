// +build !unit

package stack

import (
	"testing"
	"time"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/onsi/gomega"
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

func TestReconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
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
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = mgr.GetClient()

	recFn, requests := SetupTestReconcile(newReconciler(mgr))
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

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
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	// Deployment should be created
	deploy := &appsv1.Deployment{}
	g.Eventually(func() error { return c.Get(context.TODO(), depKey, deploy) }, timeout).
		Should(gomega.Succeed())
	// Services should be created
	discoveryService := &corev1.Service{}
	g.Eventually(func() error { return c.Get(context.TODO(), discoveryServiceKey, discoveryService) }, timeout).
		Should(gomega.Succeed())
	publicService := &corev1.Service{}
	g.Eventually(func() error { return c.Get(context.TODO(), publicServiceKey, publicService) }, timeout).
		Should(gomega.Succeed())

	// Delete the Deployment and expect Reconcile to be called for Deployment deletion
	checkResourceDeletionTriggersReconcile(g, requests, depKey, deploy)
	// Same for services
	checkResourceDeletionTriggersReconcile(g, requests, discoveryServiceKey, discoveryService)
	checkResourceDeletionTriggersReconcile(g, requests, publicServiceKey, publicService)

	// Manually delete Deployment and Services since GC might not be enabled in the test control plane
	clean(g, deploy)
	clean(g, discoveryService)
	clean(g, publicService)
}

func checkResourceDeletionTriggersReconcile(g *gomega.GomegaWithT, requests chan reconcile.Request, objKey types.NamespacedName, obj runtime.Object) {
	g.Expect(c.Delete(context.TODO(), obj)).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))
	g.Eventually(func() error { return c.Get(context.TODO(), objKey, obj) }, timeout).
		Should(gomega.Succeed())
}

func clean(g *gomega.GomegaWithT, obj runtime.Object) {
	err := c.Delete(context.TODO(), obj)
	// If the resource is already deleted, we don't care, but any other error is important
	if !apierrors.IsNotFound(err) {
		g.Expect(err).NotTo(gomega.HaveOccurred())
	}
}
