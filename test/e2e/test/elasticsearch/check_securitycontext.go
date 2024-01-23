// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/securitycontext"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

func CheckContainerSecurityContext(es esv1.Elasticsearch, k *test.K8sClient) test.Step {
	//nolint:thelper
	return test.Step{
		Name: "Elasticsearch containers SecurityContext should be set",
		Test: func(t *testing.T) {
			usesEmptyDir := usesEmptyDir(es)
			if usesEmptyDir {
				return
			}
			pods, err := k.GetPods(test.ESPodListOptions(es.Namespace, es.Name)...)
			require.NoError(t, err)

			ver := version.MustParse(es.Spec.Version)
			for _, p := range pods {
				for _, c := range p.Spec.Containers {
					assertSecurityContext(t, ver, c.SecurityContext)
				}
				for _, c := range p.Spec.InitContainers {
					assertSecurityContext(t, ver, c.SecurityContext)
				}
			}
		},
	}
}

func assertSecurityContext(t *testing.T, ver version.Version, securityContext *corev1.SecurityContext) {
	t.Helper()
	require.NotNil(t, securityContext)
	if ver.LT(securitycontext.RunAsNonRootMinStackVersion) {
		require.Nil(t, securityContext.RunAsNonRoot, "RunAsNonRoot was expected to be nil")
	} else {
		require.Equal(t, ptr.To[bool](true), securityContext.RunAsNonRoot, "RunAsNonRoot was expected to be true")
	}
	require.NotNil(t, securityContext.Privileged)
	require.False(t, *securityContext.Privileged)

	if ver.LT(version.MinFor(8, 0, 0)) {
		// We are not expecting Capabilities to be changed by the operator before 8.x
		// Also refer to https://github.com/elastic/cloud-on-k8s/pull/6755
		return
	}

	// OpenShift may add others Capabilities. We only check that ALL is included in "Drop".
	require.NotNil(t, securityContext.Capabilities)
	droppedCapabilities := securityContext.Capabilities.Drop
	hasDropAllCapability := false
	for _, capability := range droppedCapabilities {
		if capability == "ALL" {
			hasDropAllCapability = true
			break
		}
	}
	require.True(t, hasDropAllCapability, "ALL capability not found in securityContext.Capabilities.Drop")
}
