// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.
package elasticsearch

import (
	"context"
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/comparison"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/hints"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

// newTestReconciler returns a ReconcileElasticsearch struct, allowing the internal k8s client to be
// modified to be a potential failing client, and contain certain objects.
func newTestReconciler(failing bool, err error, objects ...runtime.Object) *ReconcileElasticsearch {
	r := &ReconcileElasticsearch{
		Client:   k8s.NewFakeClient(objects...),
		recorder: record.NewFakeRecorder(100),
	}
	if failing {
		r.Client = k8s.NewFailingClient(err, objects...)
	}
	return r
}

// esBuilder allows for a cleaner way to build a testable elasticsearch spec,
// minimizing repetition.
type esBuilder struct {
	es *esv1.Elasticsearch
}

// newBuilder returns a new elasticsearch test builder
// with given name/namespace combination.
func newBuilder(name, namespace string) *esBuilder {
	return &esBuilder{
		es: &esv1.Elasticsearch{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		},
	}
}

// WithAnnotations adds the given annotations to the ES object.
func (e *esBuilder) WithAnnotations(annotations map[string]string) *esBuilder {
	e.es.ObjectMeta.Annotations = annotations
	return e
}

// WithGeneration adds the metadata.generation to the ES object.
func (e *esBuilder) WithGeneration(generation int64) *esBuilder {
	e.es.ObjectMeta.Generation = generation
	return e
}

// WithStatus adds the status subresource to the ES object.
func (e *esBuilder) WithStatus(status esv1.ElasticsearchStatus) *esBuilder {
	e.es.Status = status
	return e
}

// WithVersion adds the ES version to the ES objects specification.
func (e *esBuilder) WithVersion(version string) *esBuilder {
	e.es.Spec.Version = version
	return e
}

// Build builds the final ES object and returns a pointer.
func (e *esBuilder) Build() *esv1.Elasticsearch {
	return e.es
}

// BuildAndCopy builds the final ES object and returns a copy.
func (e *esBuilder) BuildAndCopy() esv1.Elasticsearch {
	return *e.es
}

func TestReconcileElasticsearch_Reconcile(t *testing.T) {
	type k8sClientFields struct {
		failing bool
		err     error
		objects []runtime.Object
	}
	type args struct {
		request reconcile.Request
	}
	tests := []struct {
		name            string
		k8sClientFields k8sClientFields
		args            args
		wantErr         bool
		expected        esv1.Elasticsearch
	}{
		{
			"fetch fails with error, and no observedGeneration update",
			k8sClientFields{
				true,
				fmt.Errorf("internal error"),
				[]runtime.Object{
					newBuilder("testES", "test").
						WithGeneration(2).
						WithStatus(esv1.ElasticsearchStatus{ObservedGeneration: 1}).Build()},
			},
			args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "testES",
						Namespace: "test",
					},
				},
			},
			true,
			newBuilder("testES", "test").
				WithGeneration(2).
				WithStatus(esv1.ElasticsearchStatus{ObservedGeneration: 1}).BuildAndCopy(),
		},
		{
			"unmanaged ES has no error, and no observedGeneration update",
			k8sClientFields{
				false,
				nil,
				[]runtime.Object{
					newBuilder("testES", "test").
						WithGeneration(2).
						WithAnnotations(map[string]string{common.ManagedAnnotation: "false"}).
						WithStatus(esv1.ElasticsearchStatus{ObservedGeneration: 1}).Build()},
			},
			args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "testES",
						Namespace: "test",
					},
				},
			},
			false,
			newBuilder("testES", "test").
				WithGeneration(2).
				WithAnnotations(map[string]string{common.ManagedAnnotation: "false"}).
				WithStatus(esv1.ElasticsearchStatus{ObservedGeneration: 1}).BuildAndCopy(),
		},
		{
			"ES with too long name, fails initial reconcile, but has observedGeneration updated",
			k8sClientFields{
				false,
				nil,
				[]runtime.Object{
					newBuilder("testESwithtoolongofanamereallylongname", "test").
						WithGeneration(2).
						WithAnnotations(map[string]string{hints.OrchestrationsHintsAnnotation: `{"no_transient_settings":false}`}).
						WithStatus(esv1.ElasticsearchStatus{ObservedGeneration: 1}).Build(),
				},
			},
			args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "testESwithtoolongofanamereallylongname",
						Namespace: "test",
					},
				},
			},
			false,
			newBuilder("testESwithtoolongofanamereallylongname", "test").
				WithGeneration(2).
				WithAnnotations(map[string]string{hints.OrchestrationsHintsAnnotation: `{"no_transient_settings":false}`}).
				WithStatus(esv1.ElasticsearchStatus{ObservedGeneration: 2, Phase: esv1.ElasticsearchResourceInvalid}).BuildAndCopy(),
		},
		{
			"ES with too long name, and needing annotations update, fails initial reconcile, and does not have status.* updated because of a 409/resource conflict",
			k8sClientFields{
				false,
				nil,
				[]runtime.Object{
					newBuilder("testESwithtoolongofanamereallylongname", "test").
						WithGeneration(2).
						WithStatus(esv1.ElasticsearchStatus{ObservedGeneration: 1}).Build()},
			},
			args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "testESwithtoolongofanamereallylongname",
						Namespace: "test",
					},
				},
			},
			false,
			newBuilder("testESwithtoolongofanamereallylongname", "test").
				WithGeneration(2).
				WithAnnotations(map[string]string{hints.OrchestrationsHintsAnnotation: `{"no_transient_settings":false}`}).
				WithStatus(esv1.ElasticsearchStatus{ObservedGeneration: 1}).BuildAndCopy(),
		},
		{
			"Invalid ES version errors, and updates observedGeneration",
			k8sClientFields{
				false,
				nil,
				[]runtime.Object{
					newBuilder("testES", "test").
						WithGeneration(2).
						WithVersion("invalid").
						WithAnnotations(map[string]string{hints.OrchestrationsHintsAnnotation: `{"no_transient_settings":false}`}).
						WithStatus(esv1.ElasticsearchStatus{ObservedGeneration: 1}).Build()},
			},
			args{
				request: reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      "testES",
						Namespace: "test",
					},
				},
			},
			false,
			newBuilder("testES", "test").
				WithGeneration(2).
				WithVersion("invalid").
				WithAnnotations(map[string]string{hints.OrchestrationsHintsAnnotation: `{"no_transient_settings":false}`}).
				WithStatus(esv1.ElasticsearchStatus{ObservedGeneration: 2, Phase: esv1.ElasticsearchResourceInvalid}).BuildAndCopy(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestReconciler(tt.k8sClientFields.failing, tt.k8sClientFields.err, tt.k8sClientFields.objects...)
			if _, err := r.Reconcile(context.Background(), tt.args.request); (err != nil) != tt.wantErr {
				t.Errorf("ReconcileElasticsearch.Reconcile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			disableClientFailing(r, tt.k8sClientFields.failing)
			var actualES esv1.Elasticsearch
			if err := r.Client.Get(context.Background(), tt.args.request.NamespacedName, &actualES); err != nil {
				t.Error(err)
				return
			}
			comparison.AssertEqual(t, &actualES, &tt.expected)
		})
	}
}

func disableClientFailing(r *ReconcileElasticsearch, failingClient bool) {
	if failingClient {
		r.Client.(*k8s.FailingClient).DisableFailing()
		return
	}
}
