// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package container

import (
	"fmt"
)

const DefaultContainerRegistry = "docker.elastic.co"

var containerRegistry = DefaultContainerRegistry

// SetContainerRegistry sets the global container registry used to download Elastic stack images.
func SetContainerRegistry(registry string) {
	containerRegistry = registry
}

type Image string

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
)

// ImageRepository returns the full container image name by concatenating the current container registry and the image path with the given version.
func ImageRepository(img Image, version string) string {
	return fmt.Sprintf("%s/%s:%s", containerRegistry, img, version)
}
