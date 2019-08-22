// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package finalizer

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func createResource(finalizers []string, markDeletion bool) v1.Secret {
	res := v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "namespace",
			Name:       "secret",
			Finalizers: finalizers,
		},
	}
	if markDeletion {
		now := metav1.Now()
		res.ObjectMeta.DeletionTimestamp = &now
	}
	return res
}

var dummyFinalizerNames = []string{"finalizer0", "finalizer1"}
var dummyFinalizers = []Finalizer{
	{
		Name:    dummyFinalizerNames[0],
		Execute: func() error { return nil },
	},
	{
		Name:    dummyFinalizerNames[1],
		Execute: func() error { return nil },
	},
}

func TestHandler_Handle(t *testing.T) {
	tests := []struct {
		name       string
		finalizers []Finalizer
		// resource is set to a v1.Secret type, but could be anything
		resource                 v1.Secret
		wantRegisteredFinalizers []string
		wantErr                  error
	}{
		{
			name:                     "Reconcile when no finalizers are already set",
			finalizers:               dummyFinalizers,
			resource:                 createResource(nil, false),
			wantRegisteredFinalizers: dummyFinalizerNames,
			wantErr:                  nil,
		},
		{
			name:                     "Reconcile when half the finalizers are already set",
			finalizers:               dummyFinalizers,
			resource:                 createResource(dummyFinalizerNames[1:], false),
			wantRegisteredFinalizers: dummyFinalizerNames,
			wantErr:                  nil,
		},
		{
			name:                     "Reconcile when all finalizers are already set",
			finalizers:               dummyFinalizers,
			resource:                 createResource(dummyFinalizerNames, false),
			wantRegisteredFinalizers: dummyFinalizerNames,
			wantErr:                  nil,
		},
		{
			name:                     "Nothing to reconcile when no finalizers",
			finalizers:               nil,
			resource:                 createResource(nil, false),
			wantRegisteredFinalizers: nil,
			wantErr:                  nil,
		},
		{
			name:                     "Execute finalizers when resource marked for deletion",
			finalizers:               dummyFinalizers,
			resource:                 createResource(dummyFinalizerNames, true),
			wantRegisteredFinalizers: nil, // all executed
			wantErr:                  nil,
		},
		{
			name: "Return on execution error",
			finalizers: []Finalizer{
				{
					Name:    "finalizer-ok",
					Execute: func() error { return nil },
				},
				{
					Name:    "finalizer-error",
					Execute: func() error { return errors.New("finalizer error") },
				},
			},
			resource:                 createResource([]string{"finalizer-ok", "finalizer-error"}, true),
			wantRegisteredFinalizers: []string{"finalizer-ok"},      // first one executed
			wantErr:                  errors.New("finalizer error"), // error returned on second one
		},
		{
			name:                     "Nothing to execute when no finalizers",
			finalizers:               nil,
			resource:                 createResource(nil, true),
			wantRegisteredFinalizers: nil,
			wantErr:                  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// pretend resource already exists in api server
			fakeClient := fake.NewFakeClient(&tt.resource)
			handler := Handler{
				client: k8s.WrapClient(fakeClient),
			}
			err := handler.Handle(&tt.resource, tt.finalizers...)
			if tt.wantErr != nil {
				require.Error(t, tt.wantErr)
				return
			}
			require.NoError(t, err)
			// retrieve resource back from the apiserver
			var res v1.Secret
			err = fakeClient.Get(context.Background(), k8s.ExtractNamespacedName(&tt.resource), &res)
			require.NoError(t, err)
			// make sure resource's finalizers are the expected ones
			require.ElementsMatch(t, tt.wantRegisteredFinalizers, res.ObjectMeta.Finalizers)
		})
	}
}
