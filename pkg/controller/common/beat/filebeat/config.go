// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package filebeat

import (
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
)

var (
	defaultConfig = settings.MustCanonicalConfig(map[string]interface{}{
		"filebeat": map[string]interface{}{
			"autodiscover": map[string]interface{}{
				"providers": []map[string]interface{}{
					{
						"type": "kubernetes",
						"node": "${NODE_NAME}",
						"hints": map[string]interface{}{
							"enabled": "true",
							"default_config": map[string]interface{}{
								"type":  "container",
								"paths": []string{"/var/log/containers/*${data.kubernetes.container.id}.log"},
							},
						},
					},
				},
			},
		},
		"processors": []map[string]interface{}{
			{"add_cloud_metadata": nil},
			{"add_host_metadata": nil},
		},
	})
)
