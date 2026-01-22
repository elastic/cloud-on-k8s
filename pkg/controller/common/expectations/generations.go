// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package expectations

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

// ExpectedGenerations stores resource generations that are expected in the cache,
// following a resource update. It allows making sure we're not working with an
// out-of-date version of a resource we previously updated.
type ExpectedGenerations struct {
	object      client.Object
	client      k8s.Client
	generations map[types.NamespacedName]ResourceGeneration
}

// ResourceGeneration wraps UID and Generation for a given resource.
type ResourceGeneration struct {
	UID        types.UID
	Generation int64
}

// NewExpectedGenerations returns an initialized ExpectedGenerations.
// The object parameter serves as a template for empty object creation, using DeepCopyObject() before retrieving objects from the API server.
func NewExpectedGenerations(client k8s.Client, object client.Object) *ExpectedGenerations {
	return &ExpectedGenerations{
		object:      object,
		client:      client,
		generations: make(map[types.NamespacedName]ResourceGeneration),
	}
}

// ExpectGeneration registers the Generation of the given object as expected.
// The object we receive as argument here is the "updated" resource.
// We expect to see its generation (at least) in PendingGenerations().
func (e *ExpectedGenerations) ExpectGeneration(object metav1.Object) {
	resource := types.NamespacedName{Namespace: object.GetNamespace(), Name: object.GetName()}
	e.generations[resource] = ResourceGeneration{
		UID:        object.GetUID(),
		Generation: object.GetGeneration(),
	}
}

// PendingGenerations compares expected resource generations with the ones we have in the cache,
// and returns the list of resources for which the generation has not been updated yet.
// Expectations are cleared once they are matched.
func (e *ExpectedGenerations) PendingGenerations() ([]string, error) {
	var pendingObjects []string
	for objectName, expectedGen := range e.generations {
		satisfied, err := e.generationSatisfied(objectName, expectedGen)
		if err != nil {
			return nil, err
		}
		if !satisfied {
			pendingObjects = append(pendingObjects, objectName.Name)
		} else {
			// cache is up-to-date: remove the existing expectation
			delete(e.generations, objectName)
		}
	}
	return pendingObjects, nil
}

// generationSatisfied returns true if the generation of the cached resource matches what is expected.
func (e *ExpectedGenerations) generationSatisfied(name types.NamespacedName, expected ResourceGeneration) (bool, error) {
	object, ok := e.object.DeepCopyObject().(client.Object)
	if !ok {
		return false, fmt.Errorf("unable to deep copy object of type %T", e.object)
	}
	err := e.client.Get(context.Background(), name, object)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Resource does not exist anymore
			return true, nil
		}
		return false, err
	}
	if object.GetUID() != expected.UID {
		// Resource was replaced by another one with the same name
		return true, nil
	}
	if object.GetGeneration() >= expected.Generation {
		// Resource generation matches our expectations
		return true, nil
	}
	return false, nil
}

// ObjectType returns the type of objects this ExpectedGenerations tracks.
func (e *ExpectedGenerations) ObjectType() string {
	return fmt.Sprintf("%T", e.object)
}

// GetGenerations returns the map of generations, for testing purposes mostly.
func (e *ExpectedGenerations) GetGenerations() map[types.NamespacedName]ResourceGeneration {
	return e.generations
}
