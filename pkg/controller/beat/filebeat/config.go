// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package filebeat

import (
	"github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/beat/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
)

var (
	defaultConfig = settings.MustParseConfig([]byte(
		`filebeat:
  autodiscover:
    providers:
    - type: kubernetes
      node: ${NODE_NAME}
      hints:
        enabled: true
        default_config:
          type: container
          paths:
          - /var/log/containers/*${data.kubernetes.container.id}.log
processors:
- add_cloud_metadata: {}
- add_host_metadata: {}
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
