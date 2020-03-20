// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package watches

// NewDynamicWatches creates an initialized DynamicWatches container.
func NewDynamicWatches() DynamicWatches {
	return DynamicWatches{
		Secrets:               NewDynamicEnqueueRequest(),
		Pods:                  NewDynamicEnqueueRequest(),
		ElasticsearchClusters: NewDynamicEnqueueRequest(),
		Kibanas:               NewDynamicEnqueueRequest(),
	}
}

// DynamicWatches contains stateful dynamic watches. Intended as facility to pass around stateful dynamic watches and
// give each of them an identity.
type DynamicWatches struct {
	Secrets               *DynamicEnqueueRequest
	Pods                  *DynamicEnqueueRequest
	ElasticsearchClusters *DynamicEnqueueRequest
	Kibanas               *DynamicEnqueueRequest
}
