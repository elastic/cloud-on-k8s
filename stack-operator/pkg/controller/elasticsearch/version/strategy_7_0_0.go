package version

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/version"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/support"
	corev1 "k8s.io/api/core/v1"
)

//noinspection GoSnakeCaseUsage
type strategy_7_0_0 struct {
	versionHolder
	versionedNewPodLabels
	lowestHighestSupportedVersions
	// previousStrategy is used to implement the interfaces because we currently require no customization
	previousStrategy strategy_6_4_0
}

//noinspection GoSnakeCaseUsage
func newStrategy_7_0_0(v version.Version) strategy_7_0_0 {
	strategy := strategy_7_0_0{
		versionHolder:         versionHolder{version: v},
		versionedNewPodLabels: versionedNewPodLabels{version: v},
		lowestHighestSupportedVersions: lowestHighestSupportedVersions{
			// 6.6.0 is the lowest wire compatibility version for 7.x
			lowestSupportedVersion: version.MustParse("6.6.0"),
			// higher may be possible, but not proven yet, lower may also be a requirement...
			highestSupportedVersion: version.MustParse("7.0.99"),
		},
		previousStrategy: newStrategy_6_4_0(v),
	}
	return strategy
}

// ExpectedConfigMaps returns a config map that is expected to exist when the Elasticsearch pods are created.
func (s strategy_7_0_0) ExpectedConfigMap(es v1alpha1.ElasticsearchCluster) corev1.ConfigMap {
	return s.previousStrategy.ExpectedConfigMap(es)
}

// ExpectedPodSpecs returns a list of pod specs with context that we would expect to find in the Elasticsearch cluster.
func (s strategy_7_0_0) ExpectedPodSpecs(
	es v1alpha1.ElasticsearchCluster,
	paramsTmpl support.NewPodSpecParams,
) ([]support.PodSpecContext, error) {
	return s.previousStrategy.ExpectedPodSpecs(es, paramsTmpl)
}

// NewPod creates a new pod from the given parameters.
func (s strategy_7_0_0) NewPod(
	es v1alpha1.ElasticsearchCluster,
	podSpecCtx support.PodSpecContext,
) (corev1.Pod, error) {
	return s.previousStrategy.NewPod(es, podSpecCtx)
}

// UpdateDiscovery configures discovery settings based on the given list of pods.
func (s strategy_7_0_0) UpdateDiscovery(esClient *client.Client, allPods []corev1.Pod) error {
	return s.previousStrategy.UpdateDiscovery(esClient, allPods)
}
