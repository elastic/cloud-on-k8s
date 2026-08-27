// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package volume

import (
	"crypto/sha256"
	"fmt"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/name"
)

const (
	// MaxVolumeNameLength is the Kubernetes DNS-label limit for volume and volume-mount names.
	MaxVolumeNameLength = 63
)

// volumeNamer builds Kubernetes volume names capped at MaxVolumeNameLength.
var volumeNamer = name.Namer{
	MaxSuffixLength: name.MaxSuffixLength,
	MaxNameLength:   MaxVolumeNameLength,
}

func nsnHash(namespace, name string) string {
	nsn := namespace + name
	return fmt.Sprintf("%x", sha256.Sum256([]byte(nsn)))[0:6]
}

// CAVolumeName returns a DNS-label volume name for a CA certificate secret.
// prefix is typically an association type (for example "es-monitoring").
func CAVolumeName(prefix, namespace, name string) string {
	return VolumeNamespacedName(prefix, namespace, name, "ca")
}

// ClientCertVolumeName returns a DNS-label volume name for a client certificate secret.
func ClientCertVolumeName(prefix, namespace, name string) string {
	return VolumeNamespacedName(prefix, namespace, name, "client-cert")
}

// VolumeNamespacedName returns a DNS-label volume name derived from a namespace/name pair.
// The namespace and name are hashed together so the result always fits within MaxVolumeNameLength.
func VolumeNamespacedName(prefix, namespace, name, suffix string) string { //nolint:revive
	return VolumeName(prefix, nsnHash(namespace, name), suffix)
}

// VolumeName joins prefix and suffixes with hyphens and truncates the result to MaxVolumeNameLength.
func VolumeName(prefix string, suffixes ...string) string { //nolint:revive
	return volumeNamer.Suffix(prefix, suffixes...)
}
