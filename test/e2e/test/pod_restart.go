// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// PodRestartChecker records pod UIDs at a point in time and later verifies that all of them have
// been replaced, confirming the matched pods restarted. It is useful when a restart is expected as
// a side-effect of something other than a direct spec change on those pods — for example, when
// mutating builder A should cause builder B's pods to roll. Use RecordUIDs in PreMutationSteps and
// WaitForRestart in PostMutationSteps to assert the restart happened.
type PodRestartChecker struct {
	name     string
	prevUIDs map[types.UID]struct{}
	listOpts []k8sclient.ListOption
}

// NewPodRestartChecker returns a checker scoped to the pods matched by opts.
// name labels the pods in step descriptions (e.g. "Agent", "Kibana").
func NewPodRestartChecker(name string, opts ...k8sclient.ListOption) *PodRestartChecker {
	return &PodRestartChecker{
		name:     name,
		listOpts: opts,
	}
}

// RecordUIDs returns a step that snapshots the UIDs of all currently running pods.
func (c *PodRestartChecker) RecordUIDs(k *K8sClient) StepList {
	return StepList{
		{
			Name: fmt.Sprintf("Record %s pod UIDs before mutation", c.name),
			Test: Eventually(func() error {
				var pods corev1.PodList
				if err := k.Client.List(context.Background(), &pods, c.listOpts...); err != nil {
					return err
				}
				if len(pods.Items) == 0 {
					return fmt.Errorf("no %s pods found", c.name)
				}
				if c.prevUIDs == nil {
					c.prevUIDs = make(map[types.UID]struct{}, len(pods.Items))
				} else {
					clear(c.prevUIDs)
				}
				for _, pod := range pods.Items {
					c.prevUIDs[pod.UID] = struct{}{}
				}
				return nil
			}),
		},
	}
}

// WaitForRestart returns a step that polls until none of the previously recorded pods are still
// running.
func (c *PodRestartChecker) WaitForRestart(k *K8sClient) StepList {
	return StepList{
		{
			Name: fmt.Sprintf("Wait for %s pods to restart", c.name),
			Test: Eventually(func() error {
				var pods corev1.PodList
				if err := k.Client.List(context.Background(), &pods, c.listOpts...); err != nil {
					return err
				}
				for _, pod := range pods.Items {
					if _, ok := c.prevUIDs[pod.UID]; ok {
						return fmt.Errorf("%s pod %s (uid=%s) has not been restarted yet", c.name, pod.Name, pod.UID)
					}
				}
				return nil
			}),
		},
	}
}
