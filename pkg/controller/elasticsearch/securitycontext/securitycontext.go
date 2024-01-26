// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package securitycontext

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
)

var (
	// RunAsNonRootMinStackVersion is the minimum Stack version to use RunAsNonRoot with the Elasticsearch and Beats images.
	// Before 8.8.0 Elasticsearch and Beats images ran as a non-numeric user.
	// Refer to https://github.com/elastic/elasticsearch/pull/95390 and https://github.com/elastic/beats/pull/35272 for more information.
	RunAsNonRootMinStackVersion = version.MustParse("8.8.0-SNAPSHOT")

	// DropCapabilitiesMinStackVersion is the minimum Stack version to Drop all the capabilities.
	// Before 8.0.0 Elasticsearch image may run as root and require capabilities to change ownership
	// of copied files in initContainer and use chroot in "elasticsearch" container.
	DropCapabilitiesMinStackVersion = version.MustParse("8.0.0-SNAPSHOT")
)

func For(ver version.Version, enableReadOnlyRootFilesystem bool) corev1.SecurityContext {
	sc := corev1.SecurityContext{
		Privileged:               ptr.To[bool](false),
		ReadOnlyRootFilesystem:   ptr.To[bool](enableReadOnlyRootFilesystem),
		AllowPrivilegeEscalation: ptr.To[bool](false),
	}
	if ver.LT(DropCapabilitiesMinStackVersion) {
		return sc
	}
	sc.Capabilities = &corev1.Capabilities{
		Drop: []corev1.Capability{"ALL"},
	}
	if ver.LT(RunAsNonRootMinStackVersion) {
		return sc
	}
	sc.RunAsNonRoot = ptr.To[bool](true)
	return sc
}

func DefaultBeatSecurityContext(ver version.Version) *corev1.SecurityContext {
	sc := &corev1.SecurityContext{
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		Privileged:               ptr.To[bool](false),
		ReadOnlyRootFilesystem:   ptr.To[bool](true),
		AllowPrivilegeEscalation: ptr.To[bool](false),
	}
	if ver.LT(RunAsNonRootMinStackVersion) {
		return sc
	}
	sc.RunAsNonRoot = ptr.To[bool](true)
	return sc
}
