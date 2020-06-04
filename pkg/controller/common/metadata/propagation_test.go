// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package metadata

import (
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/annotation"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPropagate(t *testing.T) {
	testCases := []struct {
		name    string
		objMeta metav1.ObjectMeta
		toAdd   Metadata
		want    Metadata
	}{
		{
			name: "everything nil",
		},
		{
			name: "no propagation and nothing to add",
			objMeta: metav1.ObjectMeta{
				Annotations: map[string]string{"foo": "bar"},
				Labels:      map[string]string{"wibble": "wobble"},
			},
			toAdd: Metadata{},
			want:  Metadata{},
		},
		{
			name: "no propagation",
			objMeta: metav1.ObjectMeta{
				Annotations: map[string]string{"foo": "bar"},
				Labels:      map[string]string{"wibble": "wobble"},
			},
			toAdd: Metadata{
				Annotations: map[string]string{"bar": "baz"},
				Labels:      map[string]string{"wubble": "flub"},
			},
			want: Metadata{
				Annotations: map[string]string{"bar": "baz"},
				Labels:      map[string]string{"wubble": "flub"},
			},
		},
		{
			name: "annotation propagation without conflict",
			objMeta: metav1.ObjectMeta{
				Annotations: map[string]string{annotation.PropagateAnnotationsAnnotation: "*", "foo": "bar"},
				Labels:      map[string]string{"wibble": "wobble"},
			},
			toAdd: Metadata{
				Annotations: map[string]string{"bar": "baz"},
				Labels:      map[string]string{"wubble": "flub"},
			},
			want: Metadata{
				Annotations: map[string]string{"foo": "bar", "bar": "baz"},
				Labels:      map[string]string{"wubble": "flub"},
			},
		},
		{
			name: "annotation propagation with conflict",
			objMeta: metav1.ObjectMeta{
				Annotations: map[string]string{annotation.PropagateAnnotationsAnnotation: "*", "foo": "bar"},
				Labels:      map[string]string{"wibble": "wobble"},
			},
			toAdd: Metadata{
				Annotations: map[string]string{"foo": "baz"},
				Labels:      map[string]string{"wubble": "flub"},
			},
			want: Metadata{
				Annotations: map[string]string{"foo": "baz"},
				Labels:      map[string]string{"wubble": "flub"},
			},
		},
		{
			name: "label propagation without conflict",
			objMeta: metav1.ObjectMeta{
				Annotations: map[string]string{annotation.PropagateLabelsAnnotation: "*", "foo": "bar"},
				Labels:      map[string]string{"wibble": "wobble"},
			},
			toAdd: Metadata{
				Annotations: map[string]string{"foo": "baz"},
				Labels:      map[string]string{"wubble": "flub"},
			},
			want: Metadata{
				Annotations: map[string]string{"foo": "baz"},
				Labels:      map[string]string{"wibble": "wobble", "wubble": "flub"},
			},
		},
		{
			name: "label propagation with conflict",
			objMeta: metav1.ObjectMeta{
				Annotations: map[string]string{annotation.PropagateLabelsAnnotation: "*", "foo": "bar"},
				Labels:      map[string]string{"wibble": "wobble"},
			},
			toAdd: Metadata{
				Annotations: map[string]string{"foo": "baz"},
				Labels:      map[string]string{"wibble": "flub"},
			},
			want: Metadata{
				Annotations: map[string]string{"foo": "baz"},
				Labels:      map[string]string{"wibble": "flub"},
			},
		},
		{
			name: "propagation with nils",
			objMeta: metav1.ObjectMeta{
				Annotations: map[string]string{annotation.PropagateLabelsAnnotation: "*", annotation.PropagateAnnotationsAnnotation: "*"},
			},
			toAdd: Metadata{},
			want:  Metadata{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			obj := &esv1.Elasticsearch{
				ObjectMeta: tc.objMeta,
			}

			have := Propagate(obj, tc.toAdd)
			require.Equal(t, tc.want, have)
		})
	}

}

func TestMetadataMerge(t *testing.T) {
	testCases := []struct {
		name  string
		md    Metadata
		input Metadata
		want  Metadata
	}{
		{
			name:  "empty receiver",
			md:    Metadata{},
			input: Metadata{Labels: map[string]string{"foo": "bar"}, Annotations: map[string]string{"wibble": "wobble"}},
			want:  Metadata{Labels: map[string]string{"foo": "bar"}, Annotations: map[string]string{"wibble": "wobble"}},
		},
		{
			name:  "empty input",
			md:    Metadata{Labels: map[string]string{"foo": "bar"}, Annotations: map[string]string{"wibble": "wobble"}},
			input: Metadata{},
			want:  Metadata{Labels: map[string]string{"foo": "bar"}, Annotations: map[string]string{"wibble": "wobble"}},
		},
		{
			name:  "merge without conflicts",
			md:    Metadata{Labels: map[string]string{"foo": "bar"}, Annotations: map[string]string{"wibble": "wobble"}},
			input: Metadata{Labels: map[string]string{"baz": "quux"}, Annotations: map[string]string{"wubble": "flub"}},
			want:  Metadata{Labels: map[string]string{"foo": "bar", "baz": "quux"}, Annotations: map[string]string{"wibble": "wobble", "wubble": "flub"}},
		},
		{
			name:  "merge with conflicts",
			md:    Metadata{Labels: map[string]string{"foo": "bar"}, Annotations: map[string]string{"wibble": "wobble"}},
			input: Metadata{Labels: map[string]string{"foo": "baz", "baz": "quux"}, Annotations: map[string]string{"wibble": "wubble", "wubble": "flub"}},
			want:  Metadata{Labels: map[string]string{"foo": "baz", "baz": "quux"}, Annotations: map[string]string{"wibble": "wubble", "wubble": "flub"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			have := tc.md.Merge(tc.input)

			require.Equal(t, tc.want, have)
		})
	}
}
