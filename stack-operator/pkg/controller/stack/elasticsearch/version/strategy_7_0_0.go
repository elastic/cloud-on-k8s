package version

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/common/version"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/elasticsearch"
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

// NewExpectedPodSpecs returns a list of pod specs with context that we would expect to find in the stack
func (s strategy_7_0_0) NewExpectedPodSpecs(
	stack v1alpha1.Stack,
	paramsTmpl elasticsearch.NewPodSpecParams,
) ([]elasticsearch.PodSpecContext, error) {
	return s.previousStrategy.NewExpectedPodSpecs(stack, paramsTmpl)
}

// NewPod creates a new pod from the given parameters.
func (s strategy_7_0_0) NewPod(
	stack v1alpha1.Stack,
	podSpecCtx elasticsearch.PodSpecContext,
) (corev1.Pod, error) {
	return s.previousStrategy.NewPod(stack, podSpecCtx)
}
