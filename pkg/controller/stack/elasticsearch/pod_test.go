package elasticsearch

import (
	"fmt"
	"testing"

	deploymentsv1alpha1 "github.com/elastic/stack-operators/pkg/apis/deployments/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewPodSpecParams_Hash(t *testing.T) {
	type fields struct {
		params NewPodSpecParams
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "Hash computes a hash from the params string",
			fields: fields{
				params: NewPodSpecParams{
					Version:                        "6.4.2",
					CustomImageName:                "docker.elastic.co/elasticsearch",
					ClusterName:                    "my-stack",
					DiscoveryServiceName:           "some-discovery",
					DiscoveryZenMinimumMasterNodes: 2,
					SetVMMaxMapCount:               true,
				},
			},
			want: "17342333876247741356",
		},
		{
			name: "Hash computes a The same hash as above as DiscoveryZenMinimumMasterNodes is ignored",
			fields: fields{
				params: NewPodSpecParams{
					Version:                        "6.4.2",
					CustomImageName:                "docker.elastic.co/elasticsearch",
					ClusterName:                    "my-stack",
					DiscoveryServiceName:           "some-discovery",
					DiscoveryZenMinimumMasterNodes: 0,
					SetVMMaxMapCount:               true,
				},
			},
			want: "17342333876247741356",
		},
		{
			name: "Hash computes another hash",
			fields: fields{
				params: NewPodSpecParams{
					Version:              "6.4.1",
					CustomImageName:      "docker.elastic.co/elasticsearch",
					ClusterName:          "my-stack",
					DiscoveryServiceName: "some-discovery",
					SetVMMaxMapCount:     true,
				},
			},
			want: "17117365281282058121",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.fields.params.Hash(), "Unmactching hashes")
		})
	}
}

func TestBuildNewPodSpecParams(t *testing.T) {
	type args struct {
		s deploymentsv1alpha1.Stack
	}
	tests := []struct {
		name string
		args args
		want NewPodSpecParams
	}{
		{
			name: "Constructs params",
			args: args{
				s: deploymentsv1alpha1.Stack{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-stack",
						Namespace: "default",
					},
					Spec: deploymentsv1alpha1.StackSpec{
						Version: "6.4.2",
						Elasticsearch: deploymentsv1alpha1.ElasticsearchSpec{
							NodeCount:        2,
							SetVMMaxMapCount: true,
						},
					},
				},
			},
			want: NewPodSpecParams{
				Version:                        "6.4.2",
				ClusterName:                    "my-stack",
				DiscoveryZenMinimumMasterNodes: ComputeMinimumMasterNodes(2),
				DiscoveryServiceName:           DiscoveryServiceName("my-stack"),
				SetVMMaxMapCount:               true,
			},
		},
		{
			name: "Constructs params",
			args: args{
				s: deploymentsv1alpha1.Stack{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-stack",
						Namespace: "default",
					},
					Spec: deploymentsv1alpha1.StackSpec{
						Version: "6.4.2",
						Elasticsearch: deploymentsv1alpha1.ElasticsearchSpec{
							Image:            fmt.Sprintf("%s:%s", defaultImageRepositoryAndName, "6.4.2"),
							NodeCount:        2,
							SetVMMaxMapCount: true,
						},
					},
				},
			},
			want: NewPodSpecParams{
				Version:                        "6.4.2",
				CustomImageName:                fmt.Sprintf("%s:%s", defaultImageRepositoryAndName, "6.4.2"),
				ClusterName:                    "my-stack",
				DiscoveryZenMinimumMasterNodes: ComputeMinimumMasterNodes(2),
				DiscoveryServiceName:           DiscoveryServiceName("my-stack"),
				SetVMMaxMapCount:               true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildNewPodSpecParams(tt.args.s)
			assert.Equal(t, tt.want, got, "Unmatching NewPodSpecParams")
		})
	}
}
