// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package generation

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

func getGeneration(obj client.Object, k *test.K8sClient) (int64, error) {
	if err := k.Client.Get(context.Background(), types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, obj); err != nil {
		return 0, err
	}

	return obj.GetGeneration(), nil
}

func getObservedGeneration(obj commonv1.HasObservedGeneration, k *test.K8sClient) (int64, error) {
	if err := k.Client.Get(context.Background(), types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, obj); err != nil {
		return 0, err
	}

	return obj.GetObservedGeneration(), nil
}

// RetrieveGenerationsStep stores the current metadata.Generation, and status.ObservedGeneration into the given fields.
func RetrieveGenerationsStep(obj commonv1.HasObservedGeneration, k *test.K8sClient, generation, observedGeneration *int64) test.Step {
	return test.Step{
		Name: "Retrieve Objects metadata.Generation, and status.ObservedGeneration for comparison purpose",
		Test: test.Eventually(func() error {
			objGeneration, err := getGeneration(obj, k)
			if err != nil {
				return err
			}
			*generation = objGeneration
			objObservedGeneration, err := getObservedGeneration(obj, k)
			if err != nil {
				return err
			}
			*observedGeneration = objObservedGeneration
			return nil
		}),
	}
}

// CompareObjectGenerationsStep compares the current object's metadata.generation, and status.observedGeneration
// and fails if they don't match expectations.
func CompareObjectGenerationsStep(
	obj commonv1.HasObservedGeneration,
	k *test.K8sClient,
	isMutated bool,
	previousObjectGeneration, previousObjectObservedGeneration int64) test.Step {
	return test.Step{
		Name: "metadata.generation and status.observedGeneration may have been incremented from previous state, and should be equal",
		Test: test.Eventually(func() error {
			newObjectGeneration, err := getGeneration(obj, k)
			if err != nil {
				return err
			}
			if newObjectGeneration == 0 {
				return errors.New("expected object's metadata.generation to not be empty")
			}
			newObjectObservedGeneration, err := getObservedGeneration(obj, k)
			if err != nil {
				return err
			}
			if newObjectObservedGeneration == 0 {
				return errors.New("expected object's status.observedGeneration to not be empty")
			}
			if isMutated && newObjectGeneration <= previousObjectGeneration {
				return errors.New("expected object's metadata.generation to be incremented")
			}
			if isMutated && newObjectObservedGeneration <= previousObjectObservedGeneration {
				return errors.New("expected object's status.observedGeneration to be incremented")
			}
			if newObjectGeneration != newObjectObservedGeneration {
				return fmt.Errorf("expected object's metadata.generation and status.observedGeneration to be equal; generation: %d, observedGeneration: %d", newObjectGeneration, newObjectObservedGeneration)
			}
			return nil
		}),
	}
}
