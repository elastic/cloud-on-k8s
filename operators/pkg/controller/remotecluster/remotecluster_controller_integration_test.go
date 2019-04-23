// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package remotecluster

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/elastic/k8s-operators/operators/pkg/utils/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	c                     k8s.Client
	key                   = types.NamespacedName{Name: "remotecluster-sample-1-2", Namespace: "default"}
	trustRelationship1Key = types.NamespacedName{Name: "rc-remotecluster-sample-1-2", Namespace: "default"}
	trustRelationship2Key = types.NamespacedName{Name: "rcr-remotecluster-sample-1-2-default", Namespace: "default"}
)

var expectedRequest = reconcile.Request{NamespacedName: key}

const timeout = time.Second * 5

func TestReconcile(t *testing.T) {

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(test.Config, manager.Options{})
	client := mgr.GetClient()
	c = k8s.WrapClient(client)
	rec := newReconciler(mgr, "default")
	require.NoError(t, err)
	recFn, requests := SetupTestReconcile(rec)
	controller, err := add(mgr, recFn)
	assert.NoError(t, err)
	assert.NoError(t, addWatches(controller, rec))

	stopMgr, mgrStopped := StartTestManager(mgr, t)
	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	instance := newRemoteInCluster(
		"remotecluster-sample-1-2",
		"default", "trust-one-es",
		"default", "trust-two-es",
	)

	ca1 := newCASecret("default", "trust-one-es-ca", ca1)
	assert.NoError(t, c.Create(ca1))
	ca2 := newCASecret("default", "trust-two-es-ca", ca2)
	assert.NoError(t, c.Create(ca2))

	// Create the RemoteCluster object and expect the Reconcile and Deployment to be created
	err = c.Create(instance)
	if apierrors.IsInvalid(err) {
		t.Logf("failed to create object, got an invalid object error: %v", err)
		return
	}
	assert.NoError(t, err)
	defer c.Delete(instance)
	test.CheckReconcileCalled(t, requests, expectedRequest)

	trustRelationship1 := &v1alpha1.TrustRelationship{}
	// commercial features disabled
	assert.Error(t, c.Get(trustRelationship1Key, trustRelationship1))
	test.StartTrial(t, c)

	// looks like we need at least two interations to do the actual reconciling
	test.CheckReconcileCalledIn(t, requests, expectedRequest, 2, 2)
	// expect the creation of the first TrustRelationship
	test.RetryUntilSuccess(t, func() error {
		err := c.Get(trustRelationship1Key, trustRelationship1)
		if err != nil {
			return err
		}
		switch {
		case len(trustRelationship1.Spec.CaCert) == 0:
			return errors.New("Not reconciled yet")
		default:
			return nil
		}
	})

	// expect the creation of the second TrustRelationship
	trustRelationship2 := &v1alpha1.TrustRelationship{}
	test.RetryUntilSuccess(t, func() error {
		err := c.Get(trustRelationship2Key, trustRelationship2)
		if err != nil {
			return err
		}
		switch {
		case len(trustRelationship1.Spec.CaCert) == 0:
			return errors.New("Not reconciled yet")
		default:
			return nil
		}
	})

	// Check if state is PROPAGATED
	test.RetryUntilSuccess(t, func() error {
		fetched := v1alpha1.RemoteCluster{}
		err := c.Get(key, &fetched)
		if err != nil {
			return err
		}
		if v1alpha1.RemoteClusterPropagated != fetched.Status.State {
			return fmt.Errorf("expected %v, found %v", v1alpha1.RemoteClusterPropagated, fetched.Status.State)
		}
		return nil
	})

	// Delete one of the CA
	test.DeleteIfExists(t, c, ca1)

	// Ensure association goes back to pending if one of the CA is deleted.
	test.CheckReconcileCalled(t, requests, expectedRequest)
	test.CheckReconcileCalled(t, requests, expectedRequest)
	test.RetryUntilSuccess(t, func() error {
		fetched := v1alpha1.RemoteCluster{}
		err := c.Get(key, &fetched)
		if err != nil {
			return err
		}
		if v1alpha1.RemoteClusterPending != fetched.Status.State {
			return fmt.Errorf("expected %v, found %v", v1alpha1.RemoteClusterPending, fetched.Status.State)
		}
		return nil
	})

	test.DeleteIfExists(t, c, instance)
}
