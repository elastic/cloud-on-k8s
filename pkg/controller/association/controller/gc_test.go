// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	eslabel "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestGarbageCollectLegacyFleetServerAdditionalSecrets(t *testing.T) {
	const (
		agentNS       = "agent-ns"
		fleetServerNS = "fleet-ns"
		sharedNS      = "shared-ns"
		esName        = "es"
		esNS          = "es-ns"
	)

	// fleetServerESLabels mimics the labels that copySecret propagates from the source CA/client-cert
	// secret in the fleet server namespace. Those source secrets carry both the agent association labels
	// (set by the fleet-server→ES association reconciler) and the ES cluster identity labels (set via
	// AssociationResourceLabels), which the old copySecret function copies verbatim onto the copy.
	fleetServerESLabels := func(fleetName, fleetNS string) map[string]string {
		return map[string]string{
			AgentAssociationLabelName:         fleetName,
			AgentAssociationLabelNamespace:    fleetNS,
			AgentAssociationLabelType:         commonv1.ElasticsearchAssociationType,
			eslabel.ClusterNameLabelName:      esName,
			eslabel.ClusterNamespaceLabelName: esNS,
		}
	}

	for _, tt := range []struct {
		name          string
		secrets       []client.Object
		managedNS     []string
		wantDeleted   []string // secret names expected to be gone
		wantPreserved []string // secret names expected to survive
	}{
		{
			name: "cross-namespace legacy copies are deleted",
			secrets: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: agentNS, Name: "fleet1-es-ca", Labels: fleetServerESLabels("fleet1", fleetServerNS)},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: agentNS, Name: "fleet1-agent-es-xxx-client-cert", Labels: fleetServerESLabels("fleet1", fleetServerNS)},
				},
			},
			managedNS:   []string{agentNS},
			wantDeleted: []string{"fleet1-es-ca", "fleet1-agent-es-xxx-client-cert"},
		},
		{
			name: "orphaned legacy copy with no agent objects present is still deleted",
			secrets: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: agentNS, Name: "fleet1-es-ca", Labels: fleetServerESLabels("fleet1", fleetServerNS)},
				},
			},
			managedNS:   []string{agentNS},
			wantDeleted: []string{"fleet1-es-ca"},
		},
		{
			name: "new-scheme copies with overridden labels survive",
			secrets: []client.Object{
				// New-scheme: agentassociation labels point to the agent, not the fleet server.
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: agentNS,
						Name:      "agent1-agent-fleetserver-hashxxx-es-ca",
						Labels: map[string]string{
							AgentAssociationLabelName:      "agent1",
							AgentAssociationLabelNamespace: agentNS,
							AgentAssociationLabelType:      commonv1.FleetServerAssociationType,
						},
					},
				},
			},
			managedNS:     []string{agentNS},
			wantPreserved: []string{"agent1-agent-fleetserver-hashxxx-es-ca"},
		},
		{
			name: "original CA in fleet-server namespace is not touched",
			secrets: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: fleetServerNS, Name: "fleet1-es-ca", Labels: fleetServerESLabels("fleet1", fleetServerNS)},
				},
			},
			managedNS:     []string{agentNS, fleetServerNS},
			wantPreserved: []string{"fleet1-es-ca"},
		},
		{
			name: "same-namespace secret preserved",
			secrets: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: sharedNS, Name: "fleet1-es-ca", Labels: fleetServerESLabels("fleet1", sharedNS)},
				},
			},
			managedNS:     []string{sharedNS},
			wantPreserved: []string{"fleet1-es-ca"},
		},
		{
			name: "secret with owner reference is skipped even if labels match",
			secrets: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: agentNS, Name: "fleet1-es-ca",
						Labels: fleetServerESLabels("fleet1", fleetServerNS),
						OwnerReferences: []metav1.OwnerReference{
							{APIVersion: "agent.k8s.elastic.co/v1alpha1", Kind: "Agent", Name: "agent1"},
						},
					},
				},
			},
			managedNS:     []string{agentNS},
			wantPreserved: []string{"fleet1-es-ca"},
		},
		{
			name: "cross-namespace copies for two different fleet servers both deleted",
			secrets: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: agentNS, Name: "fleet1-es-ca", Labels: fleetServerESLabels("fleet1", fleetServerNS)},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: agentNS, Name: "fleet2-es-ca", Labels: fleetServerESLabels("fleet2", fleetServerNS)},
				},
			},
			managedNS:   []string{agentNS},
			wantDeleted: []string{"fleet1-es-ca", "fleet2-es-ca"},
		},
		{
			name: "empty managedNamespaces sweeps all namespaces",
			secrets: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: agentNS, Name: "fleet1-es-ca", Labels: fleetServerESLabels("fleet1", fleetServerNS)},
				},
			},
			managedNS:   []string{}, // empty = all namespaces
			wantDeleted: []string{"fleet1-es-ca"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(tt.secrets...)
			require.NoError(t, GarbageCollectLegacyFleetServerAdditionalSecrets(context.Background(), c, tt.managedNS))
			// Build name→namespace from input fixtures so the deleted-check is not tied to a hardcoded namespace.
			secretNSByName := make(map[string]string, len(tt.secrets))
			secretNSs := make([]string, 0, len(tt.secrets))
			for _, obj := range tt.secrets {
				secretNSByName[obj.GetName()] = obj.GetNamespace()
				secretNSs = append(secretNSs, obj.GetNamespace())
			}
			for _, name := range tt.wantDeleted {
				ns := secretNSByName[name]
				var s corev1.Secret
				err := c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &s)
				require.True(t, apierrors.IsNotFound(err), "expected %q in namespace %q to be deleted, got: %v", name, ns, err)
			}
			// Collect the namespaces of all input secrets so the preserved-check works
			// regardless of whether managedNS is empty (all-namespaces mode).
			for _, name := range tt.wantPreserved {
				found := false
				for _, ns := range secretNSs {
					var s corev1.Secret
					if err := c.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &s); err == nil {
						found = true
						break
					}
				}
				require.True(t, found, "expected %q to be preserved", name)
			}
		})
	}
}
