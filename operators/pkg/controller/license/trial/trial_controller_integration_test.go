// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package trial

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/test"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var licenseKey = types.NamespacedName{Name: "foo", Namespace: "elastic-system"}

func TestMain(m *testing.M) {
	test.RunWithK8s(m, filepath.Join("..", "..", "..", "..", "config", "crds"))
}

func TestReconcile(t *testing.T) {
	c, stop := test.StartManager(t, Add, operator.Parameters{})
	defer stop()

	now := time.Now()

	trialLicense := &v1alpha1.EnterpriseLicense{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "elastic-system"},
		Spec: v1alpha1.EnterpriseLicenseSpec{
			Type: v1alpha1.LicenseTypeEnterpriseTrial,
		},
	}

	// Create the EnterpriseLicense object
	require.NoError(t, c.Create(trialLicense.DeepCopy()))

	// license is invalid because we did not ack the Eula
	var createdLicense v1alpha1.EnterpriseLicense
	validateStatus(t, c, licenseKey, &createdLicense, v1alpha1.LicenseStatusInvalid)

	// accept EULA and update
	createdLicense.Spec.Eula.Accepted = true
	require.NoError(t, c.Update(&createdLicense))

	// test trial initialisation on create
	validateStatus(t, c, licenseKey, &createdLicense, v1alpha1.LicenseStatusValid)
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
	test.RetryUntilSuccess(t, func() error {
		require.NoError(t, c.Get(trialStatusKey, &trialStatus))
		if bytes.Equal(trialStatus.Data[license.TrialPubkeyKey], []byte("foobar")) {
			return errors.New("Manipulated secret has not been corrected")
		}
		return nil
	})

	// Delete the trial license
	require.NoError(t, deleteTrial(c))
	// recreate it
	require.NoError(t, c.Create(trialLicense))
	// expect an invalid license
	validateStatus(t, c, licenseKey, &createdLicense, v1alpha1.LicenseStatusInvalid)

	// ClusterLicense should be GC'ed but can't be tested here
}

func validateStatus(
	t *testing.T,
	c k8s.Client,
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
	startDelta := license.StartTime().Sub(now)
	assert.True(t, startDelta <= precision, "start date should be within %v, but was %v", precision, startDelta)
	endDelta := license.ExpiryDate().Sub(now.Add(30 * 24 * time.Hour))
	assert.True(t, endDelta <= precision, "end date should be within %v, but was %v", precision, endDelta)
}

func deleteTrial(c k8s.Client) error {
	var trialLicense v1alpha1.EnterpriseLicense
	if err := c.Get(licenseKey, &trialLicense); err != nil {
		return err
	}
	return c.Delete(&trialLicense)
}
