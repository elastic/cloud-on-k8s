package kibana

import (
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"
)

func TestPseudoNamespacedResourceName(t *testing.T) {
	type args struct {
		kibana v1alpha1.Kibana
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			args: args{kibana: v1alpha1.Kibana{ObjectMeta: k8s.ObjectMeta("", "a-name")}},
			want: "a-name-kibana",
		},
		{
			args: args{kibana: v1alpha1.Kibana{ObjectMeta: k8s.ObjectMeta("", "another-name")}},
			want: "another-name-kibana",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PseudoNamespacedResourceName(tt.args.kibana); got != tt.want {
				t.Errorf("PseudoNamespacedResourceName() = %v, want %v", got, tt.want)
			}
		})
	}
}
