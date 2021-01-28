// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"context"
	"fmt"
	"testing"

	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // auth on gke
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func (b Builder) CreationTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Creating APM Server should succeed",
			Test: func(t *testing.T) {
				for _, obj := range b.RuntimeObjects() {
					err := k.Client.Create(context.Background(), obj)
					require.NoError(t, err)
				}
			},
		},
		{
			// The APM Server docker image can't run with a random user id, this step adds the SA to the anyuid SCC
			// For more context see https://github.com/elastic/beats/issues/12686
			// TODO: Must be removed when APM Server docker image is fixed
			Name: "Add apm-server service account to anyuid (OpenShift Only)",
			Test: func(t *testing.T) {
				if !test.Ctx().OcpCluster {
					return
				}

				cfg, err := config.GetConfig()
				require.NoError(t, err)
				k8sClient, err := kubernetes.NewForConfig(cfg)
				require.NoError(t, err)

				// Build the user from the service account
				user := fmt.Sprintf(`"system:serviceaccount:%s:%s"`, b.ServiceAccount.Namespace, b.ServiceAccount.Name)

				// The patch below adds the service account user in the 'users' fields of a SCC
				patch := []byte(`{ "users": [` + user + `]}`)

				// We want to patch the anyuid SCC. In term of url it means that we need to send a patch request to:
				// https://<Openshift URL>/apis/security.openshift.io/v1/securitycontextconstraints/anyuid
				patchClient := k8sClient.RESTClient().
					Patch(types.MergePatchType).
					Prefix("apis", "security.openshift.io", "v1").
					Resource("securitycontextconstraints").
					Name("anyuid").
					Body(patch)

				result := patchClient.Do(context.Background())
				require.NoError(t, result.Error())
			},
		},
		{
			Name: "APM Server should be created",
			Test: func(t *testing.T) {
				var createdApmServer apmv1.ApmServer
				err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.ApmServer), &createdApmServer)
				require.NoError(t, err)
				require.Equal(t, b.ApmServer.Spec.Version, createdApmServer.Spec.Version)
				// TODO this is incomplete
			},
		},
	}
}
