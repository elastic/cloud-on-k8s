package common

import (
	"testing"

	"github.com/stretchr/testify/assert"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
)

func TestStackID(t *testing.T) {
	type args struct {
		s deploymentsv1alpha1.Stack
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			args: args{s: deploymentsv1alpha1.Stack{
				ObjectMeta: k8s.ObjectMeta(k8s.DefaultNamespace, "my-stack"),
			}},
			want: "default-my-stack",
		},
		{
			args: args{s: deploymentsv1alpha1.Stack{
				ObjectMeta: k8s.ObjectMeta(k8s.DefaultNamespace, "my-other-stack"),
			}},
			want: "default-my-other-stack",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StackID(tt.args.s)
			assert.Equal(t, tt.want, got)
		})
	}
}
