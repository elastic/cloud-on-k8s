// +build integration

package license

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/test"
	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/kubernetes/staging/src/k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c client.Client

var expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}

func listClusterLicenses(t *testing.T, c client.Client) []v1alpha1.ClusterLicense {
	clusterLicenses := v1alpha1.ClusterLicenseList{}
	assert.NoError(t, c.List(context.TODO(), &client.ListOptions{}, &clusterLicenses))
	return clusterLicenses.Items
}

func TestReconcile(t *testing.T) {

	thirtyDays := 30 * 24 * time.Hour
	now := time.Now()
	startDate := now.Add(-thirtyDays)
	expiryDate := now.Add(thirtyDays)

	instance := &v1alpha1.EnterpriseLicense{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "elastic-system"},
		Spec: v1alpha1.EnterpriseLicenseSpec{
			LicenseMeta: v1alpha1.LicenseMeta{
				UID:                "test",
				ExpiryDateInMillis: test.ToMillis(expiryDate),
			},
			Type:         "enterprise",
			SignatureRef: v1.SecretReference{},
			ClusterLicenseSpecs: []v1alpha1.ClusterLicenseSpec{
				{
					LicenseMeta: v1alpha1.LicenseMeta{
						ExpiryDateInMillis: test.ToMillis(expiryDate),
						StartDateInMillis:  test.ToMillis(startDate),
					},
					Type:         v1alpha1.LicenseTypePlatinum,
					SignatureRef: v1.SecretReference{},
				},
			},
		},
	}

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(test.Config, manager.Options{})
	assert.NoError(t, err)
	c = mgr.GetClient()

	recFn, requests := SetupTestReconcile(newReconciler(mgr))
	assert.NoError(t, add(mgr, recFn))

	stopMgr, mgrStopped := StartTestManager(mgr, t)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Create the EnterpriseLicense object and expect the Reconcile and Deployment to be created
	err = c.Create(context.TODO(), instance)
	// The instance object may not be a valid object because it might be missing some required fields.
	// Please modify the instance object by adding required fields and then remove the following if statement.
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	assert.NoError(t, err)
	defer c.Delete(context.TODO(), instance)

	cluster := &v1alpha1.ElasticsearchCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: v1alpha1.ElasticsearchSpec{
			Version:          "7.0.0",
			SetVMMaxMapCount: false,
			Topologies: []v1alpha1.ElasticsearchTopologySpec{
				{
					NodeCount: 3,
				},
			},
		},
	}
	assert.NoError(t, c.Create(context.TODO(), cluster))
	test.CheckReconcileCalled(t, requests, expectedRequest)

	// test license assignment and ownership being triggered on cluster create
	test.RetryUntilSuccess(t, func() error {
		licenses := listClusterLicenses(t, c)
		numLicenses := len(licenses)
		if numLicenses != 1 {
			return fmt.Errorf("expected exactly 1 cluster license got %d", numLicenses)
		}
		owners := licenses[0].OwnerReferences
		if len(owners) != 1 {
			return fmt.Errorf("expected exactly 1 owner, got %d", len(owners))
		}

		ownerName := owners[0].Name
		ownerKind := owners[0].Kind
		expectedKind := "ElasticsearchCluster"
		if ownerName != cluster.Name || ownerKind != expectedKind {
			return fmt.Errorf("expected owner %s (%s), got %s (%s)", cluster.Name, expectedKind, ownerName, ownerKind)
		}
		return nil
	})

	// Delete the cluster and expect Reconcile to be called for cluster deletion
	test.DeleteIfExists(t, c, cluster)
	test.CheckReconcileCalled(t, requests, expectedRequest)
	// ClusterLicense should be GC'ed but can't be tested here
}

// purpose of this test is mostly to understand and document the delaying queue behaviour
// can be removed or skipped when it causes trouble in CI because they are non-deterministic
func TestDelayingQueueInvariants(t *testing.T) {
	item := types.NamespacedName{Name: "foo", Namespace: "bar"}
	tests := []struct {
		name                 string
		adds                 func(workqueue.DelayingInterface)
		expectedObservations int
		timeout              time.Duration
	}{
		{
			name: "single add",
			adds: func(q workqueue.DelayingInterface) {
				q.Add(item)
			},
			expectedObservations: 1,
			timeout:              10 * time.Millisecond,
		},
		{
			name: "deduplication",
			adds: func(q workqueue.DelayingInterface) {
				q.Add(item)
				q.Add(item)
			},
			expectedObservations: 1,
			timeout:              500 * time.Millisecond,
		},
		{
			name: "no dedup'ing when delaying",
			adds: func(q workqueue.DelayingInterface) {
				q.Add(item)
				q.AddAfter(item, 1*time.Millisecond)
			},
			expectedObservations: 2,
			timeout:              10 * time.Millisecond,
		},
		{
			name: "but dedup's and updates item within the wait queue",
			adds: func(q workqueue.DelayingInterface) {
				q.AddAfter(item, 1*time.Hour)
				q.AddAfter(item, 1*time.Millisecond)
			},
			expectedObservations: 1,
			timeout:              10 * time.Millisecond,
		},
		{
			name: "direct add and delayed add are independent",
			adds: func(q workqueue.DelayingInterface) {
				q.AddAfter(item, 10*time.Millisecond)
				q.Add(item) // should work despite one item in the work queu
			},
			expectedObservations: 2,
			timeout:              20 * time.Millisecond,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := workqueue.NewDelayingQueue()
			tt.adds(q)
			results := make(chan int)
			var seen int
			go func() {
				for {
					item, _ := q.Get()
					results <- 1
					q.Done(item)
				}
			}()
			collect := func() {
				for {
					select {
					case r := <-results:
						seen += r
					case <-time.After(tt.timeout):
						return
					}
				}

			}
			collect()
			assert.Equal(t, tt.expectedObservations, seen)
		})
	}

}
