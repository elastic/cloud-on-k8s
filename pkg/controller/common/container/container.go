// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package container

import (
	"fmt"
	"strings"
)

const DefaultContainerRegistry = "docker.elastic.co"

var (
	containerRegistry   = DefaultContainerRegistry
	containerRepository = ""
	containerSuffix     = ""
)

// SetContainerRegistry sets the global container registry used to download Elastic stack images.
func SetContainerRegistry(registry string) {
	containerRegistry = registry
}

// SetContainerRegistry sets a global container repository used to download Elastic stack images.
func SetContainerRepository(repository string) {
	containerRepository = repository
}

func SetContainerSuffix(suffix string) {
	containerSuffix = suffix
}

type Image string

func (i Image) Name() string {
	parts := strings.Split(string(i), "/")
	if len(parts) == 2 {
		return parts[1]
	}
	return string(i)
}

const (
	APMServerImage        Image = "apm/apm-server"
	ElasticsearchImage    Image = "elasticsearch/elasticsearch"
	KibanaImage           Image = "kibana/kibana"
	EnterpriseSearchImage Image = "enterprise-search/enterprise-search"
	FilebeatImage         Image = "beats/filebeat"
	MetricbeatImage       Image = "beats/metricbeat"
	HeartbeatImage        Image = "beats/heartbeat"
	AuditbeatImage        Image = "beats/auditbeat"
	JournalbeatImage      Image = "beats/journalbeat"
	PacketbeatImage       Image = "beats/packetbeat"
	AgentImage            Image = "beats/elastic-agent"
	MapsImage             Image = "elastic-maps-service/elastic-maps-server-ubi8"
	LogstashImage         Image = "logstash/logstash"
)

// ImageRepository returns the full container image name by concatenating the current container registry and the image path with the given version.
func ImageRepository(img Image, version string) string {
	// replace repository if defined
	image := img

	if containerRepository != "" {
		image = Image(fmt.Sprintf("%s/%s", containerRepository, img.Name()))
	}

	// don't double append suffix if already contained as e.g. the case for maps
	if strings.HasSuffix(string(img), containerSuffix) {
		return fmt.Sprintf("%s/%s:%s", containerRegistry, image, version)
	}
	return fmt.Sprintf("%s/%s%s:%s", containerRegistry, image, containerSuffix, version)
}
