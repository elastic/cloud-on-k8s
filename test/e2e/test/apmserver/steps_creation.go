// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"fmt"
	"testing"

	secv1client "github.com/openshift/client-go/security/clientset/versioned/typed/security/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/stretchr/testify/require"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // auth on gke
)

func (b Builder) CreationTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Creating APM Server should succeed",
			Test: func(t *testing.T) {
				for _, obj := range b.RuntimeObjects() {
					err := k.Client.Create(obj)
					require.NoError(t, err)
				}
			},
		},
		{
			Name: "Add apm-server service account to anyuid (OpenShift Only)",
			Test: func(t *testing.T) {
				if !test.Ctx().OcpCluster {
					return
				}

				cfg, err := config.GetConfig()
				require.NoError(t, err)

				secClient := secv1client.NewForConfigOrDie(cfg)
				require.NoError(t, err)

				scc, err := secClient.SecurityContextConstraints().Get("anyuid", metav1.GetOptions{})
				require.NoError(t, err)

				scc.Users = append(scc.Users, fmt.Sprintf("system:serviceaccount:%s:%s", b.ServiceAccount.GetNamespace(), b.ServiceAccount.GetName()))
				_, err = secClient.SecurityContextConstraints().Update(scc)
				require.NoError(t, err)
			},
		},
		{
			Name: "APM Server should be created",
			Test: func(t *testing.T) {
				var createdApmServer apmv1.ApmServer
				err := k.Client.Get(k8s.ExtractNamespacedName(&b.ApmServer), &createdApmServer)
				require.NoError(t, err)
				require.Equal(t, b.ApmServer.Spec.Version, createdApmServer.Spec.Version)
				// TODO this is incomplete
			},
		},
	}
}
