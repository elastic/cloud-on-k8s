// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package autoops

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/rbac"
)

type fakeAccessReviewer struct {
	allowed bool
	err     error
}

func (f *fakeAccessReviewer) AccessAllowed(_ context.Context, _ string, _ string, _ runtime.Object) (bool, error) {
	return f.allowed, f.err
}

var _ rbac.AccessReviewer = (*configurableAccessReviewer)(nil)

// configurableAccessReviewer allows configuring access per ES cluster for testing.
type configurableAccessReviewer struct {
	// allowedClusters maps "namespace/name" to whether access is allowed
	allowedClusters map[string]bool
	// defaultAllow is used when a cluster is not in the map
	defaultAllow bool
}

func newConfigurableAccessReviewer(defaultAllow bool) *configurableAccessReviewer {
	return &configurableAccessReviewer{
		allowedClusters: make(map[string]bool),
		defaultAllow:    defaultAllow,
	}
}

func (c *configurableAccessReviewer) SetAccess(namespace, name string, allowed bool) {
	c.allowedClusters[namespace+"/"+name] = allowed
}

func (c *configurableAccessReviewer) AccessAllowed(_ context.Context, _ string, _ string, obj runtime.Object) (bool, error) {
	metaObj, ok := obj.(metav1.Object)
	if !ok {
		return c.defaultAllow, nil
	}
	key := metaObj.GetNamespace() + "/" + metaObj.GetName()
	if allowed, exists := c.allowedClusters[key]; exists {
		return allowed, nil
	}
	return c.defaultAllow, nil
}

var _ rbac.AccessReviewer = &configurableAccessReviewer{}

func TestIsAutoOpsAssociationAllowed(t *testing.T) {
	tests := []struct {
		name           string
		policy         autoopsv1alpha1.AutoOpsAgentPolicy
		es             esv1.Elasticsearch
		accessReviewer rbac.AccessReviewer
		wantAllowed    bool
		wantErr        bool
	}{
		{
			name: "access allowed by reviewer",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					ServiceAccountName: "test-sa",
				},
			},
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "es-1",
					Namespace: "ns-2",
				},
			},
			accessReviewer: &fakeAccessReviewer{allowed: true},
			wantAllowed:    true,
			wantErr:        false,
		},
		{
			name: "access denied by reviewer",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					ServiceAccountName: "test-sa",
				},
			},
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "es-1",
					Namespace: "ns-2",
				},
			},
			accessReviewer: &fakeAccessReviewer{allowed: false},
			wantAllowed:    false,
			wantErr:        false,
		},
		{
			name: "access reviewer returns error",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{
					ServiceAccountName: "test-sa",
				},
			},
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "es-1",
					Namespace: "ns-2",
				},
			},
			accessReviewer: &fakeAccessReviewer{err: errors.New("access review failed")},
			wantAllowed:    false,
			wantErr:        true,
		},
		{
			name: "uses default service account when not specified",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
				Spec: autoopsv1alpha1.AutoOpsAgentPolicySpec{},
			},
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "es-1",
					Namespace: "ns-2",
				},
			},
			accessReviewer: &fakeAccessReviewer{allowed: true},
			wantAllowed:    true,
			wantErr:        false,
		},
		{
			name: "permissive reviewer always allows",
			policy: autoopsv1alpha1.AutoOpsAgentPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "policy-1",
					Namespace: "ns-1",
				},
			},
			es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "es-1",
					Namespace: "ns-2",
				},
			},
			accessReviewer: rbac.NewPermissiveAccessReviewer(),
			wantAllowed:    true,
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventRecorder := record.NewFakeRecorder(10)
			allowed, err := isAutoOpsAssociationAllowed(
				context.Background(),
				tt.accessReviewer,
				&tt.policy,
				&tt.es,
				eventRecorder,
			)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantAllowed, allowed)
		})
	}
}
