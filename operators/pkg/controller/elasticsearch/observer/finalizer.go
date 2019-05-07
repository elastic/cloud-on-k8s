// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package observer

import (
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/finalizer"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// FinalizerName registered for each elasticsearch resource
	FinalizerName = "observer.finalizers.elasticsearch.k8s.elastic.co"
)

// Finalizer returns a finalizer to be executed upon deletion of the given cluster,
// that makes sure the cluster is not observed anymore
func (m *Manager) Finalizer(cluster types.NamespacedName) finalizer.Finalizer {
	return finalizer.Finalizer{
		Name: FinalizerName,
		Execute: func() error {
			m.StopObserving(cluster)
			return nil
		},
	}
}
