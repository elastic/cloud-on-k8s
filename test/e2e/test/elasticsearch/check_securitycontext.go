// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package elasticsearch

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	ptr "k8s.io/utils/pointer"

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
					asserSecurityContext(t, ver, c.SecurityContext)
				}
				for _, c := range p.Spec.InitContainers {
					asserSecurityContext(t, ver, c.SecurityContext)
				}
			}
		},
	}
}

func asserSecurityContext(t *testing.T, ver version.Version, securityContext *corev1.SecurityContext) {
	t.Helper()
	require.NotNil(t, securityContext)
	if ver.LT(securitycontext.MinStackVersion) {
		require.Nil(t, securityContext.RunAsNonRoot)
	} else {
		require.Equal(t, ptr.Bool(true), securityContext.RunAsNonRoot)
	}
	require.NotNil(t, securityContext.Privileged)
	require.False(t, *securityContext.Privileged)
	require.Equal(t, securityContext.Capabilities, &corev1.Capabilities{
		Drop: []corev1.Capability{"ALL"},
	})
}
