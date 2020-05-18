// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package container

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImageRepository(t *testing.T) {
	testRegistry := "my.docker.registry.com:8080"
	testCases := []struct {
		name    string
		image   Image
		version string
		want    string
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// save and restore the current registry setting in case it has been modified
			currentRegistry := containerRegistry
			defer func() {
				SetContainerRegistry(currentRegistry)
			}()

			SetContainerRegistry(testRegistry)
			have := ImageRepository(tc.image, tc.version)
			assert.Equal(t, tc.want, have)
		})
	}
}
