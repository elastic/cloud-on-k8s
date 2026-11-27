// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	agentcontroller "github.com/elastic/cloud-on-k8s/v3/pkg/controller/agent"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v3/pkg/dev/portforward"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/pointer"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/set"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v3/test/e2e/test/generation"
)

const k8sRequestRetryTimeout = time.Minute

func (b Builder) InitTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "K8S should be accessible",
			Test: test.Eventually(func() error {
				pods := corev1.PodList{}
				return k.Client.List(context.Background(), &pods)
			}),
		},
		{
			Name: "Label test pods",
			Test: test.Eventually(func() error {
				return test.LabelTestPods(
					k.Client,
					test.Ctx(),
					run.TestNameLabel,
					b.Agent.Labels[run.TestNameLabel])
			}),
			Skip: func() bool {
				return test.Ctx().Local
			},
		},
		{
			Name: "Agent CRDs should exist",
			Test: test.Eventually(func() error {
				crd := &agentv1alpha1.AgentList{}
				return k.Client.List(context.Background(), crd)
			}),
		},
		{
			Name: "Remove Agent if it already exists",
			Test: test.Eventually(func() error {
				for _, obj := range b.RuntimeObjects() {
					err := k.Client.Delete(context.Background(), obj)
					if err != nil && !apierrors.IsNotFound(err) {
						return err
					}
				}
				// wait for Agent pods to disappear
				if err := k.CheckPodCount(0, test.AgentPodListOptions(b.Agent.Namespace, b.Agent.Name)...); err != nil {
					return err
				}

				// it may take some extra time for Agent to be fully deleted
				var agent agentv1alpha1.Agent
				err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Agent), &agent)
				if err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				if err == nil {
					return fmt.Errorf("agent %s is still there", k8s.ExtractNamespacedName(&b.Agent))
				}
				return nil
			}),
		},
	}
}

func (b Builder) CreationTestSteps(k *test.K8sClient) test.StepList {
	// increase the test timeout for autopilot clusters because autoscaling k8s nodes time can increase the time
	// before the elasticsearch container is ready
	timeout := test.Ctx().TestTimeout
	if test.Ctx().AutopilotCluster {
		timeout *= 2
	}
	return test.StepList{}.
		WithSteps(test.StepList{
			test.Step{
				// This is to improve test stability see https://github.com/elastic/cloud-on-k8s/issues/7172
				Name: "Wait for Elasticsearch to be green",
				Test: test.UntilSuccess(func() error {
					for _, ref := range b.Agent.Spec.ElasticsearchRefs {
						var es esv1.Elasticsearch
						if err := k.Client.Get(context.Background(), ref.WithDefaultNamespace(b.Agent.Namespace).NamespacedName(), &es); err != nil {
							return err
						}
						if es.Status.Health != esv1.ElasticsearchGreenHealth {
							return fmt.Errorf("health is not green but %s", es.Status.Health)
						}
					}
					return nil
				}, timeout),
				Skip: func() bool {
					return len(b.Agent.Spec.ElasticsearchRefs) == 0
				},
			},
			test.Step{
				Name: "Creating an Agent should succeed",
				Test: func(t *testing.T) {
					t.Helper()
					require.NoError(t, k.CreateWithRetry(
						test.IsRetryableError,
						k8sRequestRetryTimeout,
						b.RuntimeObjects()...,
					))
				},
			},
			test.Step{
				Name: "Agent should be created",
				Test: test.RetryOnError(func() error {
					var createdAgent agentv1alpha1.Agent
					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Agent), &createdAgent); err != nil {
						return err
					}
					if b.Agent.Spec.Version != createdAgent.Spec.Version {
						return fmt.Errorf("expected version %s but got %s", b.Agent.Spec.Version, createdAgent.Spec.Version)
					}
					return nil
				}, test.IsRetryableError, k8sRequestRetryTimeout),
			},
			b.fleetServerServingStep(k),
		})
}

const (
	// fleetServerStartupGracePeriod is how long a Fleet Server container must have been running before a closed
	// port means it is stuck rather than still starting.
	fleetServerStartupGracePeriod = 2 * time.Minute
	// fleetServerMaxStuckRestarts caps the restart attempts, so that other causes of a closed port still fail the test.
	fleetServerMaxStuckRestarts = 2
)

// fleetServerServingStep works around a Fleet Server bug fixed in 8.9.0 (https://github.com/elastic/fleet-server/pull/2697):
// a 401 from an Elasticsearch node that has not loaded the service account token yet can stop Fleet Server from
// listening while it still reports healthy. Once all the nodes have loaded the token this cannot happen anymore, so a
// Fleet Server that is still not listening is restarted by deleting its Pod.
func (b Builder) fleetServerServingStep(k *test.K8sClient) test.Step {
	return test.Step{
		Name: "Fleet Server should be serving (workaround for fleet-server#2697)",
		Test: func(t *testing.T) {
			t.Helper()
			ctx := t.Context()
			esRef := b.Agent.Spec.ElasticsearchRefs[0].WithDefaultNamespace(b.Agent.Namespace).NamespacedName()
			fleetServer := k8s.ExtractNamespacedName(&b.Agent)
			// the dialer and the Elasticsearch client are reused across retries, as the port-forwarders they start in
			// local test runs are never stopped
			dial := (&net.Dialer{Timeout: 5 * time.Second}).DialContext
			if test.Ctx().AutoPortForwarding {
				dial = portforward.NewForwardingDialer().DialContext
			}
			var esClient esclient.Client
			defer func() {
				if esClient != nil {
					esClient.Close()
				}
			}()
			restarts := 0
			test.Eventually(func() error {
				if esClient == nil {
					var es esv1.Elasticsearch
					if err := k.Client.Get(ctx, esRef, &es); err != nil {
						return err
					}
					c, err := elasticsearch.NewElasticsearchClient(es, k)
					if err != nil {
						return err
					}
					esClient = c
				}
				if err := fleetServerTokenLoaded(ctx, k, esClient, esRef, fleetServer); err != nil {
					return err
				}
				respErr := fleetServerResponding(ctx, dial, fleetServer)
				if respErr == nil {
					return nil
				}
				if restarts >= fleetServerMaxStuckRestarts {
					return fmt.Errorf("fleet server is not serving after %d restarts: %w", restarts, respErr)
				}
				pods, err := k.GetPods(test.AgentPodListOptions(b.Agent.Namespace, b.Agent.Name)...)
				if err != nil {
					return err
				}
				for _, pod := range pods {
					// skip Pods already being deleted so that a single restart is not counted multiple times
					if pod.DeletionTimestamp != nil ||
						!containerRunningLongerThan(pod, agentcontroller.ContainerName, fleetServerStartupGracePeriod) {
						continue
					}
					t.Logf("Deleting stuck Fleet Server Pod %s/%s: %v", pod.Namespace, pod.Name, respErr)
					if err := k.Client.Delete(ctx, &pod); err != nil && !apierrors.IsNotFound(err) {
						return err
					}
					restarts++
				}
				return fmt.Errorf("fleet server is not serving yet: %w", respErr)
			})(t)
		},
		Skip: func() bool {
			return !b.Agent.Spec.FleetServerEnabled ||
				// probing the Service cannot tell which of several Fleet Server Pods is stuck
				b.Count() != 1 ||
				len(b.Agent.Spec.ElasticsearchRefs) == 0 ||
				b.Agent.Spec.ElasticsearchRefs[0].IsExternal() ||
				version.MustParse(b.Agent.Spec.Version).GTE(version.MinFor(8, 9, 0))
		},
	}
}

// fleetServerTokenLoaded returns an error unless the service account token of the given Fleet Server is loaded by all
// the nodes of the given Elasticsearch cluster.
func fleetServerTokenLoaded(
	ctx context.Context,
	k *test.K8sClient,
	esClient esclient.Client,
	esRef, fleetServer types.NamespacedName,
) error {
	var agent agentv1alpha1.Agent
	if err := k.Client.Get(ctx, fleetServer, &agent); err != nil {
		return err
	}
	tokenName := association.TokenName(agent.Namespace, agent.Name, agent.UID)
	esPods, err := k.GetPods(test.ESPodListOptions(esRef.Namespace, esRef.Name)...)
	if err != nil {
		return err
	}
	credentials, err := esClient.GetServiceAccountCredentials(ctx, "elastic/fleet-server")
	if err != nil {
		return err
	}
	fileToken, ok := credentials.NodesCredentials.FileTokens[tokenName]
	if !ok {
		return fmt.Errorf("service account token %s not loaded yet by any Elasticsearch node", tokenName)
	}
	loadedBy := set.Make(fileToken.Nodes...)
	for _, pod := range esPods {
		if !loadedBy.Has(pod.Name) {
			return fmt.Errorf("service account token %s not loaded yet by Elasticsearch node %s", tokenName, pod.Name)
		}
	}
	return nil
}

// fleetServerResponding returns an error if the Fleet Server HTTP service does not answer a plain HTTP request.
// Any answer is enough, including the 400 sent by a TLS-enabled Fleet Server. Connecting alone is not, because the
// port-forwarder used in local test runs accepts connections even when nothing listens in the Pod.
func fleetServerResponding(
	ctx context.Context,
	dial func(ctx context.Context, network, addr string) (net.Conn, error),
	fleetServer types.NamespacedName,
) error {
	// add .svc suffix so that requests work when using the port-forwarder during local test runs
	addr := fmt.Sprintf("%s.%s.svc:%d", agentcontroller.HTTPServiceName(fleetServer.Name), fleetServer.Namespace, agentcontroller.FleetServerPort)
	conn, err := dial(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	if _, err := conn.Write([]byte("GET / HTTP/1.0\r\n\r\n")); err != nil {
		return err
	}
	_, err = conn.Read(make([]byte, 1))
	return err
}

// containerRunningLongerThan returns true if the given container of the Pod has been running for longer than d.
func containerRunningLongerThan(pod corev1.Pod, containerName string, d time.Duration) bool {
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == containerName && status.State.Running != nil {
			return time.Since(status.State.Running.StartedAt.Time) > d
		}
	}
	return false
}

func (b Builder) CheckK8sTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Agent Elasticsearch associations should be established",
			Test: test.Eventually(func() error {
				var agent agentv1alpha1.Agent
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Agent), &agent); err != nil {
					return err
				}
				expected := len(b.Agent.Spec.ElasticsearchRefs)
				got := len(agent.Status.ElasticsearchAssociationsStatus)
				if expected != got {
					return fmt.Errorf("expected %d ES association(s) in status, got %d", expected, got)
				}
				for ref, s := range agent.Status.ElasticsearchAssociationsStatus {
					if s != commonv1.AssociationEstablished {
						return fmt.Errorf("ES association %s is %q, expected %q", ref, s, commonv1.AssociationEstablished)
					}
				}
				return nil
			}),
			Skip: func() bool {
				return len(b.Agent.Spec.ElasticsearchRefs) == 0
			},
		},
		{
			Name: "Agent status should be updated",
			Test: test.Eventually(func() error {
				var agent agentv1alpha1.Agent
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Agent), &agent); err != nil {
					return err
				}
				agent.Status.ElasticsearchAssociationsStatus = nil
				agent.Status.KibanaAssociationStatus = ""
				agent.Status.FleetServerAssociationStatus = ""
				agent.Status.ObservedGeneration = 0

				expected := agentv1alpha1.AgentStatus{
					Version:    b.Agent.Spec.Version,
					Health:     "green",
					Conditions: agent.Status.Conditions, // Ignore Conditions whose LastTransitionTime is unpredictable
				}

				switch {
				case b.Agent.Spec.Deployment != nil:
					expectedReplicas := pointer.Int32OrDefault(b.Agent.Spec.Deployment.Replicas, int32(1))
					expected.ExpectedNodes = expectedReplicas
					expected.AvailableNodes = expectedReplicas
				case b.Agent.Spec.StatefulSet != nil:
					expectedReplicas := pointer.Int32OrDefault(b.Agent.Spec.StatefulSet.Replicas, int32(1))
					expected.ExpectedNodes = expectedReplicas
					expected.AvailableNodes = expectedReplicas
				default:
					// don't check the replicas count for daemonsets
					agent.Status.ExpectedNodes = 0
					agent.Status.AvailableNodes = 0
				}
				if !reflect.DeepEqual(agent.Status, expected) {
					return fmt.Errorf("expected status %+v but got %+v", expected, agent.Status)
				}
				return nil
			}),
		},
		test.CheckFieldsNotOwnedByOperator(&b.Agent, k, nil),
	}
}

func (b Builder) CheckStackTestSteps(k *test.K8sClient) test.StepList {
	// health
	return test.StepList{
		test.Step{
			Name: "Agent health should be green",
			Test: test.Eventually(func() error {
				var agent agentv1alpha1.Agent
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Agent), &agent); err != nil {
					return err
				}

				if agent.Status.Health != agentv1alpha1.AgentGreenHealth {
					return fmt.Errorf("agent %s health is %s", agent.Name, agent.Status.Health)
				}

				return nil
			}),
		},
		test.Step{
			Name: "ES data should pass validations",
			Test: test.Eventually(func() error {
				for i, validation := range b.Validations {
					expectedOutputName := b.ValidationsOutputs[i]
					var esNsName types.NamespacedName
					for _, output := range b.Agent.Spec.ElasticsearchRefs {
						if output.OutputName == expectedOutputName ||
							output.OutputName == "" && len(b.Agent.Spec.ElasticsearchRefs) == 1 {
							esNsName = output.WithDefaultNamespace(b.Agent.Namespace).NamespacedName()
							break
						}
					}

					if esNsName.Namespace == "" && esNsName.Name == "" {
						return fmt.Errorf("agent %s doesn't have output %s", b.Agent.Name, expectedOutputName)
					}

					var es esv1.Elasticsearch
					if err := k.Client.Get(context.Background(), esNsName, &es); err != nil {
						return err
					}

					esClient, err := elasticsearch.NewElasticsearchClient(es, k)
					if err != nil {
						return err
					}

					if err := validation(esClient); err != nil {
						return err
					}
				}

				return nil
			}),
		},
	}
}

func (b Builder) UpgradeTestSteps(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "Applying the Agent mutation should succeed",
			Test: test.Eventually(func() error {
				var agent agentv1alpha1.Agent
				if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&b.Agent), &agent); err != nil {
					return err
				}
				// merge annotations
				if agent.Annotations == nil {
					agent.Annotations = make(map[string]string)
				}
				maps.Copy(agent.Annotations, b.Agent.Annotations)
				// Apply the same OCP-specific security context that RuntimeObjects() applies on
				// creation so the spec stays consistent and no spurious pending changes are detected.
				agent.Spec = b.withOCPSecurityContext().Agent.Spec
				if err := k.Client.Update(context.Background(), &agent); err != nil {
					return err
				}
				return nil
			}),
		}}
}

func (b Builder) DeletionTestSteps(k *test.K8sClient) test.StepList {
	//nolint:thelper
	return []test.Step{
		{
			Name: "Deleting the resources should return no error",
			Test: func(t *testing.T) {
				for _, obj := range b.RuntimeObjects() {
					err := k.Client.Delete(context.Background(), obj)
					require.NoError(t, err)
				}
			},
		},
		{
			Name: "The resources should not be there anymore",
			Test: test.Eventually(func() error {
				for _, obj := range b.RuntimeObjects() {
					objCopy := k8s.DeepCopyObject(obj)
					err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(obj), objCopy)
					if err != nil {
						if apierrors.IsNotFound(err) {
							continue
						}
					}
					return errors.Wrap(err, "expected 404 not found API error here")
				}
				return nil
			}),
		},
		{
			Name: "Agent pods should be eventually be removed",
			Test: test.Eventually(func() error {
				return k.CheckPodCount(0, test.AgentPodListOptions(b.Agent.Namespace, b.Agent.Name)...)
			}),
		},
	}
}

// CheckFleetServerConnected returns a step that verifies at least one agent pod reports
// FleetState == 2 (connected) by running elastic-agent status --output=json inside each container.
func (b Builder) CheckFleetServerConnected(k *test.K8sClient) test.StepList {
	return test.StepList{
		{
			Name: "At least one Agent pod should be connected to Fleet Server",
			Test: test.Eventually(func() error {
				pods, err := k.GetPods(test.AgentPodListOptions(b.Agent.Namespace, b.Agent.Name)...)
				if err != nil {
					return err
				}
				// One successful connection is enough: if any pod reached Fleet Server the
				// path is verified. Checking all pods would require every replica to
				// reconnect before the step passes, increasing test time unnecessarily.
				return test.OnAnyPod(pods, func(p corev1.Pod) error {
					stdout, stderr, err := k.ExecInContainer(
						k8s.ExtractNamespacedName(&p),
						agentcontroller.ContainerName,
						[]string{"elastic-agent", "status", "--output=json"},
					)
					if err != nil {
						return fmt.Errorf("err:%w,stdout:\n%s\nstderr:\n%s", err, stdout, stderr)
					}
					var agentStatus struct {
						FleetState int `json:"FleetState"`
					}
					if err := json.Unmarshal([]byte(stdout), &agentStatus); err != nil {
						return err
					}
					if agentStatus.FleetState != 2 {
						return fmt.Errorf("agent not connected to fleet-server, state: %d", agentStatus.FleetState)
					}
					return nil
				})
			}),
		},
	}
}

func (b Builder) MutationTestSteps(k *test.K8sClient) test.StepList {
	var agentGenerationBeforeMutation, agentObservedGenerationBeforeMutation int64

	isMutated := b.MutatedFrom != nil

	steps := test.StepList{}
	if b.MutatedFrom != nil {
		steps = test.AnnotatePodsWithBuilderHash(b, b.MutatedFrom, k)
	}
	return steps.
		WithStep(generation.RetrieveGenerationsStep(&b.Agent, k, &agentGenerationBeforeMutation, &agentObservedGenerationBeforeMutation)).
		WithSteps(b.UpgradeTestSteps(k)).
		WithSteps(b.CheckK8sTestSteps(k)).
		WithSteps(b.CheckStackTestSteps(k)).
		WithStep(generation.CompareObjectGenerationsStep(&b.Agent, k, isMutated, agentGenerationBeforeMutation, agentObservedGenerationBeforeMutation))
}
