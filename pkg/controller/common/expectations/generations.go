// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package expectations

import (
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// ExpectedGenerations stores StatefulSets generations that are expected in the cache,
// following a StatefulSet update. It allows making sure we're not working with an
// out-of-date version of the StatefulSet resource we previously updated.
type ExpectedGenerations struct {
	client      k8s.Client
	generations map[types.NamespacedName]ResourceGeneration // per StatefulSet
}

// ResourceGeneration wraps UID and Generation for a given StatefulSet.
type ResourceGeneration struct {
	UID        types.UID
	Generation int64
}

// NewExpectedGenerations returns an initialized ExpectedGenerations.
func NewExpectedGenerations(client k8s.Client) *ExpectedGenerations {
	return &ExpectedGenerations{
		client:      client,
		generations: make(map[types.NamespacedName]ResourceGeneration),
	}
}

// ExpectGeneration markes the Generation of the given StatefulSets as expected.
func (e *ExpectedGenerations) ExpectGeneration(statefulSet appsv1.StatefulSet) {
	resource := types.NamespacedName{Namespace: statefulSet.Namespace, Name: statefulSet.Name}
	e.generations[resource] = ResourceGeneration{
		UID:        statefulSet.UID,
		Generation: statefulSet.Generation,
	}
}

// GenerationsSatisfied compares expected StatefulSets generations with the ones we have in the cache,
// and returns true if they all match.
// Expectations are cleared once they are matched.
func (e *ExpectedGenerations) GenerationsSatisfied() (bool, error) {
	for statefulSet, expectedGen := range e.generations {
		satisfied, err := e.generationSatisfied(statefulSet, expectedGen)
		if err != nil {
			return false, err
		}
		if !satisfied {
			return false, nil
		}
		// cache is up-to-date: remove the existing expectation
		delete(e.generations, statefulSet)
	}
	return true, nil
}

// generationSatisfied returns true if the generation of the cached StatefulSet matches what is expected.
func (e *ExpectedGenerations) generationSatisfied(statefulSet types.NamespacedName, expected ResourceGeneration) (bool, error) {
	var ssetInCache appsv1.StatefulSet
	err := e.client.Get(statefulSet, &ssetInCache)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// StatefulSet does not exist anymore
			return true, nil
		}
		return false, err
	}
	if ssetInCache.UID != expected.UID {
		// StatefulSet was replaced by another one with the same name
		return true, nil
	}
	if ssetInCache.Generation >= expected.Generation {
		// StatefulSet generation matches our expectations
		return true, nil
	}
	return false, nil
}

// GetGenerations returns the map of generations, for testing purposes mostly.
func (e *ExpectedGenerations) GetGenerations() map[types.NamespacedName]ResourceGeneration {
	return e.generations
}
