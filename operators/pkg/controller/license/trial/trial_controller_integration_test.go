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

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/chrono"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/test"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const operatorNs = "elastic-system"

func TestMain(m *testing.M) {
	test.RunWithK8s(m, filepath.Join("..", "..", "..", "..", "config", "crds"))
}

func TestReconcile(t *testing.T) {
	c, stop := test.StartManager(t, Add, operator.Parameters{
		OperatorNamespace: operatorNs,
		TrialMode:         true,
	})
	defer stop()

	now := time.Now()

	// Create trial initialisation is controlled via config
	checker := license.NewLicenseChecker(c, operatorNs)
	// test trial initialisation on create
	validateTrialStatus(t, checker, true)
	licenses, err := license.EnterpriseLicenseList(c)
	require.NoError(t, err)
	require.Equal(t, 1, len(licenses))
	trialLicense := licenses[0]
	require.True(t, trialLicense.IsTrial())

	validateTrialDuration(t, trialLicense, now, time.Minute)

	// tamper with the trial status
	var trialStatus corev1.Secret
	trialStatusKey := types.NamespacedName{
		Namespace: operatorNs,
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
	require.NoError(t, deleteTrial(c, trialLicense.Data.UID))
	// recreate it with modified validity + 1 year
	trialLicense.Data.ExpiryDateInMillis = chrono.ToMillis(time.Now().Add(12 * 30 * 24 * time.Hour))
	require.NoError(t, license.CreateEnterpriseLicense(c, types.NamespacedName{
		Namespace: operatorNs,
		Name:      trialLicense.Data.UID,
	}, trialLicense))
	// expect an invalid license
	validateTrialStatus(t, checker, false)
	// ClusterLicense should be GC'ed but can't be tested here
}

func validateTrialStatus(t *testing.T, checker *license.Checker, expected bool) {
	// test trial initialisation on create
	test.RetryUntilSuccess(t, func() error {
		trialEnabled, err := checker.EnterpriseFeaturesEnabled()
		if err != nil {
			return err
		}
		if trialEnabled != expected {
			return fmt.Errorf("expected licensed features to be enabled [%v] but was [%v]", expected, trialEnabled)
		}
		return nil
	})
}

func validateTrialDuration(t *testing.T, license license.SourceEnterpriseLicense, now time.Time, precision time.Duration) {
	startDelta := license.StartTime().Sub(now)
	assert.True(t, startDelta <= precision, "start date should be within %v, but was %v", precision, startDelta)
	endDelta := license.ExpiryTime().Sub(now.Add(30 * 24 * time.Hour))
	assert.True(t, endDelta <= precision, "end date should be within %v, but was %v", precision, endDelta)
}

func deleteTrial(c k8s.Client, name string) error {
	var trialLicense corev1.Secret
	if err := c.Get(types.NamespacedName{Namespace: operatorNs, Name: name}, &trialLicense); err != nil {
		return err
	}
	return c.Delete(&trialLicense)
}
