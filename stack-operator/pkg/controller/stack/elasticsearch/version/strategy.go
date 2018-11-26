package version

import (
	"fmt"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/elasticsearch/client"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/common/version"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/elasticsearch"

	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	// ElasticsearchVersionLabelName is the name of the label that contains the Elasticsearch version of the resource.
	ElasticsearchVersionLabelName = "elasticsearch.stack.k8s.elastic.co/version"
)

var (
	log = logf.Log.WithName("version")
)

// ElasticsearchVersionStrategy is a strategy that provides behavior for an operator that targets an Elasticsearch
// version.
//
// TODO: Upgrade checks (e.g does the cluster contain indices that will not be supported by my target version)
// TODO: Support major version upgrades
// TODO: Create a mechanism to clean up versioned resources that are no longer needed
type ElasticsearchVersionStrategy interface {
	// Version is the current target version
	Version() version.Version
	// VerifySupportsExistingPods returns true if this strategy works for the given pods
	VerifySupportsExistingPods(pods []corev1.Pod) error
	// NewPodLabels returns version-related labels for new pods
	NewPodLabels() map[string]string

	// NewExpectedPodSpecs returns a list of pod specs with context that we would expect to find in the stack
	NewExpectedPodSpecs(
		stack v1alpha1.Stack,
		paramsTmpl elasticsearch.NewPodSpecParams,
	) ([]elasticsearch.PodSpecContext, error)

	// NewPod creates a new pod from the given parameters.
	NewPod(
		stack v1alpha1.Stack,
		podSpecCtx elasticsearch.PodSpecContext,
	) (corev1.Pod, error)

	// UpdateDiscovery configures discovery settings based on the given list of pods.
	UpdateDiscovery(esClient *client.Client, allPods []corev1.Pod) error
}

var (
	_ ElasticsearchVersionStrategy = &strategy_5_6_0{}
	_ ElasticsearchVersionStrategy = &strategy_6_4_0{}
	_ ElasticsearchVersionStrategy = &strategy_7_0_0{}
)

// LookupStrategy returns an ElasticsearchVersionStrategy that can be used for the given stack version.
func LookupStrategy(v version.Version) (ElasticsearchVersionStrategy, error) {
	switch v.Major {
	case 7:
		return newStrategy_7_0_0(v), nil
	case 6:
		if v.Minor <= 4 {
			return newStrategy_6_4_0(v), nil
		}
	case 5:
		return newStrategy_5_6_0(v), nil
	}

	return nil, fmt.Errorf("unsupported version: %s", v)
}
