// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package remotecluster

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	cachemock "github.com/elastic/cloud-on-k8s/v3/pkg/utils/test/mock"
)

func Test_namespaceFlipRequests(t *testing.T) {
	es := func(name, namespace string, remoteRefs ...commonv1.LocalObjectSelector) esv1.Elasticsearch {
		remoteClusters := make([]esv1.RemoteCluster, 0, len(remoteRefs))
		for _, ref := range remoteRefs {
			remoteClusters = append(remoteClusters, esv1.RemoteCluster{Name: ref.Name, ElasticsearchRef: ref})
		}
		return esv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec:       esv1.ElasticsearchSpec{RemoteClusters: remoteClusters},
		}
	}
	ref := func(name, namespace string) commonv1.LocalObjectSelector {
		return commonv1.LocalObjectSelector{Name: name, Namespace: namespace}
	}
	req := func(namespace, name string) reconcile.Request {
		return reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: name}}
	}

	// "flipped" is the namespace whose match state just changed, "scoped" is a namespace
	// that remains in scope.
	tests := []struct {
		name     string
		clusters []esv1.Elasticsearch
		want     []reconcile.Request
	}{
		{
			name: "case A: clusters living in the flipped namespace are enqueued regardless of refs",
			clusters: []esv1.Elasticsearch{
				es("local", "flipped"),
				es("standalone", "scoped"),
			},
			want: []reconcile.Request{
				req("flipped", "local"),
			},
		},
		{
			// A cluster declaring counterparts necessarily lives in the flipped namespace,
			// so its own case A request is always part of the result.
			name: "case B: counterparts declared by the flipped namespace's clusters are enqueued wherever they live",
			clusters: []esv1.Elasticsearch{
				es("local", "flipped",
					ref("counterpart", "scoped"),
					// implicit same-namespace ref
					ref("sibling", ""),
					// unset ref (e.g. remote cluster outside this k8s cluster): skipped
					ref("", ""),
				),
			},
			want: []reconcile.Request{
				req("flipped", "local"),
				req("scoped", "counterpart"),
				req("flipped", "sibling"),
			},
		},
		{
			name: "case C: clusters elsewhere are enqueued only if they reference into the flipped namespace",
			clusters: []esv1.Elasticsearch{
				es("cross-ref", "scoped", ref("es", "flipped")),
				// references a cluster in an unrelated namespace: not enqueued
				es("unrelated", "scoped", ref("es", "elsewhere")),
				// an unset ref namespace defaults to the cluster's own namespace: not enqueued
				es("self-ref", "scoped", ref("es", "")),
				es("standalone", "scoped"),
			},
			want: []reconcile.Request{
				req("scoped", "cross-ref"),
			},
		},
		{
			name: "mixed: overlapping cases produce each request exactly once",
			clusters: []esv1.Elasticsearch{
				// case A, and case B declaring "b" (also a case C match) and "a2"
				// (also a case A match) via an implicit same-namespace ref
				es("a", "flipped", ref("b", "scoped"), ref("a2", "")),
				// case A, deduped with a's counterpart ref
				es("a2", "flipped"),
				// case C, deduped with a's counterpart ref
				es("b", "scoped", ref("a", "flipped")),
				es("unrelated", "scoped", ref("es", "elsewhere")),
			},
			want: []reconcile.Request{
				req("flipped", "a"),
				req("flipped", "a2"),
				req("scoped", "b"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := cachemock.NewCache(t)
			c.On("List", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				list := args.Get(1).(*esv1.ElasticsearchList) //nolint:forcetypeassert
				list.Items = append(list.Items, tt.clusters...)
			}).Return(nil)

			reqs := namespaceFlipRequests(c)(
				context.Background(),
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "flipped"}},
			)

			require.ElementsMatch(t, tt.want, reqs)
		})
	}
}
