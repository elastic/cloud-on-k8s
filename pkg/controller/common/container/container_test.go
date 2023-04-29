// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package container

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImageRepository(t *testing.T) {
	testRegistry := "my.docker.registry.com:8080"
	testCases := []struct {
		name       string
		image      Image
		repository string
		suffix     string
		version    string
		want       string
	}{
		{
			name:    "APM server image",
			image:   APMServerImage,
			version: "7.5.2",
			want:    testRegistry + "/apm/apm-server:7.5.2",
		},
		{
			name:    "Elasticsearch image",
			image:   ElasticsearchImage,
			version: "7.5.2",
			want:    testRegistry + "/elasticsearch/elasticsearch:7.5.2",
		},
		{
			name:    "Kibana image",
			image:   KibanaImage,
			version: "7.5.2",
			want:    testRegistry + "/kibana/kibana:7.5.2",
		},
		{
			name:    "Maps image",
			image:   MapsImage,
			version: "7.12.0",
			want:    testRegistry + "/elastic-maps-service/elastic-maps-server-ubi8:7.12.0",
		},
		{
			name:    "Maps image with custom suffix",
			image:   MapsImage,
			version: "7.12.0",
			suffix:  "-ubi8",
			want:    testRegistry + "/elastic-maps-service/elastic-maps-server-ubi8:7.12.0",
		},
		{
			name:       "Elasticsearch image with custom repository",
			image:      ElasticsearchImage,
			version:    "42.0.0",
			repository: "elastic",
			want:       testRegistry + "/elastic/elasticsearch:42.0.0",
		},
		{
			name:       "Elasticsearch image with custom repository and suffix",
			image:      ElasticsearchImage,
			version:    "42.0.0",
			repository: "elastic",
			suffix:     "-obi1",
			want:       testRegistry + "/elastic/elasticsearch-obi1:42.0.0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// save and restore the current registry setting in case it has been modified
			currentRegistry := containerRegistry
			currentSuffix := containerSuffix
			defer func() {
				SetContainerRegistry(currentRegistry)
				SetContainerSuffix(currentSuffix)
			}()

			SetContainerRegistry(testRegistry)
			SetContainerRepository(tc.repository)
			SetContainerSuffix(tc.suffix)

			have := ImageRepository(tc.image, tc.version)
			assert.Equal(t, tc.want, have)
		})
	}
}
