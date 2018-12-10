package stack

import (
	common "github.com/elastic/stack-operators/stack-operator/pkg/apis/common/v1alpha1"
	stacktype "github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	estype "github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	kbtype "github.com/elastic/stack-operators/stack-operator/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/test/e2e/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const defaultNamespace = "default"
const defaultVersion = "6.4.2"
const defaultMemory = "1G"

var DefaultResources common.ResourcesSpec

func init() {
	// init DefaultResources
	memory, err := resource.ParseQuantity(defaultMemory)
	helpers.ExitOnErr(err)
	DefaultResources = common.ResourcesSpec{
		Limits: map[corev1.ResourceName]resource.Quantity{
			"memory": memory,
		},
	}
}

// -- Stack

type Builder struct {
	stacktype.Stack
}

func NewStackBuilder(name string) Builder {
	return Builder{
		stacktype.Stack{
			ObjectMeta: v1.ObjectMeta{
				Name:      name,
				Namespace: defaultNamespace,
			},
			Spec: stacktype.StackSpec{
				Elasticsearch: estype.ElasticsearchSpec{},
				Kibana:        kbtype.KibanaSpec{},
				Version:       defaultVersion,
			},
		},
	}
}

func (b Builder) WithNamespace(namespace string) Builder {
	b.ObjectMeta.Namespace = namespace
	return b
}

func (b Builder) WithVersion(version string) Builder {
	b.Spec.Version = version
	return b
}

// -- ES Topologies

func (b Builder) WithNoESTopologies() Builder {
	b.Spec.Elasticsearch.Topologies = []estype.ElasticsearchTopologySpec{}
	return b
}

func (b Builder) WithESMasterNodes(count int, resources common.ResourcesSpec) Builder {
	return b.WithESTopology(estype.ElasticsearchTopologySpec{
		NodeCount: int32(count),
		NodeTypes: estype.NodeTypesSpec{Master: true, Data: false},
		Resources: resources,
	})
}

func (b Builder) WithESDataNodes(count int, resources common.ResourcesSpec) Builder {
	return b.WithESTopology(estype.ElasticsearchTopologySpec{
		NodeCount: int32(count),
		NodeTypes: estype.NodeTypesSpec{Master: false, Data: true},
		Resources: resources,
	})
}

func (b Builder) WithESMasterDataNodes(count int, resources common.ResourcesSpec) Builder {
	return b.WithESTopology(estype.ElasticsearchTopologySpec{
		NodeCount: int32(count),
		NodeTypes: estype.NodeTypesSpec{Master: true, Data: true},
		Resources: resources,
	})
}

func (b Builder) WithESTopology(topology estype.ElasticsearchTopologySpec) Builder {
	b.Spec.Elasticsearch.Topologies = append(b.Spec.Elasticsearch.Topologies, topology)
	return b
}

// -- Helper functions

func GetNamespacedName(stack stacktype.Stack) types.NamespacedName {
	return types.NamespacedName{
		Name:      stack.GetName(),
		Namespace: stack.GetNamespace(),
	}
}
