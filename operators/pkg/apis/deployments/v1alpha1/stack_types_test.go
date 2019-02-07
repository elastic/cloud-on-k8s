// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestStorageStack(t *testing.T) {
	key := types.NamespacedName{
		Name:      "foo",
		Namespace: "default",
	}
	created := &Stack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		}}

	// Test Create
	fetched := &Stack{}

	assert.NoError(t, c.Create(context.TODO(), created))

	assert.NoError(t, c.Get(context.TODO(), key, fetched))
	assert.Equal(t, created, fetched)

	// Test Updating the Labels
	updated := fetched.DeepCopy()
	updated.Labels = map[string]string{"hello": "world"}
	assert.NoError(t, c.Update(context.TODO(), updated))

	assert.NoError(t, c.Get(context.TODO(), key, fetched))
	assert.Equal(t, fetched, updated)

	// Test Delete
	assert.NoError(t, c.Delete(context.TODO(), fetched))
	assert.Error(t, c.Get(context.TODO(), key, fetched))
}
