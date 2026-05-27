// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build agent || e2e

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	commonhttp "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/http"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/agent"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/kibana"
)

// TestFleetModeWithKibanaSpace tests that an Agent can enroll into a policy defined in a non-default Kibana Space.
func TestFleetModeWithKibanaSpace(t *testing.T) {
	stackVersion := version.MustParse(test.Ctx().ElasticStackVersion)
	if stackVersion.LT(kbv1.KibanaSpacesMinVersion) {
		t.Skipf(
			"Skipping test %s because Kibana Space-scoped Fleet APIs require Stack version >= %s, current version is %s",
			t.Name(), kbv1.KibanaSpacesMinVersion, test.Ctx().ElasticStackVersion)
	}

	name := "test-agent-space"
	spaceID := "e2e-test-space"
	policyID := "e2e-space-policy"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)

	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1)

	fleetServerBuilder := agent.NewBuilder(name + "-fs").
		WithRoles(agent.AgentFleetModeRoleName).
		WithOpenShiftRoles(test.UseSCCRole).
		WithDeployment().
		WithFleetMode().
		WithFleetServer().
		WithElasticsearchRefs(agent.ToOutput(esBuilder.Ref(), "default")).
		WithKibanaRef(kbBuilder.Ref()).
		WithFleetAgentDataStreamsValidation().
		// Validate system metrics from the agent enrolled with the system integration
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.cpu", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.memory", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.load", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.uptime", "default"))

	kbBuilder = kbBuilder.WithConfig(fleetConfigForKibana(t, fleetServerBuilder.Agent.Spec.Version, esBuilder.Ref(), fleetServerBuilder.Ref(), true))

	// Wrap Kibana builder to add space and policy setup after Kibana check steps
	kbWrapped := test.WrappedBuilder{
		BuildingThis: kbBuilder,
		PostCheckSteps: func(k *test.K8sClient) test.StepList {
			return test.StepList{
				{
					Name: "Create Kibana Space and Agent Policy with integrations",
					Test: test.Eventually(func() error {
						return setupKibanaSpaceAndPolicy(k, kbBuilder.Kibana, esBuilder.Elasticsearch.Name, spaceID, policyID)
					}),
				},
			}
		},
	}

	// Agent that will enroll into a policy in a custom space
	agentBuilder := agent.NewBuilder(name + "-ea").
		WithRoles(agent.AgentFleetModeRoleName).
		WithOpenShiftRoles(test.UseSCCRole).
		WithFleetMode().
		WithKibanaRef(kbBuilder.Ref()).
		WithFleetServerRef(fleetServerBuilder.Ref()).
		WithPolicyID(policyID).
		WithSpaceID(spaceID)

	fleetServerBuilder = agent.ApplyYamls(t, fleetServerBuilder, "", E2EAgentFleetModePodTemplate)
	agentBuilder = agent.ApplyYamls(t, agentBuilder, "", E2EAgentFleetModePodTemplate)

	test.Sequence(nil, test.EmptySteps, esBuilder, kbWrapped, fleetServerBuilder, agentBuilder).RunSequential(t)
}

// setupKibanaSpaceAndPolicy creates a Kibana Space and an Agent Policy with integrations within that space.
func setupKibanaSpaceAndPolicy(k *test.K8sClient, kb kbv1.Kibana, esClusterName, spaceID, policyID string) error {
	var secret corev1.Secret
	secretKey := types.NamespacedName{
		Namespace: kb.Namespace,
		Name:      esv1.ElasticUserSecret(esClusterName),
	}
	if err := k.Client.Get(context.Background(), secretKey, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("elastic user secret %s not found yet", secretKey.Name)
		}
		return err
	}
	password := string(secret.Data["elastic"])

	// Create the Space
	spaceBody := fmt.Sprintf(`{"id": %q, "name": %q, "description": "E2E test space"}`, spaceID, spaceID)
	_, _, err := kibana.DoRequest(k, kb, password, http.MethodPost, "/api/spaces/space", []byte(spaceBody), nil)
	if err != nil && !isConflictError(err) {
		return fmt.Errorf("failed to create Kibana Space: %w", err)
	}

	// Get the installed package versions
	systemPkgVersion, err := getInstalledPackageVersion(k, kb, password, "system")
	if err != nil {
		return fmt.Errorf("failed to get system package version: %w", err)
	}
	k8sPkgVersion, err := getInstalledPackageVersion(k, kb, password, "kubernetes")
	if err != nil {
		return fmt.Errorf("failed to get kubernetes package version: %w", err)
	}

	// Create the Agent Policy in the custom space
	policyBody := fmt.Sprintf(`{
		"id": %q,
		"name": "E2E Test Policy in Space",
		"namespace": "default",
		"monitoring_enabled": ["logs", "metrics"]
	}`, policyID)

	spaceScopedPath := fmt.Sprintf("/s/%s/api/fleet/agent_policies", spaceID)
	_, _, err = kibana.DoRequest(k, kb, password, http.MethodPost, spaceScopedPath, []byte(policyBody), nil)
	if err != nil && !isConflictError(err) {
		return fmt.Errorf("failed to create Agent Policy in space %s: %w", spaceID, err)
	}

	packagePolicyPath := fmt.Sprintf("/s/%s/api/fleet/package_policies", spaceID)

	// Add the system package policy with system metrics enabled
	systemPackagePolicy := fmt.Sprintf(`{
		"name": "system-1",
		"namespace": "default",
		"policy_id": %q,
		"package": {
			"name": "system",
			"version": %q
		},
		"inputs": {
			"system-system/metrics": {
				"enabled": true,
				"streams": {
					"system.cpu": {"enabled": true},
					"system.memory": {"enabled": true},
					"system.load": {"enabled": true},
					"system.uptime": {"enabled": true}
				}
			}
		}
	}`, policyID, systemPkgVersion)

	_, _, err = kibana.DoRequest(k, kb, password, http.MethodPost, packagePolicyPath, []byte(systemPackagePolicy), nil)
	if err != nil && !isConflictError(err) {
		return fmt.Errorf("failed to add system package to policy in space %s: %w", spaceID, err)
	}

	// Add the kubernetes package policy with container logs enabled to spawn a filebeat subprocess
	k8sPackagePolicy := fmt.Sprintf(`{
		"name": "kubernetes-1",
		"namespace": "default",
		"policy_id": %q,
		"package": {
			"name": "kubernetes",
			"version": %q
		},
		"inputs": {
			"container-logs-filestream": {
				"enabled": true,
				"streams": {
					"kubernetes.container_logs": {
						"enabled": true,
						"vars": {
							"paths": ["/var/log/containers/*${kubernetes.container.id}.log"],
							"symlinks": true,
							"data_stream.dataset": "kubernetes.container_logs"
						}
					}
				}
			}
		}
	}`, policyID, k8sPkgVersion)

	_, _, err = kibana.DoRequest(k, kb, password, http.MethodPost, packagePolicyPath, []byte(k8sPackagePolicy), nil)
	if err != nil && !isConflictError(err) {
		return fmt.Errorf("failed to add kubernetes package to policy in space %s: %w", spaceID, err)
	}

	return nil
}

// getInstalledPackageVersion retrieves the version of an installed Fleet package.
func getInstalledPackageVersion(k *test.K8sClient, kb kbv1.Kibana, password, packageName string) (string, error) {
	respBody, _, err := kibana.DoRequest(k, kb, password, http.MethodGet, "/api/fleet/epm/packages/"+packageName, nil, nil)
	if err != nil {
		return "", err
	}

	var resp struct {
		Item struct {
			Version string `json:"version"`
		} `json:"item"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return "", fmt.Errorf("failed to parse package response: %w", err)
	}

	if resp.Item.Version == "" {
		return "", fmt.Errorf("package %s not installed or version not found", packageName)
	}

	return resp.Item.Version, nil
}

func isConflictError(err error) bool {
	var apiErr *commonhttp.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict
}
