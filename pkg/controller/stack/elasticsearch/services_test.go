package elasticsearch

import (
	"testing"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPublicServiceURL(t *testing.T) {
	type args struct {
		stack deploymentsv1alpha1.Stack
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "A service URL",
			args: args{stack: deploymentsv1alpha1.Stack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "a-stack-name",
					Namespace: "default",
				},
			}},
			want: "http://a-stack-name-es-public.default.svc.cluster.local:9200",
		},
		{
			name: "Another Service URL",
			args: args{stack: deploymentsv1alpha1.Stack{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "another-stack-name",
					Namespace: "default",
				},
			}},
			want: "http://another-stack-name-es-public.default.svc.cluster.local:9200",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PublicServiceURL(tt.args.stack)
			assert.Equal(t, tt.want, got)
		})
	}
}
