// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package securitycontext

import (
	corev1 "k8s.io/api/core/v1"
	ptr "k8s.io/utils/pointer"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
)

var (
	// MinStackVersion is the minimum Stack version to use RunAsNonRoot with the Elasticsearch image.
	// Before 8.8.0 Elasticsearch image runs has non-numeric user.
	// Refer to https://github.com/elastic/elasticsearch/pull/95390 for more information.
	MinStackVersion = version.MustParse("8.8.0-SNAPSHOT")
)

func For(ver version.Version, enableReadOnlyRootFilesystem bool) corev1.SecurityContext {
	sc := corev1.SecurityContext{
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		Privileged:               ptr.Bool(false),
		ReadOnlyRootFilesystem:   ptr.Bool(enableReadOnlyRootFilesystem),
		AllowPrivilegeEscalation: ptr.Bool(false),
	}
	if ver.LT(MinStackVersion) {
		return sc
	}
	sc.RunAsNonRoot = ptr.Bool(true)
	return sc
}

func DefaultBeatSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		Privileged:               ptr.Bool(false),
		RunAsNonRoot:             ptr.Bool(true),
		ReadOnlyRootFilesystem:   ptr.Bool(true),
		AllowPrivilegeEscalation: ptr.Bool(false),
	}
}
