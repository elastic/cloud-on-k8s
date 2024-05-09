// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package watches

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func Test_objToReconcileRequest(t *testing.T) {
	labelName := "obj-name-label"
	fn := objToReconcileRequest[client.Object](labelName)

	tests := []struct {
		name string
		obj  client.Object
		want []reconcile.Request
	}{
		{
			name: "reconcile based on the Pod label",
			obj: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns", Name: "my-pod",
				Labels: map[string]string{labelName: "my-obj-name"},
			}},
			want: []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "my-obj-name"}}},
		},
		{
			name: "don't reconcile if no labels",
			obj: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns", Name: "my-pod",
			}},
			want: nil,
		},
		{
			name: "don't reconcile if label not set",
			obj: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns", Name: "my-pod",
				Labels: map[string]string{"other": "label"},
			}},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fn(context.Background(), tt.obj)
			require.Equal(t, tt.want, got)
		})
	}
}
