package services

import (
	"testing"

	"github.com/elastic/k8s-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
				ObjectMeta: metav1.ObjectMeta{
					Name:      "an-es-name",
					Namespace: "default",
				},
			}},
			want: "https://an-es-name-es-public.default.svc.cluster.local:9200",
		},
		{
			name: "Another Service URL",
			args: args{es: v1alpha1.ElasticsearchCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "another-es-name",
					Namespace: "default",
				},
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
