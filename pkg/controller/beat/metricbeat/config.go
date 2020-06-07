// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package metricbeat

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
)

var (
	k8sHostsPresetConfig = settings.MustParseConfig([]byte(
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
    - filesystem
    - fsstat
    processors:
    - drop_event:
        when:
          regexp:
            system:
              filesystem:
                mount_point: ^/(sys|cgroup|proc|dev|etc|host|lib)($|/)
  - module: kubernetes
    period: 10s
    host: ${NODE_NAME}
    hosts:
    - https://${HOSTNAME}:10250
    bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
    ssl:
      verification_mode: none
    metricsets:
    - node
    - system
    - pod
    - container
    - volume
processors:
- add_cloud_metadata: {}
- add_host_metadata: {}
`))
)
