package sset

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetClaim(t *testing.T) {
	tests := []struct {
		name      string
		claims    []corev1.PersistentVolumeClaim
		claimName string
		want      *corev1.PersistentVolumeClaim
	}{
		{
			name: "return matching claim",
			claims: []corev1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Name: "claim1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "claim2"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "claim3"}},
			},
			claimName: "claim2",
			want:      &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "claim2"}},
		},
		{
			name: "return nil if no match",
			claims: []corev1.PersistentVolumeClaim{
				{ObjectMeta: metav1.ObjectMeta{Name: "claim1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "claim2"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "claim3"}},
			},
			claimName: "claim4",
			want:      nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetClaim(tt.claims, tt.claimName); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetClaim() = %v, want %v", got, tt.want)
			}
		})
	}
}
