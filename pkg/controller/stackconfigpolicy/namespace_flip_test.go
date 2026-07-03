// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package stackconfigpolicy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	policyv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/stackconfigpolicy/v1alpha1"
	cachemock "github.com/elastic/cloud-on-k8s/v3/pkg/utils/test/mock"
)

func Test_namespaceFlipRequests(t *testing.T) {
	policy := func(name, namespace string) policyv1alpha1.StackConfigPolicy {
		return policyv1alpha1.StackConfigPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		}
	}
	req := func(namespace, name string) reconcile.Request {
		return reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: name}}
	}

	// "flipped" is the namespace whose match state just changed, "scoped" is a namespace
	// that remains in scope, "operator-ns" is the operator namespace (always managed, so
	// it can never be the flipped one).
	tests := []struct {
		name     string
		policies []policyv1alpha1.StackConfigPolicy
		want     []reconcile.Request
	}{
		{
			name: "policies living in the flipped namespace are enqueued",
			policies: []policyv1alpha1.StackConfigPolicy{
				policy("local", "flipped"),
				policy("elsewhere", "scoped"),
			},
			want: []reconcile.Request{
				req("flipped", "local"),
			},
		},
		{
			name: "operator namespace policies are enqueued on any flip, they target cluster-wide",
			policies: []policyv1alpha1.StackConfigPolicy{
				policy("global", "operator-ns"),
				policy("elsewhere", "scoped"),
			},
			want: []reconcile.Request{
				req("operator-ns", "global"),
			},
		},
		{
			name: "mixed: both the flipped namespace's and the operator namespace's policies are enqueued",
			policies: []policyv1alpha1.StackConfigPolicy{
				policy("local", "flipped"),
				policy("global", "operator-ns"),
				policy("elsewhere", "scoped"),
			},
			want: []reconcile.Request{
				req("flipped", "local"),
				req("operator-ns", "global"),
			},
		},
		{
			name: "no policies",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := cachemock.NewCache(t)
			c.On("List", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
				list := args.Get(1).(*policyv1alpha1.StackConfigPolicyList) //nolint:forcetypeassert
				list.Items = append(list.Items, tt.policies...)
			}).Return(nil)

			reqs := namespaceFlipRequests(c, "operator-ns")(
				context.Background(),
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "flipped"}},
			)

			require.ElementsMatch(t, tt.want, reqs)
		})
	}
}
