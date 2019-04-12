// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build integration

package v1alpha1

import (
	"testing"

	"github.com/go-test/deep"
	"github.com/onsi/gomega"
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
					Config: Config{},
				},
			},
			SnapshotRepository: nil,
			FeatureFlags:       nil,
			UpdateStrategy:     UpdateStrategy{},
		},
	}
	g := gomega.NewGomegaWithT(t)

	// Test Create
	fetched := &Elasticsearch{}
	g.Expect(c.Create(context.Background(), created)).NotTo(gomega.HaveOccurred())

	g.Expect(c.Get(context.Background(), key, fetched)).NotTo(gomega.HaveOccurred())

	if diff := deep.Equal(fetched, created); diff != nil {
		t.Error(diff)
	}

	// Test Updating the Labels
	updated := fetched.DeepCopy()
	updated.Labels = map[string]string{"hello": "world"}
	g.Expect(c.Update(context.Background(), updated)).NotTo(gomega.HaveOccurred())

	g.Expect(c.Get(context.Background(), key, fetched)).NotTo(gomega.HaveOccurred())
	if diff := deep.Equal(fetched, updated); diff != nil {
		t.Error(diff)
	}

	// Test Delete
	g.Expect(c.Delete(context.Background(), fetched)).NotTo(gomega.HaveOccurred())
	g.Expect(c.Get(context.Background(), key, fetched)).To(gomega.HaveOccurred())
}
