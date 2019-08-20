// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apm

import (
	"fmt"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/apm/v1alpha1"
	commonv1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/common/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/apmserver"
	"github.com/elastic/cloud-on-k8s/operators/test/e2e/test/elasticsearch"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// TestCrossNSAssociation tests associating Elasticsearch and an APM Server running in different namespaces.
func TestCrossNSAssociation(t *testing.T) {
	// This test currently does not work in the E2E environment because each namespace has a dedicated
	// controller (see https://github.com/elastic/cloud-on-k8s/issues/1438)
	if !(test.Ctx().Local || test.Ctx().GlobalOperator.AllInOne) {
		t.SkipNow()
	}

	esNamespace := test.Ctx().ManagedNamespace(0)
	apmNamespace := test.Ctx().ManagedNamespace(1)
	name := "test-cross-ns-assoc"

	esBuilder := elasticsearch.NewBuilder(name).
		WithNamespace(esNamespace).
		WithESMasterDataNodes(1, elasticsearch.DefaultResources).
		WithRestrictedSecurityContext()
	apmBuilder := apmserver.NewBuilder(name).
		WithNamespace(apmNamespace).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1).
		WithRestrictedSecurityContext().
		WithConfig(map[string]interface{}{
			"apm-server.ilm.enabled":                           false,
			"setup.template.settings.index.number_of_replicas": 0, // avoid ES yellow state on a 1 node ES cluster
		})

	test.Sequence(nil, test.EmptySteps, esBuilder, apmBuilder).
		RunSequential(t)
}

func TestAPMAssociationWithNonExistentES(t *testing.T) {
	name := "test-apm-assoc-non-existent-es"
	apmBuilder := apmserver.NewBuilder(name).
		WithElasticsearchRef(commonv1alpha1.ObjectSelector{
			Name: "non-existent-es",
		}).
		WithNodeCount(1)

	k := test.NewK8sClientOrFatal()
	steps := test.StepList{}
	steps = steps.WithSteps(apmBuilder.InitTestSteps(k))
	steps = steps.WithSteps(apmBuilder.CreationTestSteps(k))
	steps = steps.WithStep(test.Step{
		Name: "Non existent backend should generate event",
		Test: test.Eventually(func() error {
			eventList, err := k.GetEvents(test.EventListOptions(apmBuilder.ApmServer.Namespace, apmBuilder.ApmServer.Name))
			if err != nil {
				return err
			}

			for _, evt := range eventList {
				if evt.Type == corev1.EventTypeWarning && evt.Reason == events.EventAssociationError {
					return nil
				}
			}

			return fmt.Errorf("event did not fire: %s", events.EventAssociationError)
		}),
	})
	steps = steps.WithSteps(apmBuilder.DeletionTestSteps(k))

	steps.RunSequential(t)
}

func TestAPMAssociationWhenReferencedESDisappears(t *testing.T) {
	name := "test-apm-del-referenced-es"
	esBuilder := elasticsearch.NewBuilder(name).
		WithESMasterDataNodes(3, elasticsearch.DefaultResources)
	apmBuilder := apmserver.NewBuilder(name).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1)

	failureSteps := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			test.Step{
				Name: "Updating to invalid Elasticsearch reference should succeed",
				Test: func(t *testing.T) {
					var apm v1alpha1.ApmServer
					require.NoError(t, k.Client.Get(k8s.ExtractNamespacedName(&apmBuilder.ApmServer), &apm))
					apm.Spec.ElasticsearchRef.Namespace = "xxxx"
					require.NoError(t, k.Client.Update(&apm))
				},
			},
			test.Step{
				Name: "Lost Elasticsearch association should generate events",
				Test: test.Eventually(func() error {
					eventList, err := k.GetEvents(test.EventListOptions(apmBuilder.ApmServer.Namespace, apmBuilder.ApmServer.Name))
					if err != nil {
						return err
					}

					assocEstablishedEventSeen := false
					assocLostEventSeen := false
					noBackendEventSeen := false

					for _, evt := range eventList {
						switch {
						case evt.Type == corev1.EventTypeNormal && evt.Reason == events.EventAssociationStatusChange:
							prevStatus, currStatus := annotation.ExtractAssociationStatus(evt.ObjectMeta)
							if prevStatus == commonv1alpha1.AssociationEstablished && currStatus != prevStatus {
								assocLostEventSeen = true
							}

							if currStatus == commonv1alpha1.AssociationEstablished {
								assocEstablishedEventSeen = true
							}
						case evt.Type == corev1.EventTypeWarning && evt.Reason == events.EventAssociationError:
							noBackendEventSeen = true
						}
					}

					if assocEstablishedEventSeen && assocLostEventSeen && noBackendEventSeen {
						return nil
					}

					return fmt.Errorf("expected events did not fire: assocEstablished=%v assocLost=%v noBackend=%v",
						assocEstablishedEventSeen, assocLostEventSeen, noBackendEventSeen)
				}),
			},
		}
	}

	test.RunUnrecoverableFailureScenario(t, failureSteps, apmBuilder, esBuilder)
}
