package version

import (
	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common/version"
)

type versionHolder struct {
	version version.Version
}

func (h versionHolder) Version() version.Version {
	return h.version
}

type versionedNewPodLabels struct {
	version version.Version
}

func (l versionedNewPodLabels) PodLabels() map[string]string {
	labels := make(map[string]string, 1)
	labels[ElasticsearchVersionLabelName] = l.version.String()
	return labels
}
