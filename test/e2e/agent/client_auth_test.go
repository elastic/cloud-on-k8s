// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build agent || e2e

package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/ghodss/yaml"
	"k8s.io/apimachinery/pkg/types"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	agentcontroller "github.com/elastic/cloud-on-k8s/v3/pkg/controller/association/controller"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/agent"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/client-auth"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/helper"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/kibana"
)

// TestClientAuthTransition_StandaloneAgent tests that when Elasticsearch transitions from client authentication
// required to disabled, a standalone Agent remains healthy and its client certificate secrets are cleaned up.
func TestClientAuthRequiredTransition_StandaloneAgent(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping client authentication test: no enterprise test license configured")
	}

	name := "test-sa-mtls-trans"
	namespace := test.Ctx().ManagedNamespace(0)

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired().
		TolerateMutationChecksFailures()

	agentBuilder := agent.NewBuilder(name).
		WithElasticsearchRefs(agent.ToOutput(esBuilder.Ref(), "default")).
		WithOpenShiftRoles(test.UseSCCRole).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent.filebeat", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent.metricbeat", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.cpu", "default"))

	agentBuilder = agent.ApplyYamls(t, agentBuilder, E2EAgentSystemIntegrationConfig, E2EAgentSystemIntegrationPodTemplate).MoreResourcesForIssue4730()

	// Wrap the ES builder with license setup.
	esWithLicense := test.LicenseTestBuilder(esBuilder)
	// 1 client certificate; elastic-agent
	esWithLicense.PostCheckSteps = func(k *test.K8sClient) test.StepList {
		return test.StepList{clientauth.CheckClientCertificatesCountStep(k, namespace, esBuilder.Elasticsearch.Name, 1)}
	}

	// Transition ES to client auth disabled.
	esMutated := esBuilder.DeepCopy().WithMutatedFrom(&esBuilder)
	esMutated.Elasticsearch.Spec.HTTP.TLS.Client.Authentication = false

	esMutatedWrapped := test.WrappedBuilder{
		BuildingThis: esMutated,
		PostMutationSteps: func(k *test.K8sClient) test.StepList {
			return test.CheckTestSteps(agentBuilder, k).
				WithSteps(test.StepList{
					clientauth.CheckClientCertificatesCountStep(k, namespace, esBuilder.Elasticsearch.Name, 0),
				})
		},
	}

	test.RunMutations(t, []test.Builder{esWithLicense, agentBuilder}, []test.Builder{esMutatedWrapped})
}

// TestClientAuthCustomCertificate_StandaloneAgent tests that a standalone Agent works with a user-provided
// client certificate when Elasticsearch requires client authentication.
func TestClientAuthRequiredCustomCertificate_StandaloneAgent(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping client authentication test: no enterprise test license configured")
	}

	name := "test-sa-mtls-custom"
	namespace := test.Ctx().ManagedNamespace(0)
	userCertSecretName := name + "-user-client-cert"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired().
		TolerateMutationChecksFailures()

	agentBuilder := agent.NewBuilder(name).
		WithElasticsearchRefs(agent.ToOutputWithClientCert(
			commonv1.ObjectSelector{Name: esBuilder.Elasticsearch.Name, Namespace: esBuilder.Elasticsearch.Namespace},
			userCertSecretName, "default")).
		WithOpenShiftRoles(test.UseSCCRole).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent.filebeat", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.LogsType, "elastic_agent.metricbeat", "default")).
		WithDefaultESValidation(agent.HasWorkingDataStream(agent.MetricsType, "system.cpu", "default"))

	agentBuilder = agent.ApplyYamls(t, agentBuilder, E2EAgentSystemIntegrationConfig, E2EAgentSystemIntegrationPodTemplate).MoreResourcesForIssue4730()

	certPEM, keyPEM := helper.GenerateSelfSignedClientCert(t, name)

	agentWrapped := test.WrappedBuilder{
		BuildingThis: agentBuilder,
		PostCheckSteps: func(k *test.K8sClient) test.StepList {
			return test.StepList{clientauth.CheckClientCertificateDataStep(k, namespace, esBuilder.Elasticsearch.Name,
				agentcontroller.AgentAssociationLabelName, agentBuilder.Agent.Name, certPEM, keyPEM)}
		},
	}

	before, after := clientauth.UserCustomCertificateSecretLifecycleSteps(namespace, userCertSecretName, certPEM, keyPEM)

	test.BeforeAfterSequence(before, after, test.LicenseTestBuilder(esBuilder), agentWrapped).RunSequential(t)
}

// TestClientAuthTransition_FleetAgent tests that when Elasticsearch transitions from client authentication
// required to disabled, a fleet-managed Agent (and its Fleet Server) remain healthy and transitive client
// certificate secrets are cleaned up.
func TestClientAuthRequiredTransition_FleetAgent(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping client authentication test: no enterprise test license configured")
	}

	name := "test-fa-mtls-trans"
	namespace := test.Ctx().ManagedNamespace(0)

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired().
		TolerateMutationChecksFailures()

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
		WithFleetAgentDataStreamsValidation()

	kbBuilder = kbBuilder.WithConfig(fleetConfigWithOutputsForKibana(t, fleetServerBuilder.Agent.Spec.Version, esBuilder.Ref(), fleetServerBuilder.Ref()))

	agentBuilder := agent.NewBuilder(name + "-ea").
		WithRoles(agent.AgentFleetModeRoleName).
		WithOpenShiftRoles(test.UseSCCRole).
		WithFleetMode().
		WithKibanaRef(kbBuilder.Ref()).
		WithFleetServerRef(fleetServerBuilder.Ref())

	fleetServerBuilder = agent.ApplyYamls(t, fleetServerBuilder, "", E2EAgentFleetModePodTemplate)
	agentBuilder = agent.ApplyYamls(t, agentBuilder, "", E2EAgentFleetModePodTemplate)

	// Wrap the ES builder with license setup.
	esWithLicense := test.LicenseTestBuilder(esBuilder)
	// 3 client certificates; Kibana, fleet-server and elastic-agent
	esWithLicense.PostCheckSteps = func(k *test.K8sClient) test.StepList {
		return test.StepList{clientauth.CheckClientCertificatesCountStep(k, namespace, esBuilder.Elasticsearch.Name, 3)}
	}

	// Transition ES to client auth disabled.
	esMutated := esBuilder.DeepCopy().WithMutatedFrom(&esBuilder)
	esMutated.Elasticsearch.Spec.HTTP.TLS.Client.Authentication = false

	esMutatedWrapped := test.WrappedBuilder{
		BuildingThis: esMutated,
		PostMutationSteps: func(k *test.K8sClient) test.StepList {
			return test.CheckTestSteps(kbBuilder, k).
				WithSteps(test.CheckTestSteps(fleetServerBuilder, k)).
				WithSteps(test.CheckTestSteps(agentBuilder, k)).
				WithSteps(test.StepList{
					clientauth.CheckClientCertificatesCountStep(k, namespace, esBuilder.Elasticsearch.Name, 0),
				})
		},
	}

	test.RunMutations(t, []test.Builder{esWithLicense, kbBuilder, fleetServerBuilder, agentBuilder}, []test.Builder{esMutatedWrapped})
}

// TestClientAuthCustomCertificate_FleetAgent tests that a fleet-managed Agent uses the same user-provided
// client certificate as its Fleet Server when Elasticsearch requires client authentication.
func TestClientAuthRequiredCustomCertificate_FleetAgent(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping client authentication test: no enterprise test license configured")
	}

	name := "test-fa-mtls-custom"
	namespace := test.Ctx().ManagedNamespace(0)
	userCertSecretName := name + "-user-client-cert"

	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources).
		WithClientAuthenticationRequired().
		TolerateMutationChecksFailures()

	kbBuilder := kibana.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1)

	fleetServerBuilder := agent.NewBuilder(name + "-fs").
		WithRoles(agent.AgentFleetModeRoleName).
		WithOpenShiftRoles(test.UseSCCRole).
		WithDeployment().
		WithFleetMode().
		WithFleetServer().
		WithElasticsearchRefs(agent.ToOutputWithClientCert(
			commonv1.ObjectSelector{Name: esBuilder.Elasticsearch.Name, Namespace: esBuilder.Elasticsearch.Namespace},
			userCertSecretName, "default")).
		WithKibanaRef(kbBuilder.Ref()).
		WithFleetAgentDataStreamsValidation()

	kbBuilder = kbBuilder.WithConfig(fleetConfigWithOutputsForKibana(t, fleetServerBuilder.Agent.Spec.Version, esBuilder.Ref(), fleetServerBuilder.Ref()))

	agentBuilder := agent.NewBuilder(name + "-ea").
		WithRoles(agent.AgentFleetModeRoleName).
		WithOpenShiftRoles(test.UseSCCRole).
		WithFleetMode().
		WithKibanaRef(kbBuilder.Ref()).
		WithFleetServerRef(fleetServerBuilder.Ref())

	fleetServerBuilder = agent.ApplyYamls(t, fleetServerBuilder, "", E2EAgentFleetModePodTemplate)
	agentBuilder = agent.ApplyYamls(t, agentBuilder, "", E2EAgentFleetModePodTemplate)

	certPEM, keyPEM := helper.GenerateSelfSignedClientCert(t, name)

	// Wrap the agent builder to add post-check verification steps.
	agentWrapped := test.WrappedBuilder{
		BuildingThis: agentBuilder,
		PostCheckSteps: func(k *test.K8sClient) test.StepList {
			return test.StepList{clientauth.CheckClientCertificateDataStep(k, namespace, esBuilder.Elasticsearch.Name, agentcontroller.AgentAssociationLabelName, fleetServerBuilder.Agent.Name, certPEM, keyPEM)}.
				WithSteps(test.StepList{
					{
						Name: "Verify fleet-managed Agent has transitive client cert configured",
						Test: test.Eventually(func() error {
							var ag agentv1alpha1.Agent
							if err := k.Client.Get(context.Background(), types.NamespacedName{
								Namespace: namespace,
								Name:      agentBuilder.Agent.Name,
							}, &ag); err != nil {
								return err
							}
							for _, assoc := range ag.GetAssociations() {
								if assoc.AssociationType() != commonv1.FleetServerAssociationType {
									continue
								}
								conf, err := assoc.AssociationConf()
								if err != nil {
									return err
								}
								if conf == nil || conf.TransitiveESRef == nil || !conf.TransitiveESRef.ClientCertIsConfigured() {
									return fmt.Errorf("fleet-managed Agent should have a transitive ES client cert configured")
								}
								return nil
							}
							return fmt.Errorf("fleet-managed Agent has no Fleet Server association")
						}),
					},
				})
		},
	}

	before, after := clientauth.UserCustomCertificateSecretLifecycleSteps(namespace, userCertSecretName, certPEM, keyPEM)

	test.BeforeAfterSequence(before, after, test.LicenseTestBuilder(esBuilder), kbBuilder, fleetServerBuilder, agentWrapped).RunSequential(t)
}

// TestClientAuthRequiredCustomCertificate_FleetServerToAgent tests that a fleet-managed Agent uses a user-provided
// client certificate when connecting to a Fleet Server that has client authentication enabled.
func TestClientAuthRequiredCustomCertificate_FleetServerToAgent(t *testing.T) {
	if test.Ctx().TestLicense == "" {
		t.Skip("Skipping client authentication test: no enterprise test license configured")
	}

	name := "test-fs-mtls-custom"
	namespace := test.Ctx().ManagedNamespace(0)
	userCertSecretName := name + "-user-client-cert"

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
		WithClientAuthenticationRequired().
		WithElasticsearchRefs(agent.ToOutput(esBuilder.Ref(), "default")).
		WithKibanaRef(kbBuilder.Ref()).
		WithFleetAgentDataStreamsValidation()

	kbBuilder = kbBuilder.WithConfig(fleetConfigWithOutputsForKibana(t, fleetServerBuilder.Agent.Spec.Version, esBuilder.Ref(), fleetServerBuilder.Ref()))

	agentBuilder := agent.NewBuilder(name+"-ea").
		WithRoles(agent.AgentFleetModeRoleName).
		WithOpenShiftRoles(test.UseSCCRole).
		WithFleetMode().
		WithKibanaRef(kbBuilder.Ref()).
		WithFleetServerRefWithClientCert(fleetServerBuilder.Ref(), userCertSecretName)

	fleetServerBuilder = agent.ApplyYamls(t, fleetServerBuilder, "", E2EAgentFleetModePodTemplate)
	agentBuilder = agent.ApplyYamls(t, agentBuilder, "", E2EAgentFleetModePodTemplate)

	certPEM, keyPEM := helper.GenerateSelfSignedClientCert(t, name)

	// Wrap the agent builder to verify the custom cert is used in the fleet-server trust bundle.
	agentWrapped := test.WrappedBuilder{
		BuildingThis: agentBuilder,
		PostCheckSteps: func(k *test.K8sClient) test.StepList {
			return test.StepList{
				clientauth.CheckClientCertificateDataStep(k, namespace, fleetServerBuilder.Agent.Name,
					agentcontroller.AgentAssociationLabelName, agentBuilder.Agent.Name, certPEM, keyPEM),
				{
					Name: "Verify fleet-managed Agent has fleet-server client cert configured",
					Test: test.Eventually(func() error {
						var ag agentv1alpha1.Agent
						if err := k.Client.Get(context.Background(), types.NamespacedName{
							Namespace: namespace,
							Name:      agentBuilder.Agent.Name,
						}, &ag); err != nil {
							return err
						}
						for _, assoc := range ag.GetAssociations() {
							if assoc.AssociationType() != commonv1.FleetServerAssociationType {
								continue
							}
							conf, err := assoc.AssociationConf()
							if err != nil {
								return err
							}
							if conf == nil || !conf.ClientCertIsConfigured() {
								return fmt.Errorf("fleet-managed Agent should have a fleet-server client cert configured")
							}
							return nil
						}
						return fmt.Errorf("fleet-managed Agent has no Fleet Server association")
					}),
				},
			}
		},
	}

	before, after := clientauth.UserCustomCertificateSecretLifecycleSteps(namespace, userCertSecretName, certPEM, keyPEM)

	test.BeforeAfterSequence(before, after, test.LicenseTestBuilder(esBuilder), kbBuilder, fleetServerBuilder, agentWrapped).RunSequential(t)
}

// fleetConfigWithOutputsForKibana builds a Kibana config that uses xpack.fleet.outputs instead of
// xpack.fleet.agents.elasticsearch.hosts. The two cannot coexist. Defining outputs explicitly is
// necessary for mTLS tests so the Kibana controller can inject ssl.certificate and ssl.key into
// the fleet output via injectFleetOutputClientCerts.
func fleetConfigWithOutputsForKibana(t *testing.T, agentVersion string, esRef commonv1.ObjectSelector, fsRef commonv1.ObjectSelector) map[string]interface{} {
	t.Helper()
	cfg := map[string]interface{}{}

	v, err := version.Parse(agentVersion)
	if err != nil {
		t.Fatalf("Unable to parse Agent version: %v", err)
	}
	if v.GTE(version.MustParse("7.16.0")) {
		if err := yaml.Unmarshal([]byte(E2EFleetPolicies), &cfg); err != nil {
			t.Fatalf("Unable to parse Fleet policies: %v", err)
		}
	}

	esURL := fmt.Sprintf("https://%s-es-http.%s.svc:9200", esRef.Name, esRef.Namespace)

	cfg["xpack.fleet.outputs"] = []map[string]interface{}{
		{
			"id":                    "eck-fleet-agent-output-elasticsearch",
			"is_default":            true,
			"is_default_monitoring": true,
			"name":                  "eck-elasticsearch",
			"type":                  "elasticsearch",
			"hosts":                 []string{esURL},
		},
	}

	cfg["xpack.fleet.agents.fleet_server.hosts"] = []string{
		fmt.Sprintf("https://%s-agent-http.%s.svc:8220", fsRef.Name, fsRef.Namespace),
	}

	return cfg
}
