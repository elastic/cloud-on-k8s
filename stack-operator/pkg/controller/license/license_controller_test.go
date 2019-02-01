package license

import (
	"fmt"
	"testing"
	"time"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/test"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
			UID:                "test",
			Type:               "enterprise",
			ExpiryDateInMillis: test.ToMillis(expiryDate),
			SignatureRef:       v1.SecretReference{},
			ClusterLicenses: []v1alpha1.ClusterLicense{
				{
					Spec: v1alpha1.ClusterLicenseSpec{
						Type:               v1alpha1.LicenseTypePlatinum,
						ExpiryDateInMillis: test.ToMillis(expiryDate),
						StartDateInMillis:  test.ToMillis(startDate),
						SignatureRef:       v1.SecretReference{},
					},
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

	test.RetryUntilSuccess(t, func() error {
		numLicenses := len(listClusterLicenses(t, c))
		if numLicenses != 1 {
			return fmt.Errorf("expected exactly 1 cluster license got %d", numLicenses)
		}
		return nil
	})

	// Delete the cluster and expect Reconcile to be called for cluster deletion
	test.DeleteIfExists(t, c, cluster)
	test.CheckReconcileCalled(t, requests, expectedRequest)
	// ClusterLicense should be GC'ed but can't be tested here
}
