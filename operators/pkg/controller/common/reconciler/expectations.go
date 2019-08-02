// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package reconciler

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// TODO: garbage collect/finalize deprecated UIDs
type Expectations struct {
	generations map[types.UID]int64
}

func NewExpectations() *Expectations {
	return &Expectations{
		generations: make(map[types.UID]int64),
	}
}

func (e *Expectations) ExpectGeneration(meta metav1.ObjectMeta) {
	e.generations[meta.UID] = meta.Generation
}

func (e *Expectations) GenerationExpected(metaObjs ...metav1.ObjectMeta) bool {
	for _, meta := range metaObjs {
		if expectedGen, exists := e.generations[meta.UID]; exists && meta.Generation < expectedGen {
			return false
		}
	}
	return true
}
