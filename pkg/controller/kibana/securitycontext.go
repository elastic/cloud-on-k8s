// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package kibana

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

var (
	defaultSecurityContext = corev1.SecurityContext{
		AllowPrivilegeEscalation: ptr.To(bool(false)),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{
				corev1.Capability("ALL"),
			},
		},
		Privileged:             ptr.To(bool(false)),
		ReadOnlyRootFilesystem: ptr.To(bool(true)),
		RunAsUser:              ptr.To(int64(defaultFSUser)),
		RunAsGroup:             ptr.To(int64(defaultFSGroup)),
	}
	defaultPodSecurityContext = corev1.PodSecurityContext{
		FSGroup: ptr.To(int64(defaultFSGroup)),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
)
