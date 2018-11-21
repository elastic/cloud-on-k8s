package version

import (
	"fmt"

	"github.com/elastic/stack-operators/stack-operator/pkg/apis/deployments/v1alpha1"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/common/version"
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/stack/elasticsearch"
	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	ElasticsearchVersionLabelName = "elasticsearch.stack.k8s.elastic.co/version"
)

var (
	log = logf.Log.WithName("version")
)

// upgrade checks?
// open ended versioning
// upgrades
// check out 5.x?
//

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
}

var _ ElasticsearchVersionStrategy = &strategy_5_6_0{}
var _ ElasticsearchVersionStrategy = &strategy_6_4_0{}
var _ ElasticsearchVersionStrategy = &strategy_7_0_0{}

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

type versionHolder struct {
	version version.Version
}

func (h versionHolder) Version() version.Version {
	return h.version
}

type versionedNewPodLabels struct {
	version version.Version
}

func (l versionedNewPodLabels) NewPodLabels() map[string]string {
	labels := make(map[string]string, 1)
	labels[ElasticsearchVersionLabelName] = l.version.String()
	return labels
}
