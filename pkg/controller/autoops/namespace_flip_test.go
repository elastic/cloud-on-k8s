// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	cachemock "github.com/elastic/cloud-on-k8s/v3/pkg/utils/test/mock"
)

func Test_namespaceFlipRequests(t *testing.T) {
	policy := func(name, namespace string) autoopsv1alpha1.AutoOpsAgentPolicy {
		return autoopsv1alpha1.AutoOpsAgentPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		}
	}
	req := func(namespace, name string) reconcile.Request {
		return reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: name}}
	}

	// Policies select Elasticsearch clusters cluster-wide by label, so a flip of any
	// namespace can change any policy's set of matching clusters: all policies are
	// re-enqueued, wherever they live.
	tests := []struct {
		name     string
		policies []autoopsv1alpha1.AutoOpsAgentPolicy
		want     []reconcile.Request
	}{
		{
			name: "all policies are enqueued, both in and outside the flipped namespace",
			policies: []autoopsv1alpha1.AutoOpsAgentPolicy{
				policy("local", "flipped"),
				policy("elsewhere", "scoped"),
			},
			want: []reconcile.Request{
				req("flipped", "local"),
				req("scoped", "elsewhere"),
			},
		},
		{
			name: "no policies",
			want: []reconcile.Request{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := cachemock.NewCache(t)
			c.On("List", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				list := args.Get(1).(*autoopsv1alpha1.AutoOpsAgentPolicyList) //nolint:forcetypeassert
				list.Items = append(list.Items, tt.policies...)
			}).Return(nil)

			reqs := namespaceFlipRequests(logr.Discard(), c)(
				context.Background(),
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "flipped"}},
			)

			require.ElementsMatch(t, tt.want, reqs)
		})
	}
}
