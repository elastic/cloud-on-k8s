// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build apm || e2e

package apm

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"

	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/apmserver"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/kibana"
)

// TestCrossNSAssociation tests associating Elasticsearch and an APM Server running in different namespaces.
func TestCrossNSAssociation(t *testing.T) {
	esNamespace := test.Ctx().ManagedNamespace(0)
	apmNamespace := test.Ctx().ManagedNamespace(1)
	name := "test-cross-ns-assoc"

	esBuilder := elasticsearch.NewBuilder(name).
		WithNamespace(esNamespace).
		WithESMasterDataNodes(2, elasticsearch.DefaultResources). // TODO: revert when https://github.com/elastic/cloud-on-k8s/issues/7418 is resolved.
		WithRestrictedSecurityContext()
	apmBuilder := apmserver.NewBuilder(name).
		WithNamespace(apmNamespace).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1).
		WithRestrictedSecurityContext().
		WithConfig(map[string]interface{}{
			"apm-server.ilm.enabled":                           false,
			"setup.template.settings.index.number_of_replicas": 0, // avoid ES yellow state on a 1 node ES cluster
		}).
		WithoutIntegrationCheck()

	test.Sequence(nil, test.EmptySteps, esBuilder, apmBuilder).RunSequential(t)
}

// TestAPMKibanaAssociation tests associating an APM Server with Kibana.
func TestAPMKibanaAssociation(t *testing.T) {
	stackVersion := version.MustParse(test.Ctx().ElasticStackVersion)
	if !stackVersion.GTE(apmv1.ApmAgentConfigurationMinVersion) {
		t.SkipNow()
	}

	ns := test.Ctx().ManagedNamespace(0)
	name := "test-apm-kb-assoc"

	esBuilder := elasticsearch.NewBuilder(name).
		WithNamespace(ns).
		WithESMasterDataNodes(2, elasticsearch.DefaultResources). // TODO: revert when https://github.com/elastic/cloud-on-k8s/issues/7418 is resolved.
		WithRestrictedSecurityContext()

	kbBuilder := kibana.NewBuilder(name).
		WithNamespace(ns).
		WithElasticsearchRef(esBuilder.Ref()).
		WithNodeCount(1).
		WithRestrictedSecurityContext().
		WithAPMIntegration()

	apmBuilder := apmserver.NewBuilder(name).
		WithNamespace(ns).
		WithElasticsearchRef(esBuilder.Ref()).
		WithKibanaRef(kbBuilder.Ref()).
		WithNodeCount(1).
		WithRestrictedSecurityContext().
		WithConfig(map[string]interface{}{
			"apm-server.ilm.enabled":                           false,
			"setup.template.settings.index.number_of_replicas": 0, // avoid ES yellow state on a 1 node ES cluster
		})

	test.Sequence(nil, test.EmptySteps, esBuilder, kbBuilder, apmBuilder).RunSequential(t)
}

func TestAPMAssociationWithNonExistentES(t *testing.T) {
	name := "test-apm-assoc-non-existent-es"
	apmBuilder := apmserver.NewBuilder(name).
		WithElasticsearchRef(commonv1.ObjectSelector{
			Name: "non-existent-es",
		}).
		WithNodeCount(1).
		WithoutIntegrationCheck()

	k := test.NewK8sClientOrFatal()
	steps := test.StepList{}
	steps = steps.WithSteps(apmBuilder.InitTestSteps(k))
	steps = steps.WithSteps(apmBuilder.CreationTestSteps(k))
	steps = steps.WithStep(test.Step{
		Name: "Non existent backend should generate event",
		Test: test.Eventually(func() error {
			eventList, err := k.GetEvents(test.EventListOptions(apmBuilder.ApmServer.Namespace, apmBuilder.ApmServer.Name)...)
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
		WithNodeCount(1).
		WithoutIntegrationCheck()

	failureSteps := func(k *test.K8sClient) test.StepList {
		return test.StepList{
			test.Step{
				Name: "Updating to invalid Elasticsearch reference should succeed",
				Test: test.Eventually(func() error {
					var apm apmv1.ApmServer
					if err := k.Client.Get(context.Background(), k8s.ExtractNamespacedName(&apmBuilder.ApmServer), &apm); err != nil {
						return err
					}
					apm.Spec.ElasticsearchRef.Namespace = "xxxx"
					return k.Client.Update(context.Background(), &apm)
				}),
			},
			test.Step{
				Name: "Lost Elasticsearch association should generate events",
				Test: test.Eventually(func() error {
					eventList, err := k.GetEvents(test.EventListOptions(apmBuilder.ApmServer.Namespace, apmBuilder.ApmServer.Name)...)
					if err != nil {
						return err
					}

					assocEstablishedEventSeen := false
					assocLostEventSeen := false
					noBackendEventSeen := false

					for _, evt := range eventList {
						switch {
						case evt.Type == corev1.EventTypeNormal && evt.Reason == events.EventAssociationStatusChange:
							// build expected string and use it for comparisons with actual
							establishedString := commonv1.NewSingleAssociationStatusMap(commonv1.AssociationEstablished).String()
							prevStatusString, currStatusString := annotation.ExtractAssociationStatusStrings(evt.ObjectMeta)

							if prevStatusString == establishedString && currStatusString != prevStatusString {
								assocLostEventSeen = true
							}

							if currStatusString == establishedString {
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
