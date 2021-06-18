// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package stackmon

import (
	"errors"
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/stretchr/testify/require"
)

func TestFullContainerImage(t *testing.T) {
	tests := []struct {
		name         string
		es           esv1.Elasticsearch
		defaultImage container.Image
		fullImage    string
		err          error
	}{
		{
			name: "with default Elasticsearch image",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Version: "7.14.1",
				},
			},
			defaultImage: "beats/filebeat",
			fullImage:    "docker.elastic.co/beats/filebeat:7.14.1",
		},
		{
			name: "with custom Elasticsearch image",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Image: "my.registry.space/elasticsearch/elasticsearch:7.15.0",
				},
			},
			defaultImage: "beats/metricbeat",
			fullImage:    "my.registry.space/beats/metricbeat:7.15.0",
		},
		{
			name: "with custom Elasticsearch image that doesn't follow the Elastic scheme",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					Image: "my.registry.space/es/es:7.14.0",
				},
			},
			defaultImage: "beats/filebeat",
			err:          errors.New("stack monitoring not supported with custom image"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			image, err := fullContainerImage(tc.es, tc.defaultImage)

			if err != nil {
				require.Error(t, tc.err)
				require.Equal(t, tc.err, err)
			} else {
				require.Equal(t, tc.fullImage, image)
			}
		})
	}
}
