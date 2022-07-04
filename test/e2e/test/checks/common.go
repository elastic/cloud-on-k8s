// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package checks

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

// CheckDeployment checks the Deployment resource exists
func CheckDeployment(subj test.Subject, k *test.K8sClient, deploymentName string) test.Step {
	return test.Step{
		Name: subj.Kind() + " deployment should be created",
		Test: test.Eventually(func() error {
			var dep appsv1.Deployment
			err := k.Client.Get(context.Background(), types.NamespacedName{
				Namespace: subj.NSN().Namespace,
				Name:      deploymentName,
			}, &dep)
			if subj.Count() == 0 && apierrors.IsNotFound(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if *dep.Spec.Replicas != subj.Count() {
				return fmt.Errorf("invalid %s replicas count: expected %d, got %d", subj.Kind(), subj.Count(), *dep.Spec.Replicas)
			}
			return nil
		}),
	}
}

// CheckPods checks that the test subject's expected pods are eventually ready.
func CheckPods(subj test.Subject, k *test.K8sClient) test.Step {
	// This is a shared test but it is common for Enterprise Search Pods to take some time to be ready, especially
	// during the initial bootstrap, or during version upgrades. Let's increase the timeout
	// for this particular step.
	timeout := test.Ctx().TestTimeout * 2
	return test.Step{
		Name: subj.Kind() + " Pods should eventually be ready",
		Test: test.UntilSuccess(func() error {
			var pods corev1.PodList
			if err := k.Client.List(context.Background(), &pods, subj.ListOptions()...); err != nil {
				return err
			}

			// builder hash matches
			expectedBuilderHash := hash.HashObject(subj.Spec())
			for _, pod := range pods.Items {
				if err := test.ValidateBuilderHashAnnotation(pod, expectedBuilderHash); err != nil {
					return err
				}
			}

			// pod count matches
			if len(pods.Items) != int(subj.Count()) {
				return fmt.Errorf("invalid %s pod count: expected %d, got %d", subj.NSN().Name, subj.Count(), len(pods.Items))
			}

			// pods are running
			for _, pod := range pods.Items {
				if pod.Status.Phase != corev1.PodRunning {
					return fmt.Errorf("pod not running yet")
				}
			}

			// pods are ready
			for _, pod := range pods.Items {
				if !k8s.IsPodReady(pod) {
					return fmt.Errorf("pod not ready yet")
				}
			}

			return nil
		}, timeout),
	}
}

// CheckServices checks that all expected services have been created
func CheckServices(subj test.Subject, k *test.K8sClient) test.Step {
	return test.Step{
		Name: subj.Kind() + " services should be created",
		Test: test.Eventually(func() error {
			for _, s := range []string{
				subj.ServiceName(),
			} {
				if _, err := k.GetService(subj.NSN().Namespace, s); err != nil {
					return err
				}
			}
			return nil
		}),
	}
}

// CheckServicesEndpoints checks that services have the expected number of endpoints
func CheckServicesEndpoints(subj test.Subject, k *test.K8sClient) test.Step {
	return test.Step{
		Name: subj.Kind() + " services should have endpoints",
		Test: test.Eventually(func() error {
			for endpointName, addrCount := range map[string]int{
				subj.ServiceName(): int(subj.Count()),
			} {
				if addrCount == 0 {
					continue // maybe no test resource in this builder
				}
				endpoints, err := k.GetEndpoints(subj.NSN().Namespace, endpointName)
				if err != nil {
					return err
				}
				if len(endpoints.Subsets) == 0 {
					return fmt.Errorf("no subset for endpoint %s", endpointName)
				}
				if len(endpoints.Subsets[0].Addresses) != addrCount {
					return fmt.Errorf("%d addresses found for endpoint %s, expected %d", len(endpoints.Subsets[0].Addresses), endpointName, addrCount)
				}
			}
			return nil
		}),
	}
}
