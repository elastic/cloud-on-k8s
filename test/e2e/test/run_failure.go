// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RunRecoverableFailureScenario tests a failure scenario that is recoverable.
func RunRecoverableFailureScenario(t *testing.T, failureSteps StepsFunc, builders ...Builder) {
	t.Helper()
	runFailureScenario(t, true, failureSteps, builders...)
}

// RunUnrecoverableFailureScenario tests a failure scenario that is not recoverable.
func RunUnrecoverableFailureScenario(t *testing.T, failureSteps StepsFunc, builders ...Builder) {
	t.Helper()
	runFailureScenario(t, false, failureSteps, builders...)
}

func runFailureScenario(t *testing.T, recoverable bool, failureSteps StepsFunc, builders ...Builder) {
	t.Helper()
	skipIfIncompatibleBuilders(t, builders...)
	k := NewK8sClientOrFatal()

	steps := StepList{}

	for _, b := range builders {
		steps = steps.WithSteps(b.InitTestSteps(k))
	}
	for _, b := range builders {
		steps = steps.WithSteps(b.CreationTestSteps(k))
	}
	for _, b := range builders {
		steps = steps.WithSteps(CheckTestSteps(b, k))
	}

	// Trigger the failure
	steps = steps.WithSteps(failureSteps(k))

	if recoverable {
		// Check we recover
		for _, b := range builders {
			steps = steps.WithSteps(CheckTestSteps(b, k))
		}
	}

	for _, b := range builders {
		steps = steps.WithSteps(b.DeletionTestSteps(k))
	}

	steps.RunSequential(t)
}

func KillNodeSteps(podMatch func(p corev1.Pod) bool, opts ...client.ListOption) StepsFunc {
	var killedPod corev1.Pod
	//nolint:thelper
	return func(k *K8sClient) StepList {
		return StepList{
			{
				Name: "Kill a node",
				Test: func(t *testing.T) {
					pods, err := k.GetPods(opts...)
					require.NoError(t, err)
					var found bool
					killedPod, found = GetFirstPodMatching(pods, podMatch)
					require.True(t, found)
					err = k.DeletePod(killedPod)
					require.NoError(t, err)
				},
			},
			{
				Name: "Wait for pod to be deleted",
				Test: Eventually(func() error {
					pod, err := k.GetPod(killedPod.Namespace, killedPod.Name)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
					if apierrors.IsNotFound(err) || killedPod.UID != pod.UID {
						return nil
					}
					return fmt.Errorf("pod %s not deleted yet", killedPod.Name)
				}),
			},
		}
	}
}
