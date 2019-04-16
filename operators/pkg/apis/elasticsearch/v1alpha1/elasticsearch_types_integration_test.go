// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package v1alpha1

import (
	"testing"

	"github.com/go-test/deep"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestStorageElasticsearch(t *testing.T) {
	key := types.NamespacedName{
		Name:      "foo",
		Namespace: "default",
	}
	created := &Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: ElasticsearchSpec{
			Nodes: []NodeSpec{
				{
					NodeCount: 3,
				},
			},
		},
	}
	// Test Create
	fetched := &Elasticsearch{}
	require.NoError(t, c.Create(context.Background(), created))
	require.NoError(t, c.Get(context.Background(), key, fetched))

	if diff := deep.Equal(fetched, created); diff != nil {
		t.Error(diff)
	}

	// Test updating the configuration
	updated := fetched.DeepCopy()
	updated.Spec.Nodes[0].Config = &Config{Data: map[string]interface{}{"hello": "world"}}
	require.NoError(t, c.Update(context.Background(), updated))

	require.NoError(t, c.Get(context.Background(), key, fetched))
	if diff := deep.Equal(fetched, updated); diff != nil {
		t.Error(diff)
	}

	// Test Delete
	require.NoError(t, c.Delete(context.Background(), fetched))
	require.Error(t, c.Get(context.Background(), key, fetched))
}
