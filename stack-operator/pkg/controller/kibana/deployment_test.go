package kibana

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/kibana/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
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
			args: args{kibana: v1alpha1.Kibana{ObjectMeta: v1.ObjectMeta{Name: "a-name"}}},
			want: "a-name-kibana",
		},
		{
			args: args{kibana: v1alpha1.Kibana{ObjectMeta: v1.ObjectMeta{Name: "another-name"}}},
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
