// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build mixed || e2e

package helper

import (
	"context"
	"fmt"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
)

// OperatorLeader returns the identity of the holder of the operator leader election Lease, and whether the Lease is
// live, i.e. it was renewed within its duration. The identity is unique to an operator process: a restarted operator
// acquires the Lease with a new identity, and an operator process exits when it loses the Lease. An unchanged identity
// between two points in time therefore means that a single operator process was the leader, and the only one
// reconciling resources, during that period. As the operator does not release the Lease when it exits, the Lease keeps
// the identity of a terminated leader until another operator acquires it, in which case it is not live.
func OperatorLeader(ctx context.Context, k *test.K8sClient) (string, bool, error) {
	var lease coordinationv1.Lease
	if err := k.Client.Get(ctx, types.NamespacedName{Namespace: test.Ctx().Operator.Namespace, Name: operator.LeaderElectionLeaseName}, &lease); err != nil {
		return "", false, err
	}
	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity == "" {
		return "", false, fmt.Errorf("operator leader election Lease %s has no holder", operator.LeaderElectionLeaseName)
	}
	live := lease.Spec.RenewTime != nil && lease.Spec.LeaseDurationSeconds != nil &&
		time.Now().Before(lease.Spec.RenewTime.Add(time.Duration(*lease.Spec.LeaseDurationSeconds)*time.Second))
	return *lease.Spec.HolderIdentity, live, nil
}
