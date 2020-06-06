// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package filebeat

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
)

var (
	k8sAutodiscoverPresetConfig = settings.MustParseConfig([]byte(
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
