// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package annotation

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/require"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
)

func TestGetMetadataToPropagate(t *testing.T) {
	testCases := []struct {
		name    string
		objMeta metav1.ObjectMeta
		want    MetadataToPropagate
	}{
		{
			name:    "no annotations or labels",
			objMeta: metav1.ObjectMeta{},
			want:    MetadataToPropagate{},
		},
		{
			name: "propagation not configured",
			objMeta: metav1.ObjectMeta{
				Annotations: map[string]string{"foo": "bar"},
				Labels:      map[string]string{"wibble": "wobble"},
			},
			want: MetadataToPropagate{},
		},
		{
			name: "propagate all annotations",
			objMeta: metav1.ObjectMeta{
				Annotations: map[string]string{PropagateAnnotationsAnnotation: "*", corev1.LastAppliedConfigAnnotation: "{}", "foo": "bar"},
				Labels:      map[string]string{"wibble": "wobble"},
			},
			want: MetadataToPropagate{
				Annotations: map[string]string{"foo": "bar"},
			},
		},
		{
			name: "propagate some annotations",
			objMeta: metav1.ObjectMeta{
				Annotations: map[string]string{PropagateAnnotationsAnnotation: "foo, baz", "foo": "bar", "baz": "quux", "quuz": "corge"},
				Labels:      map[string]string{"wibble": "wobble"},
			},
			want: MetadataToPropagate{
				Annotations: map[string]string{"foo": "bar", "baz": "quux"},
			},
		},
		{
			name: "propagate all labels",
			objMeta: metav1.ObjectMeta{
				Annotations: map[string]string{PropagateLabelsAnnotation: "*", "foo": "bar"},
				Labels:      map[string]string{"wibble": "wobble", "wubble": "flub"},
			},
			want: MetadataToPropagate{
				Labels: map[string]string{"wibble": "wobble", "wubble": "flub"},
			},
		},
		{
			name: "propagate some labels",
			objMeta: metav1.ObjectMeta{
				Annotations: map[string]string{PropagateLabelsAnnotation: "wibble, wubble"},
				Labels:      map[string]string{"wibble": "wobble", "wubble": "flub", "globble": "nobble"},
			},
			want: MetadataToPropagate{
				Labels: map[string]string{"wibble": "wobble", "wubble": "flub"},
			},
		},
		{
			name: "propagate all labels and annotations",
			objMeta: metav1.ObjectMeta{
				Annotations: map[string]string{PropagateAnnotationsAnnotation: "*", PropagateLabelsAnnotation: "*", "foo": "bar"},
				Labels:      map[string]string{"wibble": "wobble"},
			},
			want: MetadataToPropagate{
				Annotations: map[string]string{"foo": "bar"},
				Labels:      map[string]string{"wibble": "wobble"},
			},
		},
		{
			name: "nil labels",
			objMeta: metav1.ObjectMeta{
				Annotations: map[string]string{PropagateLabelsAnnotation: "*", "foo": "bar"},
			},
			want: MetadataToPropagate{},
		},
		{
			name: "empty string as propagation list",
			objMeta: metav1.ObjectMeta{
				Annotations: map[string]string{PropagateAnnotationsAnnotation: "", PropagateLabelsAnnotation: " ", "foo": "bar"},
				Labels:      map[string]string{"wibble": "wobble"},
			},
			want: MetadataToPropagate{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			obj := &esv1.Elasticsearch{
				ObjectMeta: tc.objMeta,
			}

			have := GetMetadataToPropagate(obj)
			require.Equal(t, tc.want, have)
		})
	}
}
