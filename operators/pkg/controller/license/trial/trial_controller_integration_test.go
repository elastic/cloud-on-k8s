// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package trial

import (
	"fmt"
	"testing"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/elastic/k8s-operators/operators/pkg/utils/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var c k8s.Client

var licenseKey = types.NamespacedName{Name: "foo", Namespace: "elastic-system"}
var expectedRequest = reconcile.Request{NamespacedName: licenseKey}

func validateStatus(
	t *testing.T,
	key types.NamespacedName,
	createdLicense *v1alpha1.EnterpriseLicense,
	expected v1alpha1.LicenseStatus,
) {
	// test trial initialisation on create
	test.RetryUntilSuccess(t, func() error {
		err := c.Get(key, createdLicense)
		if err != nil {
			return err
		}
		if createdLicense.Status.LicenseStatus != expected {
			return fmt.Errorf("expected %v license but was %v", expected, createdLicense.Status.LicenseStatus)
		}
		return nil
	})
}

func validateTrialDuration(t *testing.T, license v1alpha1.EnterpriseLicense, now time.Time, precision time.Duration) {
	startDelta := license.StartDate().Sub(now)
	assert.True(t, startDelta <= precision, "start date should be within %v, but was %v", precision, startDelta)
	endDelta := license.ExpiryDate().Sub(now.Add(30 * 24 * time.Hour))
	assert.True(t, endDelta <= precision, "end date should be within %v, but was %v", precision, endDelta)
}

func deleteTrial(c k8s.Client, trialLicense *v1alpha1.EnterpriseLicense) error {
	// Delete the trial license
	trialLicense.Finalizers = nil
	if err := c.Update(trialLicense); err != nil {
		return err
	}
	return c.Delete(trialLicense)
}

func TestReconcile(t *testing.T) {

	now := time.Now()

	trialLicense := &v1alpha1.EnterpriseLicense{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "elastic-system"},
		Spec: v1alpha1.EnterpriseLicenseSpec{
			Type: v1alpha1.LicenseTypeEnterpriseTrial,
		},
	}
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
	// Create the EnterpriseLicense object
	assert.NoError(t, c.Create(trialLicense.DeepCopy()))
	test.CheckReconcileCalled(t, requests, expectedRequest)

	var createdLicense v1alpha1.EnterpriseLicense
	// test trial initialisation on create
	validateStatus(t, licenseKey, &createdLicense, v1alpha1.LicenseStatusValid)
	validateTrialDuration(t, createdLicense, now, time.Second)

	// Delete the trial license
	require.NoError(t, deleteTrial(c, &createdLicense))
	// recreate it
	require.NoError(t, c.Create(trialLicense))
	test.CheckReconcileCalled(t, requests, expectedRequest)
	// expect an invalid license
	validateStatus(t, licenseKey, &createdLicense, v1alpha1.LicenseStatusInvalid)

	test.CheckReconcileCalled(t, requests, expectedRequest)
	// ClusterLicense should be GC'ed but can't be tested here
}
