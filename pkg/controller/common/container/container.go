// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package container

import (
	"fmt"
	"strings"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
)

const (
	DefaultContainerRegistry = "docker.elastic.co"

	UBISuffix    = "-ubi"  // suffix to use when --ubi-only
	OldUBISuffix = "-ubi8" // old suffix to use when --ubi-only
)

var (
	containerRegistry   = DefaultContainerRegistry
	containerRepository = ""
	containerSuffix     = ""

	major7UbiSuffixMinVersion = version.MinFor(7, 17, 16) // min 7.x to use UBISuffix
	major8UbiSuffixMinVersion = version.MinFor(8, 12, 0)  // min 8.x to use UBISuffix
)

// SetContainerRegistry sets the global container registry used to download Elastic stack images.
func SetContainerRegistry(registry string) {
	containerRegistry = registry
}

// SetContainerRepository sets a global container repository used to download Elastic stack images.
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
	AgentImagePre9        Image = "beats/elastic-agent"
	AgentImage            Image = "elastic-agent/elastic-agent"
	MapsImage             Image = "elastic-maps-service/elastic-maps-server"
	LogstashImage         Image = "logstash/logstash"
	PackageRegistryImage  Image = "package-registry/distribution"
)

var MinMapsVersionOnARM = version.MinFor(8, 16, 0)

func AgentImageFor(version version.Version) Image {
	if version.Major < 9 {
		return AgentImagePre9
	}
	return AgentImage
}

// ImageRepository returns the full container image name by concatenating the current container registry and the image path with the given version.
// A UBI suffix (-ubi8 or -ubi suffix depending on the version) is appended to the image name for the maps image,
// or any image if the operator is configured with --ubi-only.
func ImageRepository(img Image, ver version.Version) string {
	// replace repository if defined
	image := img

	if containerRepository != "" {
		image = Image(fmt.Sprintf("%s/%s", containerRepository, img.Name()))
	}

	suffix := ""
	useUBISuffix := containerSuffix == UBISuffix
	// use an UBI suffix for maps server image or any image in UBI mode
	if useUBISuffix || isOlderMapsServerImg(img, ver) {
		suffix = getUBISuffix(ver)
	}
	// Starting with 9.x ubi is the default for all stack images
	if useUBISuffix && ver.Major >= 9 {
		suffix = ""
	}
	// use the global container suffix in non-UBI mode
	if !useUBISuffix {
		suffix += containerSuffix
	}

	if img == PackageRegistryImage {
		return getPackageRegistryImage(useUBISuffix, suffix, ver)
	}

	return fmt.Sprintf("%s/%s%s:%s", containerRegistry, image, suffix, ver)
}

// isOderMapsServerImg returns true if the given image is a Maps server image and
// older than 8.16.0 as of which release the Maps server images are multi-arch similar to
// other stack images and come in non-UBI variants as well.
func isOlderMapsServerImg(img Image, ver version.Version) bool {
	return img == MapsImage && ver.LT(MinMapsVersionOnARM)
}

// getUBISuffix returns the UBI suffix to use depending on the given version.
func getUBISuffix(ver version.Version) string {
	if ver.Major == 7 && ver.LT(major7UbiSuffixMinVersion) {
		return OldUBISuffix
	}
	if ver.Major == 8 && ver.LT(major8UbiSuffixMinVersion) {
		return OldUBISuffix
	}
	return UBISuffix
}

// getPackageRegistryImage returns the Package Registry image with the appropriate tag.
// Package Registry uses by default the 'lite' image variant. Unlike other stack component
// images, UBI suffix goes in the tag (lite-X.Y.Z-ubi) and not at the end of the image name.
func getPackageRegistryImage(useUBI bool, suffix string, v version.Version) string {
	if !useUBI {
		return fmt.Sprintf("%s/%s%s:lite-%s", containerRegistry, PackageRegistryImage, suffix, v)
	}

	// Since UBI images are only offered from certain versions onwards,
	// fallback to tested backwards-compatible versions for unsupported releases
	switch {
	case v.LT(version.From(8, 19, 8)):
		// Fallback to 8.19.8-ubi for all versions below 8.19.8
		return fmt.Sprintf("%s/%s:lite-8.19.8-ubi", containerRegistry, PackageRegistryImage)
	case v.Major == 9 && v.Minor <= 1 && v.LT(version.From(9, 1, 8)):
		// Fallback to 9.1.8-ubi for 9.0.x and 9.1.x versions below 9.1.8
		return fmt.Sprintf("%s/%s:lite-9.1.8-ubi", containerRegistry, PackageRegistryImage)
	case v.Major == 9 && v.Minor > 1 && v.LT(version.From(9, 2, 2)):
		// Fallback to 9.2.2-ubi for 9.2.x versions below 9.2.2
		return fmt.Sprintf("%s/%s:lite-9.2.2-ubi", containerRegistry, PackageRegistryImage)
	default:
		// Use the requested version for all other cases (>= 9.2.2 or >= 8.19.8 non-9.x)
		return fmt.Sprintf("%s/%s%s:lite-%s-ubi", containerRegistry, PackageRegistryImage, suffix, v)
	}
}
