// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package remotecluster

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/license"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/test"
)

var (
	key                   = types.NamespacedName{Name: "remotecluster-sample-1-2", Namespace: "default"}
	trustRelationship1Key = types.NamespacedName{Name: "rc-remotecluster-sample-1-2", Namespace: "default"}
	trustRelationship2Key = types.NamespacedName{Name: "rcr-remotecluster-sample-1-2-default", Namespace: "default"}
)

func TestMain(m *testing.M) {
	test.RunWithK8s(m, filepath.Join("..", "..", "..", "config", "crds"))
}

func StartTrial(t *testing.T, c k8s.Client, namespace string) {
	require.NoError(t, license.CreateTrialLicense(c, namespace))
	trialKey := types.NamespacedName{Namespace: namespace, Name: string(license.LicenseTypeEnterpriseTrial)}
	var el license.EnterpriseLicense
	var secret v1.Secret
	test.RetryUntilSuccess(t, func() error {
		s, l, err := license.TrialLicense(c, trialKey)
		el = l
		secret = s
		return err
	})
	_, err := license.InitTrial(c, secret, &el)
	require.NoError(t, err)
}

func TestReconcile(t *testing.T) {
	operatorNamespace := "operator-namespace"

	c, stop := test.StartManager(t, Add, operator.Parameters{
		OperatorNamespace: operatorNamespace, // trial license will be installed in that namespace
	})
	defer stop()

	instance := newRemoteInCluster(
		"remotecluster-sample-1-2",
		"default", "trust-one-es",
		"default", "trust-two-es",
	)

	ca1 := newCASecret("default", "trust-one-es-es-transport-certs-public", ca1)
	require.NoError(t, c.Create(ca1))
	ca2 := newCASecret("default", "trust-two-es-es-transport-certs-public", ca2)

	require.NoError(t, c.Create(ca2))

	// Create the RemoteCluster object
	require.NoError(t, c.Create(instance))

	// commercial features disabled: no trust relationship should be created
	test.RetryUntilSuccess(t, func() error {
		// status should be updated accordingly
		var remoteCluster v1alpha1.RemoteCluster
		if err := c.Get(key, &remoteCluster); err != nil {
			return err
		}
		if remoteCluster.Status.Phase != v1alpha1.RemoteClusterFeatureDisabled {
			return errors.New("remote cluster status phase not set to disabled yet")
		}
		// there should be no trust relationship created
		trustRelationship1 := &v1alpha1.TrustRelationship{}
		require.True(t, apierrors.IsNotFound(c.Get(trustRelationship1Key, trustRelationship1)))
		return nil
	})

	// start a trial license to enable the feature
	StartTrial(t, c, operatorNamespace)

	// expect the creation of the first TrustRelationship
	trustRelationship1 := &v1alpha1.TrustRelationship{}
	test.RetryUntilSuccess(t, func() error {
		err := c.Get(trustRelationship1Key, trustRelationship1)
		if err != nil {
			return err
		}
		if len(trustRelationship1.Spec.CaCert) == 0 {
			return errors.New("no ca cert reconciled yet")
		}
		return nil
	})

	// expect the creation of the second TrustRelationship
	trustRelationship2 := &v1alpha1.TrustRelationship{}
	test.RetryUntilSuccess(t, func() error {
		err := c.Get(trustRelationship2Key, trustRelationship2)
		if err != nil {
			return err
		}
		if len(trustRelationship2.Spec.CaCert) == 0 {
			return errors.New("no ca cert reconciled yet")
		}
		return nil
	})

	// Check if state is PROPAGATED
	test.RetryUntilSuccess(t, func() error {
		fetched := v1alpha1.RemoteCluster{}
		err := c.Get(key, &fetched)
		if err != nil {
			return err
		}
		if v1alpha1.RemoteClusterPropagated != fetched.Status.Phase {
			return fmt.Errorf("expected %v, found %v", v1alpha1.RemoteClusterPropagated, fetched.Status.Phase)
		}
		return nil
	})

	// Delete one of the CA
	require.NoError(t, c.Delete(ca1))

	// Ensure association goes back to pending if one of the CA is deleted.
	test.RetryUntilSuccess(t, func() error {
		fetched := v1alpha1.RemoteCluster{}
		err := c.Get(key, &fetched)
		if err != nil {
			return err
		}
		if v1alpha1.RemoteClusterPending != fetched.Status.Phase {
			return fmt.Errorf("expected %v, found %v", v1alpha1.RemoteClusterPending, fetched.Status.Phase)
		}
		return nil
	})
}
