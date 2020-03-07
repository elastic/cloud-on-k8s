package labels

import (
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHasLabel(t *testing.T) {
	tests := []struct {
		name   string
		inp    *appsv1.Deployment
		labels []string
		want   bool
	}{
		{
			name: "no labels on object",
			inp: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
			},
			labels: []string{"x", "y"},
			want:   false,
		},
		{
			name: "empty label set provided",
			inp: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"x": "y"},
				},
			},
			labels: []string{},
			want:   false,
		},
		{
			name: "labels that match",
			inp: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"x": "y", "a": "b"},
				},
			},
			labels: []string{"x", "a"},
			want:   true,
		},
		{
			name: "labels that don't match",
			inp: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"x": "y", "a": "b"},
				},
			},
			labels: []string{"c", "d"},
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			have := HasLabel(tc.inp, tc.labels...)
			require.Equal(t, tc.want, have)
		})
	}
}
