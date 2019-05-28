package overrides

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestEnvBuilder(t *testing.T) {
	envVarList := []corev1.EnvVar{{Name: "a", Value: "a-value"}, {Name: "b", Value: "b-value"}, {Name: "c", Value: "c-value"}}
	tests := []struct {
		name        string
		initialVars []corev1.EnvVar
		operations  func(e *EnvBuilder)
		want        []corev1.EnvVar
	}{
		{
			name:       "no vars",
			operations: func(e *EnvBuilder) {},
			want:       []corev1.EnvVar{},
		},
		{
			name:        "initial list",
			initialVars: envVarList,
			operations:  func(e *EnvBuilder) {},
			want:        envVarList,
		},
		{
			name:        "add if missing",
			initialVars: envVarList,
			operations: func(e *EnvBuilder) {
				e.AddIfMissing(
					corev1.EnvVar{Name: "b", Value: "should-not-override"},
					corev1.EnvVar{Name: "c", Value: "should-not-override"},
					corev1.EnvVar{Name: "d", Value: "d-value"},
					corev1.EnvVar{Name: "e", Value: "e-value"},
				)
			},
			want: append(envVarList,
				corev1.EnvVar{Name: "d", Value: "d-value"},
				corev1.EnvVar{Name: "e", Value: "e-value"},
			),
		},
		{
			name:        "add or override",
			initialVars: envVarList,
			operations: func(e *EnvBuilder) {
				e.AddOrOverride(
					corev1.EnvVar{Name: "b", Value: "should-override"},
					corev1.EnvVar{Name: "c", Value: "should-override"},
					corev1.EnvVar{Name: "d", Value: "d-value"},
					corev1.EnvVar{Name: "e", Value: "e-value"},
				)
			},
			want: []corev1.EnvVar{
				{Name: "a", Value: "a-value"},
				{Name: "b", Value: "should-override"},
				{Name: "c", Value: "should-override"},
				{Name: "d", Value: "d-value"},
				{Name: "e", Value: "e-value"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewEnvBuilder(tt.initialVars...)
			tt.operations(e)
			require.Equal(t, tt.want, e.GetEnvVars())
		})
	}
}
