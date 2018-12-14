package support

import (
	"testing"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/k8s"

	"github.com/stretchr/testify/assert"
)

func TestPublicServiceURL(t *testing.T) {
	type args struct {
		es v1alpha1.ElasticsearchCluster
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "A service URL",
			args: args{es: v1alpha1.ElasticsearchCluster{
				ObjectMeta: k8s.ObjectMeta(k8s.DefaultNamespace, "an-es-name"),
			}},
			want: "https://an-es-name-es-public.default.svc.cluster.local:9200",
		},
		{
			name: "Another Service URL",
			args: args{es: v1alpha1.ElasticsearchCluster{
				ObjectMeta: k8s.ObjectMeta(k8s.DefaultNamespace, "another-es-name"),
			}},
			want: "https://another-es-name-es-public.default.svc.cluster.local:9200",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PublicServiceURL(tt.args.es)
			assert.Equal(t, tt.want, got)
		})
	}
}
