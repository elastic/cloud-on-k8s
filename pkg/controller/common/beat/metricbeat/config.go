// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package metricbeat

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
)

var (
	defaultConfig = settings.MustCanonicalConfig(map[string]interface{}{
		"metricbeat": map[string]interface{}{
			"autodiscover": map[string]interface{}{
				"providers": []map[string]interface{}{
					{
						"type": "kubernetes",
						"node": "${NODE_NAME}",
						"hints": map[string]interface{}{
							"enabled":        "true",
							"default_config": map[string]interface{}{},
						},
					},
				},
			},
			"modules": []map[string]interface{}{
				{
					"module":     "system",
					"period":     "10s",
					"metricsets": []string{"cpu", "load", "memory", "network", "process", "process_summary"},
					"processes":  []string{".*"},
					"process.include_top_n": map[string]interface{}{
						"by_cpu":    5,
						"by_memory": 5,
					},
				},
				{
					"module":     "system",
					"period":     "1m",
					"metricsets": []string{"filesystem", "fsstat"},
					"processors": []map[string]interface{}{
						{
							"drop_event.when.regexp": map[string]interface{}{
								"system.filesystem.mount_point": "^/(sys|cgroup|proc|dev|etc|host|lib)($|/)",
							},
						},
					},
				},
				{
					"module":                "kubernetes",
					"period":                "10s",
					"metricsets":            []string{"node", "system", "pod", "container", "volume"},
					"host":                  "${NODE_NAME}",
					"hosts":                 []string{"https://${HOSTNAME}:10250"},
					"bearer_token_file":     "/var/run/secrets/kubernetes.io/serviceaccount/token",
					"ssl.verification_mode": "none",
				},
				{
					"module":     "kubernetes",
					"period":     "10s",
					"metricsets": []string{"proxy"},
					"host":       "${NODE_NAME}",
					"hosts":      []string{"https://${HOSTNAME}:10249"},
				},
			},
		},
		"setup.dashboards.enabled":                 "true",
		"setup.kibana.host":                        "https://kibana-sample-kb-http.default.svc:5601",
		"setup.kibana.username":                    "elastic",
		"setup.kibana.password":                    "x4rVl41N48C6IJS3sW8L27Uf",
		"setup.kibana.ssl.certificate_authorities": "/mnt/elastic-internal/kibana-certs/ca.crt",
		"processors": []map[string]interface{}{
			{"add_cloud_metadata": nil},
			//{"add_host_metadata": nil},
		},
	})
)

/*
module: kubernetes
      metricsets:
        - node
        - system
        - pod
        - container
        - volume
      period: 10s
      host: ${NODE_NAME}
      hosts: ["https://${HOSTNAME}:10250"]
      bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
      ssl.verification_mode: "none"
      # If using Red Hat OpenShift remove ssl.verification_mode entry and
      # uncomment these settings:
      #ssl.certificate_authorities:
        #- /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
    - module: kubernetes
      metricsets:
        - proxy
      period: 10s
      host: ${NODE_NAME}
      hosts: ["localhost:10249"]

*/

/*
   metricbeat.config.modules:
     # Mounted `metricbeat-daemonset-modules` configmap:
     path: ${path.config}/modules.d/*.yml
     # Reload module configs as they change:
     reload.enabled: false

   # To enable hints based autodiscover uncomment this:
   #metricbeat.autodiscover:
   #  providers:
   #    - type: kubernetes
   #      node: ${NODE_NAME}
   #      hints.enabled: true

   processors:
     - add_cloud_metadata:


- module: system
      period: 10s
      metricsets:
        - cpu
        - load
        - memory
        - network
        - process
        - process_summary
        #- core
        #- diskio
        #- socket
      processes: ['.*']
      process.include_top_n:
        by_cpu: 5      # include top 5 processes by CPU
        by_memory: 5   # include top 5 processes by memory

    - module: system
      period: 1m
      metricsets:
        - filesystem
        - fsstat
      processors:
      - drop_event.when.regexp:
          system.filesystem.mount_point: '^/(sys|cgroup|proc|dev|etc|host|lib)($|/)'

- module: kubernetes
      metricsets:
        - node
        - system
        - pod
        - container
        - volume
      period: 10s
      host: ${NODE_NAME}
      hosts: ["https://${HOSTNAME}:10250"]
      bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
      ssl.verification_mode: "none"
      # If using Red Hat OpenShift remove ssl.verification_mode entry and
      # uncomment these settings:
      #ssl.certificate_authorities:
        #- /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
    - module: kubernetes
      metricsets:
        - proxy
      period: 10s
      host: ${NODE_NAME}
      hosts: ["localhost:10249"]


*/
