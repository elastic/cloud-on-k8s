// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package trial

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/license"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/elastic/k8s-operators/operators/pkg/utils/test"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
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
		if createdLicense.Status != expected {
			return fmt.Errorf("expected %v license but was %v", expected, createdLicense.Status)
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

func deleteTrial() error {
	var trialLicense v1alpha1.EnterpriseLicense
	if err := c.Get(licenseKey, &trialLicense); err != nil {
		return err
	}
	// Delete the trial license
	trialLicense.Finalizers = nil
	if err := c.Update(&trialLicense); err != nil {
		return err
	}
	return c.Delete(&trialLicense)
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
	// license is invalid because we did not ack the Eula
	test.CheckReconcileCalled(t, requests, expectedRequest)
	var createdLicense v1alpha1.EnterpriseLicense
	validateStatus(t, licenseKey, &createdLicense, v1alpha1.LicenseStatusInvalid)
	// accept EULA and update
	createdLicense.Spec.Eula.Accepted = true
	assert.NoError(t, c.Update(&createdLicense))

	// expecting 3 cycles: resource update, status update, noop because controller updates spec
	test.CheckReconcileCalledIn(t, requests, expectedRequest, 3, 3)

	// test trial initialisation on create
	validateStatus(t, licenseKey, &createdLicense, v1alpha1.LicenseStatusValid)
	validateTrialDuration(t, createdLicense, now, time.Minute)

	// tamper with the trial status
	var trialStatus corev1.Secret
	trialStatusKey := types.NamespacedName{
		Namespace: "elastic-system",
		Name:      license.TrialStatusSecretKey,
	}
	require.NoError(t, c.Get(trialStatusKey, &trialStatus))
	trialStatus.Data[license.TrialPubkeyKey] = []byte("foobar")
	require.NoError(t, c.Update(&trialStatus))
	test.CheckReconcileCalled(t, requests, expectedRequest)
	test.RetryUntilSuccess(t, func() error {
		require.NoError(t, c.Get(trialStatusKey, &trialStatus))
		if bytes.Equal(trialStatus.Data[license.TrialPubkeyKey], []byte("foobar")) {
			return errors.New("Manipulated secret has not been corrected")
		}
		return nil
	})

	// Delete the trial license
	require.NoError(t, deleteTrial())
	test.CheckReconcileCalled(t, requests, expectedRequest)
	// recreate it
	require.NoError(t, c.Create(trialLicense))
	test.CheckReconcileCalled(t, requests, expectedRequest)
	// expect an invalid license
	validateStatus(t, licenseKey, &createdLicense, v1alpha1.LicenseStatusInvalid)

	// ClusterLicense should be GC'ed but can't be tested here
}
