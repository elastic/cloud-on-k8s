package stackmon

import (
	_ "embed" // for the beats config files
	"errors"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	common_name "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/stackmon/monitoring"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

var (
	// filebeatConfig is a static configuration for Filebeat to collect Beats logs
	//go:embed filebeat.yml
	filebeatConfig string

	// metricbeatConfigTemplate is a configuration template for Metricbeat to collect monitoring data from Beats resources
	//go:embed metricbeat.yml
	metricbeatConfigTemplate string
)

func Filebeat(client k8s.Client, resource monitoring.HasMonitoring, version string) (stackmon.BeatSidecar, error) {
	filebeat, err := stackmon.NewFileBeatSidecar(client, resource, version, filebeatConfig, nil)
	if err != nil {
		return stackmon.BeatSidecar{}, err
	}

	return filebeat, nil
}

func MetricBeat(client k8s.Client, mon monitoring.HasMonitoring, version string) (stackmon.BeatSidecar, error) {
	var username, password string
	var sideCar stackmon.BeatSidecar
	var err error
	associations := monitoring.GetMetricsAssociation(mon)
	if len(associations) != 1 {
		// should never happen because of the pre-creation validation
		return sideCar, errors.New("only one Elasticsearch reference is supported for Stack Monitoring")
	}
	assoc := associations[0]

	credentials, err := association.ElasticsearchAuthSettings(client, assoc)
	if err != nil {
		return sideCar, err
	}

	username, password = credentials.Username, credentials.Password
	sideCar, err = stackmon.NewMetricBeatSidecar(
		client,
		commonv1.BeatMonitoringAssociationType,
		mon,
		version,
		metricbeatConfigTemplate,
		common_name.NewNamer("beat"),
		"http://localhost:5066",
		username,
		password,
		false,
	)
	if err != nil {
		return sideCar, err
	}
	return sideCar, nil
}
