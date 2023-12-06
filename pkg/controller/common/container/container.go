// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package container

import (
	"fmt"
	"strings"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
)

const DefaultContainerRegistry = "docker.elastic.co"

var (
	containerRegistry   = DefaultContainerRegistry
	containerRepository = ""
	containerSuffix     = ""

	major7UbiSuffixMinVersion = version.MinFor(7, 17, 16) // min 7.x to use UBISuffix
	major8UbiSuffixMinVersion = version.MinFor(8, 12, 0)  // min 8.x to use UBISuffix

	UBISuffix    = "-ubi"  // suffix to use when --ubi-only
	OldUBISuffix = "-ubi8" // old suffix to use when --ubi-only
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
	MapsImage             Image = "elastic-maps-service/elastic-maps-server"
	LogstashImage         Image = "logstash/logstash"
)

var ()

// ImageRepository returns the full container image name by concatenating the current container registry and the image path with the given version.
// A UBI suffix (-ubi8 or -ubi suffix depending on the version) is prepended to the image name for the maps image,
// or any image if the operator is configured with --ubi-only.
func ImageRepository(img Image, version string) string {
	// replace repository if defined
	image := img

	if containerRepository != "" {
		image = Image(fmt.Sprintf("%s/%s", containerRepository, img.Name()))
	}

	suffix := ""
	UBIOnly := containerSuffix == UBISuffix
	// use an UBI suffix for maps server image or any image in UBI mode
	if UBIOnly || img == MapsImage {
		suffix = getUBISuffix(version)
	}
	// use the global container suffix in non-UBI mode
	if !UBIOnly {
		suffix += containerSuffix
	}

	return fmt.Sprintf("%s/%s%s:%s", containerRegistry, image, suffix, version)
}

// getUBISuffix returns the UBI suffix to use depending on the given version
func getUBISuffix(v string) string {
	ver := version.MustParse(v)
	if ver.Major == 7 && ver.LT(major7UbiSuffixMinVersion) {
		return OldUBISuffix
	}
	if ver.Major == 8 && ver.LT(major8UbiSuffixMinVersion) {
		return OldUBISuffix
	}
	return UBISuffix
}
