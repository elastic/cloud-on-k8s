// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package trial

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/test"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const operatorNs = "elastic-system"

var testLicenseNSN = types.NamespacedName{
	Namespace: operatorNs,
	Name:      "eck-trial-license",
}

func TestMain(m *testing.M) {
	test.RunWithK8s(m)
}

func TestReconcile(t *testing.T) {
	c, stop := test.StartManager(t, Add, operator.Parameters{OperatorNamespace: operatorNs})
	defer stop()

	now := time.Now()

	require.NoError(t, test.EnsureNamespace(c, operatorNs))

	// Create trial initialisation is controlled via config
	require.NoError(t, license.CreateTrialLicense(c, testLicenseNSN))
	checker := license.NewLicenseChecker(c, operatorNs)
	// test trial initialisation on create
	validateTrialStatus(t, checker, true)
	licenses, err := license.EnterpriseLicenses(c)
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
	// retry in case of edit conflict with reconciliation loop
	test.RetryUntilSuccess(t, func() error {
		require.NoError(t, c.Get(context.Background(), trialStatusKey, &trialStatus))
		trialStatus.Data[license.TrialPubkeyKey] = []byte("foobar")
		return c.Update(context.Background(), &trialStatus)
	})
	test.RetryUntilSuccess(t, func() error {
		require.NoError(t, c.Get(context.Background(), trialStatusKey, &trialStatus))
		if bytes.Equal(trialStatus.Data[license.TrialPubkeyKey], []byte("foobar")) {
			return errors.New("Manipulated secret has not been corrected")
		}
		return nil
	})

	// Delete the trial license
	require.NoError(t, deleteTrial(c))
	// recreate it with modified validity + 1 year
	require.NoError(t, license.CreateTrialLicense(c, testLicenseNSN))
	// expect an invalid license
	validateTrialStatus(t, checker, false)
	// ClusterLicense should be GC'ed but can't be tested here
}

func validateTrialStatus(t *testing.T, checker license.Checker, expected bool) {
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

func validateTrialDuration(t *testing.T, license license.EnterpriseLicense, now time.Time, precision time.Duration) {
	startDelta := license.StartTime().Sub(now)
	assert.True(t, startDelta <= precision, "start date should be within %v, but was %v", precision, startDelta)
	endDelta := license.ExpiryTime().Sub(now.Add(30 * 24 * time.Hour))
	assert.True(t, endDelta <= precision, "end date should be within %v, but was %v", precision, endDelta)
}

func deleteTrial(c k8s.Client) error {
	var trialLicense corev1.Secret
	if err := c.Get(context.Background(), testLicenseNSN, &trialLicense); err != nil {
		return err
	}
	return c.Delete(context.Background(), &trialLicense)
}
