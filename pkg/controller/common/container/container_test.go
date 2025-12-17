// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package container

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
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
			name:    "APM server UBI image before 9.x",
			image:   APMServerImage,
			version: "8.99.99",
			suffix:  "-ubi",
			want:    testRegistry + "/apm/apm-server-ubi:8.99.99",
		},
		{
			name:    "APM server UBI image since 9.x",
			image:   APMServerImage,
			version: "9.0.0",
			suffix:  "-ubi",
			want:    testRegistry + "/apm/apm-server:9.0.0",
		},
		{
			name:    "Kibana image",
			image:   KibanaImage,
			version: "7.5.2",
			want:    testRegistry + "/kibana/kibana:7.5.2",
		},
		{
			name:    "Elasticsearch image",
			image:   ElasticsearchImage,
			version: "7.5.2",
			want:    testRegistry + "/elasticsearch/elasticsearch:7.5.2",
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
		{
			name:       "Elasticsearch 9 image in ubi mode",
			image:      ElasticsearchImage,
			version:    "9.0.0",
			repository: "elastic",
			suffix:     "-ubi",
			want:       testRegistry + "/elastic/elasticsearch:9.0.0",
		},
		{
			name:       "Elasticsearch 8 image in ubi mode",
			image:      ElasticsearchImage,
			version:    "8.12.0",
			repository: "elastic",
			suffix:     "-ubi",
			want:       testRegistry + "/elastic/elasticsearch-ubi:8.12.0",
		},
		{
			name:    "Elasticsearch old 8 image in ubi mode uses old -ubi8 suffix",
			image:   ElasticsearchImage,
			version: "8.11.0",
			suffix:  "-ubi",
			want:    testRegistry + "/elasticsearch/elasticsearch-ubi8:8.11.0",
		},
		{
			name:       "Elasticsearch 7 image in ubi mode",
			image:      ElasticsearchImage,
			version:    "7.17.16",
			repository: "elastic",
			suffix:     "-ubi",
			want:       testRegistry + "/elastic/elasticsearch-ubi:7.17.16",
		},
		{
			name:    "Elasticsearch old 7 image in ubi mode uses old -ubi8 suffix",
			image:   ElasticsearchImage,
			version: "7.17.15",
			suffix:  "-ubi",
			want:    testRegistry + "/elasticsearch/elasticsearch-ubi8:7.17.15",
		},
		{
			name:       "Maps 7 image with custom repository always uses -ubi suffix",
			image:      MapsImage,
			repository: "elastic",
			version:    "7.17.16",
			want:       testRegistry + "/elastic/elastic-maps-server-ubi:7.17.16",
		},
		{
			name:       "Maps old 7 image with custom repository always uses -ubi8 suffix",
			image:      MapsImage,
			repository: "elastic",
			version:    "7.17.15",
			want:       testRegistry + "/elastic/elastic-maps-server-ubi8:7.17.15",
		},
		{
			name:    "Maps 8 image in ubi mode ignores the -ubi suffix",
			image:   MapsImage,
			version: "8.12.0",
			suffix:  "-ubi",
			want:    testRegistry + "/elastic-maps-service/elastic-maps-server-ubi:8.12.0",
		},
		{
			name:    "Maps old 8 image in ubi mode ignores the -ubi suffix",
			image:   MapsImage,
			version: "8.11.0",
			suffix:  "-ubi",
			want:    testRegistry + "/elastic-maps-service/elastic-maps-server-ubi8:8.11.0",
		},
		{
			name:    "Maps 7 image in ubi mode ignores the -ubi suffix",
			image:   MapsImage,
			version: "7.17.16",
			suffix:  "-ubi",
			want:    testRegistry + "/elastic-maps-service/elastic-maps-server-ubi:7.17.16",
		},
		{
			name:    "Maps old 7 image in ubi mode ignores the -ubi suffix",
			image:   MapsImage,
			version: "7.17.15",
			suffix:  "-ubi",
			want:    testRegistry + "/elastic-maps-service/elastic-maps-server-ubi8:7.17.15",
		},
		{
			name:    "Maps 8 image with custom suffix",
			image:   MapsImage,
			version: "8.12.0",
			suffix:  "-obi1",
			want:    testRegistry + "/elastic-maps-service/elastic-maps-server-ubi-obi1:8.12.0",
		},
		{
			name:    "Maps old 8 image with custom suffix",
			image:   MapsImage,
			version: "8.11.0",
			suffix:  "-obi1",
			want:    testRegistry + "/elastic-maps-service/elastic-maps-server-ubi8-obi1:8.11.0",
		},
		{
			name:    "Maps 8 image post 8.16 wolfi-based",
			image:   MapsImage,
			version: "8.16.0",
			want:    testRegistry + "/elastic-maps-service/elastic-maps-server:8.16.0",
		},
		{
			name:    "Maps 8 image post 8.16 ubi requested",
			image:   MapsImage,
			version: "8.16.0",
			suffix:  "-ubi",
			want:    testRegistry + "/elastic-maps-service/elastic-maps-server-ubi:8.16.0",
		},
		{
			name:    "Package registry image",
			image:   PackageRegistryImage,
			version: "1.0.0",
			want:    testRegistry + "/package-registry/distribution:lite-1.0.0",
		},
		{
			name:    "Package registry image -ubi suffix",
			image:   PackageRegistryImage,
			version: "1.0.0",
			suffix:  "-ubi",
			want:    testRegistry + "/package-registry/distribution:lite-1.0.0-ubi",
		},
		{
			name:    "Package registry image -random suffix",
			image:   PackageRegistryImage,
			version: "1.0.0",
			suffix:  "-random",
			want:    testRegistry + "/package-registry/distribution-random:lite-1.0.0",
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

			have := ImageRepository(tc.image, version.MustParse(tc.version))
			assert.Equal(t, tc.want, have)
		})
	}
}

func TestAgentImageFor(t *testing.T) {
	type args struct {
		version version.Version
	}
	tests := []struct {
		name string
		args args
		want Image
	}{
		{
			name: "New default elastic-agent/elastic-agent ",
			args: args{
				version: version.MustParse("9.5.0"),
			},
			want: "elastic-agent/elastic-agent",
		},
		{
			name: "Legacy image in beats namespace priot to 9.0",
			args: args{
				version: version.MustParse("8.0.0"),
			},
			want: "beats/elastic-agent",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AgentImageFor(tt.args.version); got != tt.want {
				t.Errorf("AgentImageFor() = %v, want %v", got, tt.want)
			}
		})
	}
}
