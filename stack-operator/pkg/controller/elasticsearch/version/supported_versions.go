package version

import (
	"fmt"

	"github.com/elastic/k8s-operators/stack-operator/pkg/controller/common/version"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

type LowestHighestSupportedVersions struct {
	LowestSupportedVersion  version.Version
	HighestSupportedVersion version.Version
}

func (lh LowestHighestSupportedVersions) VerifySupportsExistingPods(
	pods []corev1.Pod,
) error {
	for _, pod := range pods {
		labelValue, ok := pod.Labels[ElasticsearchVersionLabelName]
		if !ok {
			return fmt.Errorf("pod %s is missing the version label %s", pod.Name, ElasticsearchVersionLabelName)
		}
		v, err := version.Parse(labelValue)
		if err != nil {
			return errors.Wrapf(err, "pod %s has an invalid version label", pod.Name)
		}

		if !v.IsSameOrAfter(lh.LowestSupportedVersion) {
			return fmt.Errorf(
				"pod %s has version %v, which is older than the lowest supported version %s",
				pod.Name,
				v,
				lh.LowestSupportedVersion,
			)
		}

		if !lh.HighestSupportedVersion.IsSameOrAfter(*v) {
			return fmt.Errorf(
				"pod %s has version %v, which is newer than the highest supported version %s",
				pod.Name,
				v,
				lh.HighestSupportedVersion,
			)
		}
	}
	return nil
}
