// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package mutation

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/mutation/comparison"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/reconcile"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/pod"
)

// ReuseOptions specify the way we are allowed to mutate one pod to another
type ReuseOptions struct {
	// ReusePods indicates a pod can be reused on cluster mutations
	// (restart the ES process with a different config in a running pod)
	ReusePods bool
	// ReusePVCs indicates a PVC can be reused on cluster mutations
	// (replace a pod by a new one, but keep the same PVC & PV)
	ReusePVCs bool
}

// CanReuse returns true if one of the reuse method can be used
func (o ReuseOptions) CanReuse() bool {
	return o.ReusePods || o.ReusePVCs
}
