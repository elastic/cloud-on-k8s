// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func Test_podsToReconcilerequest(t *testing.T) {
	tests := []struct {
		name   string
		object handler.MapObject
		want   []reconcile.Request
	}{
		{
			name: "ent search pod",
			object: handler.MapObject{
				Meta: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "name", Namespace: "ns",
					Labels: map[string]string{EnterpriseSearchNameLabelName: "name"}},
				},
				Object: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "name", Namespace: "ns",
					Labels: map[string]string{EnterpriseSearchNameLabelName: "name"}},
				},
			},
			want: []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Namespace: "ns",
						Name:      "name",
					},
				},
			},
		},
		{
			name: "not an ent search pod",
			object: handler.MapObject{
				Meta:   &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "name", Namespace: "ns"}},
				Object: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "name", Namespace: "ns"}},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := podsToReconcilerequest(tt.object); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("podsToReconcilerequest() = %v, want %v", got, tt.want)
			}
		})
	}
}
