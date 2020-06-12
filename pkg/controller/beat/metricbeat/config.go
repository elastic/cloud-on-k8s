// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package metricbeat

import (
	"github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
)

var (
	defaultConfig = settings.MustParseConfig([]byte(
		`metricbeat:
  autodiscover:
    providers:
    - hints:
        default_config: {}
        enabled: "true"
      node: ${NODE_NAME}
      type: kubernetes
  modules:
  - module: system
    period: 10s
    metricsets:
    - cpu
    - load
    - memory
    - network
    - process
    - process_summary
    process:
      include_top_n:
        by_cpu: 5
        by_memory: 5
    processes:
    - .*
  - module: system
    period: 1m
    metricsets:
    - fsstat
processors:
- add_cloud_metadata: {}
`))
)

func (d *Driver) defaultConfig() (common.DefaultConfig, error) {
	kibanaConfig, err := common.BuildKibanaConfig(d.Client, v1beta1.BeatKibanaAssociation{Beat: &d.Beat})
	if err != nil {
		return common.DefaultConfig{}, err
	}
	return common.DefaultConfig{
		Managed:   kibanaConfig,
		Unmanaged: defaultConfig,
	}, nil
}
