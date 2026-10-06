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
// WaitForRestart in PostMutationSteps to assert the restart happened, or CheckNotRestarted to assert it did not.
type PodRestartChecker struct {
	name string
	// prevUIDs maps the UIDs of the recorded pods to the restart count of each of their containers.
	prevUIDs map[types.UID]map[string]int32
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

// RecordUIDs returns a step that snapshots the UIDs of all currently running pods, and the restart count of their
// containers.
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
					c.prevUIDs = make(map[types.UID]map[string]int32, len(pods.Items))
				} else {
					clear(c.prevUIDs)
				}
				for _, pod := range pods.Items {
					c.prevUIDs[pod.UID] = containerRestartCounts(pod)
				}
				return nil
			}),
		},
	}
}

// WaitForRestart returns a step that polls until all previously recorded pods have been replaced
// and at least as many new pods (with different UIDs) are running.
func (c *PodRestartChecker) WaitForRestart(k *K8sClient) StepList {
	return StepList{
		{
			Name: fmt.Sprintf("Wait for %s pods to restart", c.name),
			Test: Eventually(func() error {
				var pods corev1.PodList
				if err := k.Client.List(context.Background(), &pods, c.listOpts...); err != nil {
					return err
				}
				var newPods int
				for _, pod := range pods.Items {
					if _, ok := c.prevUIDs[pod.UID]; ok {
						return fmt.Errorf("%s pod %s (uid=%s) has not been restarted yet", c.name, pod.Name, pod.UID)
					}
					newPods++
				}
				if newPods < len(c.prevUIDs) {
					return fmt.Errorf("%s pods restarting: %d/%d replacement pods running", c.name, newPods, len(c.prevUIDs))
				}
				return nil
			}),
		},
	}
}

// CheckNotRestarted returns an error if one of the previously recorded pods has been replaced, or if one of their
// containers has restarted since they were recorded.
func (c *PodRestartChecker) CheckNotRestarted(ctx context.Context, k *K8sClient) error {
	var pods corev1.PodList
	if err := k.Client.List(ctx, &pods, c.listOpts...); err != nil {
		return err
	}
	if len(pods.Items) != len(c.prevUIDs) {
		return fmt.Errorf("expected %d %s pods, got %d", len(c.prevUIDs), c.name, len(pods.Items))
	}
	for _, pod := range pods.Items {
		prevRestartCounts, ok := c.prevUIDs[pod.UID]
		if !ok {
			return fmt.Errorf("%s pod %s (uid=%s) has been restarted", c.name, pod.Name, pod.UID)
		}
		for container, restartCount := range containerRestartCounts(pod) {
			if restarts := restartCount - prevRestartCounts[container]; restarts > 0 {
				return fmt.Errorf("container %s of %s pod %s restarted %d times", container, c.name, pod.Name, restarts)
			}
		}
	}
	return nil
}

// containerRestartCounts returns the restart count of each container of the given pod.
func containerRestartCounts(pod corev1.Pod) map[string]int32 {
	restartCounts := make(map[string]int32, len(pod.Status.ContainerStatuses))
	for _, status := range pod.Status.ContainerStatuses {
		restartCounts[status.Name] = status.RestartCount
	}
	return restartCounts
}
