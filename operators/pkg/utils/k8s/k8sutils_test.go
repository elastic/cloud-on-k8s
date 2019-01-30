// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestToObjectMeta(t *testing.T) {
	assert.Equal(
		t,
		metav1.ObjectMeta{Namespace: "namespace", Name: "name"},
		ToObjectMeta(types.NamespacedName{Namespace: "namespace", Name: "name"}),
	)
}

func TestExtractNamespacedName(t *testing.T) {
	assert.Equal(
		t,
		types.NamespacedName{Namespace: "namespace", Name: "name"},
		ExtractNamespacedName(metav1.ObjectMeta{Namespace: "namespace", Name: "name"}),
	)
}
