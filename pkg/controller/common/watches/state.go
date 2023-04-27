// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package watches

// NewDynamicWatches creates an initialized DynamicWatches container.
func NewDynamicWatches() DynamicWatches {
	return DynamicWatches{
		ConfigMaps:          NewDynamicEnqueueRequest(),
		Secrets:             NewDynamicEnqueueRequest(),
		Services:            NewDynamicEnqueueRequest(),
		Pods:                NewDynamicEnqueueRequest(),
		ReferencedResources: NewDynamicEnqueueRequest(),
	}
}

// DynamicWatches contains stateful dynamic watches. Intended as facility to pass around stateful dynamic watches and
// give each of them an identity.
type DynamicWatches struct {
	ConfigMaps          *DynamicEnqueueRequest
	Secrets             *DynamicEnqueueRequest
	Services            *DynamicEnqueueRequest
	Pods                *DynamicEnqueueRequest
	ReferencedResources *DynamicEnqueueRequest
}
