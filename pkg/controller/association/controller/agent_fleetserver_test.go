// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/agent"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
	utilmaps "github.com/elastic/cloud-on-k8s/v3/pkg/utils/maps"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/rbac"
)

// esAssocConfAnnotation returns the JSON-encoded association conf annotation value for a fleet server's ES association.
func esAssocConfAnnotation(conf commonv1.AssociationConf) string {
	data, _ := json.Marshal(conf)
	return string(data)
}

// esConfAnnotationKey returns the annotation key for a fleet server's ES association conf given the ES ObjectSelector.
func esConfAnnotationKey(esSelector commonv1.ObjectSelector) string {
	return commonv1.ElasticsearchConfigAnnotationName(esSelector)
}

func TestClientCertSecretName(t *testing.T) {
	agent := &agentv1alpha1.Agent{
		Name: "my-agent", Namespace: "ns",
	}
	ref := commonv1.ObjectSelector{Name: "my-es", Namespace: "es-ns"}
	ref2 := commonv1.ObjectSelector{Name: "other-es", Namespace: "es-ns"}

	for _, tt := range []struct {
		name       string
		associated commonv1.Associated
		ref        commonv1.AssociationRef
		assocName  string
		wantName   string
	}{
		{
			name:       "produces deterministic name with hash",
			associated: agent,
			ref:        ref,
			assocName:  "agent-es",
			wantName:   "my-agent-agent-es-" + hash.HashObject(ref.NamespacedName()) + "-client-cert",
		},
		{
			name:       "different refs produce different names",
			associated: agent,
			ref:        ref2,
			assocName:  "agent-es",
			wantName:   "my-agent-agent-es-" + hash.HashObject(ref2.NamespacedName()) + "-client-cert",
		},
		{
			name:       "different association names produce different names",
			associated: agent,
			ref:        ref,
			assocName:  "other-assoc",
			wantName:   "my-agent-other-assoc-" + hash.HashObject(ref.NamespacedName()) + "-client-cert",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := clientCertSecretName(tt.associated, tt.ref, tt.assocName)
			require.Equal(t, tt.wantName, got)
		})
	}

	// Verify idempotency.
	name1 := clientCertSecretName(agent, ref, "agent-es")
	name2 := clientCertSecretName(agent, ref, "agent-es")
	require.Equal(t, name1, name2)
}

func TestAdditionalSecrets(t *testing.T) {
	esSelector := commonv1.ObjectSelector{Name: "es1", Namespace: "es-ns"}

	for _, tt := range []struct {
		name        string
		agent       *agentv1alpha1.Agent
		fleetServer *agentv1alpha1.Agent
		assoc       commonv1.Association
		wantSecrets []association.AdditionalSecret
		wantErr     bool
	}{
		{
			name: "fleet server ref not set returns nil",
			agent: &agentv1alpha1.Agent{
				Name: "agent1", Namespace: "ns",
				Spec: agentv1alpha1.AgentSpec{Version: "8.0.0"},
			},
			wantSecrets: nil,
		},
		{
			name: "fleet server has no ES refs returns nil",
			agent: &agentv1alpha1.Agent{
				Name: "agent1", Namespace: "ns",
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				Name: "fleet1", Namespace: "fs-ns",
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
				},
			},
			wantSecrets: nil,
		},
		{
			name: "same-namespace fleet server with omitted namespace: CA entry returned for hashing, no copy created",
			agent: &agentv1alpha1.Agent{
				Name: "agent1", Namespace: "ns",
				Spec: agentv1alpha1.AgentSpec{
					Version: "8.0.0",
					// Namespace intentionally omitted — ECK treats this as same namespace.
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				Name: "fleet1", Namespace: "ns",
				Annotations: map[string]string{
					esConfAnnotationKey(esSelector): esAssocConfAnnotation(commonv1.AssociationConf{
						AuthSecretName: "-",
						CACertProvided: true,
						CASecretName:   "fleet1-es-ca",
						URL:            "https://es1-http.es-ns.svc:9200",
					}),
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{ObjectSelector: esSelector},
					},
				},
			},
			wantSecrets: []association.AdditionalSecret{
				{Source: types.NamespacedName{Namespace: "ns", Name: "fleet1-es-ca"}, Keys: []string{"ca.crt"}, TargetName: "fleet1-es-ca"},
			},
		},
		{
			name: "same-namespace fleet server: CA entry returned for hashing, no copy created",
			agent: &agentv1alpha1.Agent{
				Name: "agent1", Namespace: "ns",
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				Name: "fleet1", Namespace: "ns",
				Annotations: map[string]string{
					esConfAnnotationKey(esSelector): esAssocConfAnnotation(commonv1.AssociationConf{
						AuthSecretName: "-",
						CACertProvided: true,
						CASecretName:   "fleet1-es-ca",
						URL:            "https://es1-http.es-ns.svc:9200",
					}),
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{ObjectSelector: esSelector},
					},
				},
			},
			wantSecrets: []association.AdditionalSecret{
				{Source: types.NamespacedName{Namespace: "ns", Name: "fleet1-es-ca"}, Keys: []string{"ca.crt"}, TargetName: "fleet1-es-ca"},
			},
		},
		{
			name: "fleet server has ES ref with CA returns CA secret",
			agent: &agentv1alpha1.Agent{
				Name: "agent1", Namespace: "ns",
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				Name: "fleet1", Namespace: "fs-ns",
				Annotations: map[string]string{
					esConfAnnotationKey(esSelector): esAssocConfAnnotation(commonv1.AssociationConf{
						AuthSecretName: "-",
						CACertProvided: true,
						CASecretName:   "fleet1-es-ca",
						URL:            "https://es1-http.es-ns.svc:9200",
					}),
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{ObjectSelector: esSelector},
					},
				},
			},
			wantSecrets: []association.AdditionalSecret{
				{Source: types.NamespacedName{Namespace: "fs-ns", Name: "fleet1-es-ca"}, Keys: []string{"ca.crt"}, TargetName: "agent1-agent-fleetserver-" + hash.HashObject(types.NamespacedName{Name: "fleet1", Namespace: "fs-ns"}) + "-es-ca"},
			},
		},
		{
			name: "fleet server has ES ref with CA and user-provided client cert returns both",
			agent: &agentv1alpha1.Agent{
				Name: "agent1", Namespace: "ns",
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				Name: "fleet1", Namespace: "fs-ns",
				Annotations: map[string]string{
					esConfAnnotationKey(esSelector): esAssocConfAnnotation(commonv1.AssociationConf{
						AuthSecretName:       "-",
						CACertProvided:       true,
						CASecretName:         "fleet1-es-ca",
						ClientCertSecretName: "copied-user-cert",
						URL:                  "https://es1-http.es-ns.svc:9200",
					}),
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{
							ObjectSelector:              esSelector,
							ClientCertificateSecretName: "user-provided-cert"},
					},
				},
			},
			wantSecrets: []association.AdditionalSecret{
				{Source: types.NamespacedName{Namespace: "fs-ns", Name: "fleet1-es-ca"}, Keys: []string{"ca.crt"}, TargetName: "agent1-agent-fleetserver-" + hash.HashObject(types.NamespacedName{Name: "fleet1", Namespace: "fs-ns"}) + "-es-ca"},
				{Source: types.NamespacedName{Namespace: "fs-ns", Name: "copied-user-cert"}, TargetName: "agent1-agent-fleetserver-" + hash.HashObject(types.NamespacedName{Name: "fleet1", Namespace: "fs-ns"}) + "-es-client-cert"},
			},
		},
		{
			name: "CA not provided: empty slice returned, GC handles stale copies",
			agent: &agentv1alpha1.Agent{
				Name: "agent1", Namespace: "ns",
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				Name: "fleet1", Namespace: "fs-ns",
				Annotations: map[string]string{
					esConfAnnotationKey(esSelector): esAssocConfAnnotation(commonv1.AssociationConf{
						AuthSecretName: "-",
						CACertProvided: false,
						CASecretName:   "fleet1-es-ca",
						URL:            "https://es1-http.es-ns.svc:9200",
					}),
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{ObjectSelector: esSelector},
					},
				},
			},
			wantSecrets: []association.AdditionalSecret{},
		},
		{
			name: "client cert removed from ESRef but still in conf: only CA returned, GC removes stale client cert copy",
			agent: &agentv1alpha1.Agent{
				Name: "agent1", Namespace: "ns",
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				Name: "fleet1", Namespace: "fs-ns",
				Annotations: map[string]string{
					esConfAnnotationKey(esSelector): esAssocConfAnnotation(commonv1.AssociationConf{
						AuthSecretName:       "-",
						CACertProvided:       true,
						CASecretName:         "fleet1-es-ca",
						ClientCertSecretName: "copied-user-cert", // stale: user removed cert ref from ESRef
						URL:                  "https://es1-http.es-ns.svc:9200",
					}),
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						// ClientCertificateSecretName deliberately absent — user removed the ref
						{ObjectSelector: esSelector},
					},
				},
			},
			wantSecrets: []association.AdditionalSecret{
				{Source: types.NamespacedName{Namespace: "fs-ns", Name: "fleet1-es-ca"}, Keys: []string{"ca.crt"}, TargetName: "agent1-agent-fleetserver-" + hash.HashObject(types.NamespacedName{Name: "fleet1", Namespace: "fs-ns"}) + "-es-ca"},
			},
		},
		{
			name: "client cert in conf but no user-provided cert name: only CA returned",
			agent: &agentv1alpha1.Agent{
				Name: "agent1", Namespace: "ns",
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				Name: "fleet1", Namespace: "fs-ns",
				Annotations: map[string]string{
					esConfAnnotationKey(esSelector): esAssocConfAnnotation(commonv1.AssociationConf{
						AuthSecretName:       "-",
						CACertProvided:       true,
						CASecretName:         "fleet1-es-ca",
						ClientCertSecretName: "auto-generated-cert",
						URL:                  "https://es1-http.es-ns.svc:9200",
					}),
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{ObjectSelector: esSelector},
					},
				},
			},
			wantSecrets: []association.AdditionalSecret{
				{Source: types.NamespacedName{Namespace: "fs-ns", Name: "fleet1-es-ca"}, Keys: []string{"ca.crt"}, TargetName: "agent1-agent-fleetserver-" + hash.HashObject(types.NamespacedName{Name: "fleet1", Namespace: "fs-ns"}) + "-es-ca"},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assoc := &agentv1alpha1.AgentFleetServerAssociation{Agent: tt.agent}
			objects := []client.Object{tt.agent}
			if tt.fleetServer != nil {
				objects = append(objects, tt.fleetServer)
			}
			c := k8s.NewFakeClient(objects...)

			secrets, err := additionalSecrets(context.Background(), c, assoc)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantSecrets, secrets)
		})
	}
}

func TestDeleteOrphanedTransitiveClientCertSecrets(t *testing.T) {
	assocMeta := metadata.Metadata{
		Labels: map[string]string{
			"agentassociation.k8s.elastic.co/name":      "agent1",
			"agentassociation.k8s.elastic.co/namespace": "ns",
			"agentassociation.k8s.elastic.co/type":      commonv1.FleetServerAssociationType,
		},
	}
	associated := &agentv1alpha1.Agent{
		Name: "agent1", Namespace: "ns",
	}

	matchingLabels := map[string]string{
		"agentassociation.k8s.elastic.co/name":      "agent1",
		"agentassociation.k8s.elastic.co/namespace": "ns",
		"agentassociation.k8s.elastic.co/type":      commonv1.FleetServerAssociationType,
		labels.ClientCertificateLabelName:           "true",
		transitiveESClientCertLabel:                 "true",
	}

	// trustSource describes a trust source published with the agent's association labels.
	type trustSource struct {
		server     client.Object
		originName string
		transitive bool
		// agent overrides the agent the trust source was published for.
		agent string
		// extraLabels are labels of the trust source which do not identify the agent: the ones propagated from the
		// agent, or the ones of the Fleet Server it references.
		extraLabels map[string]string
	}
	otherAgentLabels := maps.Clone(matchingLabels)
	otherAgentLabels["agentassociation.k8s.elastic.co/name"] = "agent2"
	// the labels of the client certificates created by older operator versions: the server soft-owner labels, set to
	// Elasticsearch for the transitive client certificate and to the Agent for the Fleet Server client certificate,
	// and no transitive label
	legacyTransitiveLabels := maps.Clone(matchingLabels)
	delete(legacyTransitiveLabels, transitiveESClientCertLabel)
	legacyTransitiveLabels[reconciler.SoftOwnerNameLabel] = "es1"
	legacyTransitiveLabels[reconciler.SoftOwnerNamespaceLabel] = "es-ns"
	legacyTransitiveLabels[reconciler.SoftOwnerKindLabel] = esv1.Kind
	legacyFleetServerLabels := maps.Clone(legacyTransitiveLabels)
	legacyFleetServerLabels[reconciler.SoftOwnerNameLabel] = "fleet1"
	legacyFleetServerLabels[reconciler.SoftOwnerNamespaceLabel] = "fs-ns"
	legacyFleetServerLabels[reconciler.SoftOwnerKindLabel] = agentv1alpha1.Kind
	esServer := &esv1.Elasticsearch{Namespace: "es-ns", Name: "es1"}
	sameNamespaceESServer := &esv1.Elasticsearch{Namespace: "ns", Name: "es1"}
	fleetServer := &agentv1alpha1.Agent{Namespace: "fs-ns", Name: "fleet1"}

	for _, tt := range []struct {
		name                   string
		existingObjects        []client.Object
		trustSources           []trustSource
		currentSecretName      string
		wantRemaining          []string
		wantTrustSourceOrigins []string
	}{
		{
			name:          "no matching secrets is a no-op",
			wantRemaining: nil,
		},
		{
			name: "deletes all matching secrets when currentSecretName is empty",
			existingObjects: []client.Object{
				&corev1.Secret{Name: "old-cert-1", Namespace: "ns", Labels: matchingLabels},
				&corev1.Secret{Name: "old-cert-2", Namespace: "ns", Labels: matchingLabels},
			},
			currentSecretName: "",
			wantRemaining:     nil,
		},
		{
			name: "preserves current secret and deletes others",
			existingObjects: []client.Object{
				&corev1.Secret{Name: "current-cert", Namespace: "ns", Labels: matchingLabels},
				&corev1.Secret{Name: "orphaned-cert", Namespace: "ns", Labels: matchingLabels},
			},
			currentSecretName: "current-cert",
			wantRemaining:     []string{"current-cert"},
		},
		{
			// The trust source published by the agent's own Fleet Server association has the same association
			// labels, but not the transitive label: it must survive.
			name: "preserves the current trust source and deletes the others",
			trustSources: []trustSource{
				{server: esServer, originName: "current-cert", transitive: true},
				{server: esServer, originName: "orphaned-cert", transitive: true},
				{server: fleetServer, originName: "fleet-server-client-cert"},
			},
			currentSecretName:      "current-cert",
			wantTrustSourceOrigins: []string{"current-cert", "fleet-server-client-cert"},
		},
		{
			name: "deletes all transitive trust sources when currentSecretName is empty",
			trustSources: []trustSource{
				{server: esServer, originName: "orphaned-cert", transitive: true},
				{server: fleetServer, originName: "fleet-server-client-cert"},
			},
			currentSecretName:      "",
			wantTrustSourceOrigins: []string{"fleet-server-client-cert"},
		},
		{
			// A trust source published into an Elasticsearch running in the agent's namespace matches the client
			// certificate selector: it must not be deleted as an orphaned client certificate.
			name: "preserves the current trust source published into the agent namespace",
			existingObjects: []client.Object{
				&corev1.Secret{Name: "current-cert", Namespace: "ns", Labels: matchingLabels},
			},
			trustSources: []trustSource{
				{server: sameNamespaceESServer, originName: "current-cert", transitive: true},
			},
			currentSecretName:      "current-cert",
			wantRemaining:          []string{"current-cert"},
			wantTrustSourceOrigins: []string{"current-cert"},
		},
		{
			name: "keeps the secrets and trust sources of other agents",
			existingObjects: []client.Object{
				&corev1.Secret{Name: "orphaned-cert", Namespace: "ns", Labels: matchingLabels},
				&corev1.Secret{Name: "other-agent-cert", Namespace: "ns", Labels: otherAgentLabels},
			},
			trustSources: []trustSource{
				{server: esServer, originName: "orphaned-cert", transitive: true},
				{server: esServer, originName: "other-agent-cert", transitive: true, agent: "agent2"},
			},
			currentSecretName:      "current-cert",
			wantRemaining:          []string{"other-agent-cert"},
			wantTrustSourceOrigins: []string{"other-agent-cert"},
		},
		{
			name: "deletes the legacy transitive client certificates which are not the current one",
			existingObjects: []client.Object{
				&corev1.Secret{Name: "legacy-old-cert", Namespace: "ns", Labels: legacyTransitiveLabels},
				&corev1.Secret{Name: "legacy-current-cert", Namespace: "ns", Labels: legacyTransitiveLabels},
			},
			currentSecretName: "legacy-current-cert",
			wantRemaining:     []string{"legacy-current-cert"},
		},
		{
			name: "deletes all the legacy transitive client certificates when currentSecretName is empty",
			existingObjects: []client.Object{
				&corev1.Secret{Name: "legacy-old-cert", Namespace: "ns", Labels: legacyTransitiveLabels},
			},
			currentSecretName: "",
		},
		{
			name: "keeps the legacy Fleet Server client certificate",
			existingObjects: []client.Object{
				&corev1.Secret{Name: "legacy-fleet-server-cert", Namespace: "ns", Labels: legacyFleetServerLabels},
			},
			currentSecretName: "current-cert",
			wantRemaining:     []string{"legacy-fleet-server-cert"},
		},
		{
			name: "keeps the legacy transitive client certificates of other agents",
			existingObjects: []client.Object{
				&corev1.Secret{Name: "legacy-other-agent-cert", Namespace: "ns", Labels: utilmaps.Merge(maps.Clone(legacyTransitiveLabels), map[string]string{
					"agentassociation.k8s.elastic.co/name": "agent2",
				})},
			},
			currentSecretName: "current-cert",
			wantRemaining:     []string{"legacy-other-agent-cert"},
		},
		{
			// The labels propagated from the agent can change along with the Elasticsearch reference: the
			// certificate and the trust source of the previous reference carry the former ones.
			name: "deletes the orphaned secrets whatever the labels propagated from the agent",
			existingObjects: []client.Object{
				&corev1.Secret{Name: "orphaned-cert", Namespace: "ns", Labels: utilmaps.Merge(maps.Clone(matchingLabels), map[string]string{"team": "former"})},
				&corev1.Secret{Name: "legacy-orphaned-cert", Namespace: "ns", Labels: utilmaps.Merge(maps.Clone(legacyTransitiveLabels), map[string]string{"team": "former"})},
			},
			trustSources: []trustSource{
				{server: esServer, originName: "orphaned-cert", transitive: true, extraLabels: map[string]string{"team": "former"}},
			},
			currentSecretName: "current-cert",
		},
		{
			// The Fleet Server referenced by the agent can change along with the Elasticsearch reference.
			name: "deletes the orphaned secrets whatever the Fleet Server referenced by the agent",
			existingObjects: []client.Object{
				&corev1.Secret{Name: "orphaned-cert", Namespace: "ns", Labels: utilmaps.Merge(maps.Clone(matchingLabels), map[string]string{
					"agent.k8s.elastic.co/name": "former-fleet-server", "agent.k8s.elastic.co/namespace": "former-ns",
				})},
			},
			trustSources: []trustSource{
				{server: esServer, originName: "orphaned-cert", transitive: true, extraLabels: map[string]string{
					"agent.k8s.elastic.co/name": "former-fleet-server", "agent.k8s.elastic.co/namespace": "former-ns",
				}},
			},
			currentSecretName: "current-cert",
		},
		{
			name: "keeps the client certificates which are not transitive",
			existingObjects: []client.Object{
				&corev1.Secret{Name: "own-fleet-server-cert", Namespace: "ns", Labels: map[string]string{
					"agentassociation.k8s.elastic.co/name":      "agent1",
					"agentassociation.k8s.elastic.co/namespace": "ns",
					"agentassociation.k8s.elastic.co/type":      commonv1.FleetServerAssociationType,
					labels.ClientCertificateLabelName:           "true",
				}},
			},
			currentSecretName: "current-cert",
			wantRemaining:     []string{"own-fleet-server-cert"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(tt.existingObjects...)
			for _, ts := range tt.trustSources {
				meta := assocMeta
				if ts.agent != "" {
					meta = meta.Merge(metadata.Metadata{Labels: map[string]string{"agentassociation.k8s.elastic.co/name": ts.agent}})
				}
				if ts.transitive {
					meta = meta.Merge(metadata.Metadata{Labels: map[string]string{transitiveESClientCertLabel: "true"}})
				}
				meta = meta.Merge(metadata.Metadata{Labels: ts.extraLabels})
				require.NoError(t, certificates.ReconcileClientCertTrustSource(
					t.Context(), c, ts.server, types.NamespacedName{Namespace: "ns", Name: ts.originName},
					[]byte("cert"), meta,
				))
			}

			err := deleteOrphanedTransitiveESClientCertSecrets(t.Context(), c, associated, tt.currentSecretName)
			require.NoError(t, err)

			var secretList corev1.SecretList
			require.NoError(t, c.List(t.Context(), &secretList))
			var remaining []string
			for _, s := range secretList.Items {
				if _, isTrustSource := certificates.ClientCertTrustSourceOrigin(s); isTrustSource {
					continue
				}
				remaining = append(remaining, s.Name)
			}
			require.Equal(t, tt.wantRemaining, remaining)
			require.ElementsMatch(t, tt.wantTrustSourceOrigins, trustSourceOrigins(t, c))
		})
	}
}

// trustSourceOrigins returns the names of the client certificate secrets of all the published trust sources.
func trustSourceOrigins(t *testing.T, c k8s.Client) []string {
	t.Helper()
	trustSources, err := certificates.ListClientCertTrustSources(t.Context(), c, nil)
	require.NoError(t, err)
	var origins []string //nolint:prealloc
	for _, ts := range trustSources {
		origin, _ := certificates.ClientCertTrustSourceOrigin(ts)
		origins = append(origins, origin.Name)
	}
	return origins
}

func TestFleetManagedAgentTransitiveESRef(t *testing.T) {
	esSelector := commonv1.ObjectSelector{Name: "es1", Namespace: "es-ns"}

	assocMeta := metadata.Metadata{
		Labels: map[string]string{
			"agentassociation.k8s.elastic.co/name":      "agent1",
			"agentassociation.k8s.elastic.co/namespace": "ns",
			"agentassociation.k8s.elastic.co/type":      commonv1.FleetServerAssociationType,
		},
	}

	orphanLabels := map[string]string{
		"agentassociation.k8s.elastic.co/name":      "agent1",
		"agentassociation.k8s.elastic.co/namespace": "ns",
		"agentassociation.k8s.elastic.co/type":      commonv1.FleetServerAssociationType,
		labels.ClientCertificateLabelName:           "true",
		transitiveESClientCertLabel:                 "true",
	}

	transitiveAgent := &agentv1alpha1.Agent{
		Name: "agent1", Namespace: "ns",
		Spec: agentv1alpha1.AgentSpec{
			Version:        "8.0.0",
			FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
		},
	}
	transitiveFleetServer := func(esConf commonv1.AssociationConf, esRef agentv1alpha1.Output) *agentv1alpha1.Agent {
		return &agentv1alpha1.Agent{
			Name: "fleet1", Namespace: "fs-ns",
			Annotations: map[string]string{esConfAnnotationKey(esRef.ObjectSelector): esAssocConfAnnotation(esConf)},
			Spec: agentv1alpha1.AgentSpec{
				Version:            "8.0.0",
				FleetServerEnabled: true,
				ElasticsearchRefs:  []agentv1alpha1.Output{esRef},
			},
		}
	}
	managedESConf := commonv1.AssociationConf{
		AuthSecretName: "-",
		CACertProvided: true,
		CASecretName:   "fleet1-es-ca",
		URL:            "https://es1-http.es-ns.svc:9200",
	}
	clientAuthES := func(required bool) *esv1.Elasticsearch {
		es := &esv1.Elasticsearch{Name: "es1", Namespace: "es-ns"}
		if required {
			es.Annotations = map[string]string{annotation.ClientAuthenticationRequiredAnnotation: "true"}
		}
		return es
	}
	transitiveCertName := "agent1-agent-es-" + hash.HashObject(esSelector.NamespacedName()) + "-client-cert"
	transitiveCASecretName := association.AdditionalSecretNamer.Suffix("agent1", "agent-fleetserver", hash.HashObject(types.NamespacedName{Name: "fleet1", Namespace: "fs-ns"}), "es-ca")
	// legacyTransitiveCert is the transitive client certificate as created by older operator versions, with the
	// server soft-owner labels, which are not used any more.
	// legacyGeneratedCert returns a transitive client certificate as created by older operator versions: it has the
	// server soft-owner labels and no transitive label.
	legacyGeneratedCert := func(name string) *corev1.Secret {
		lbls := maps.Clone(orphanLabels)
		delete(lbls, transitiveESClientCertLabel)
		return &corev1.Secret{
			Name: name, Namespace: "ns",
			Labels: utilmaps.Merge(lbls, map[string]string{
				reconciler.SoftOwnerNameLabel:      "es1",
				reconciler.SoftOwnerNamespaceLabel: "es-ns",
				reconciler.SoftOwnerKindLabel:      esv1.Kind,
			}),
			Data: map[string][]byte{certificates.CertFileName: []byte("legacy-cert")},
		}
	}
	userCertCopyName := "agent1-agent-fleetserver-" + hash.HashObject(types.NamespacedName{Name: "fleet1", Namespace: "fs-ns"}) + "-es-client-cert"
	// previousES is an ES the Fleet Server referenced before es1, in the agent namespace.
	previousES := types.NamespacedName{Namespace: "ns", Name: "previous-es"}
	// legacyUserCertCopy is the copy of the Fleet Server user-provided client certificate as created by older operator
	// versions, with the server soft-owner labels of the Fleet Server client certificate, for previousES.
	legacyUserCertCopy := &corev1.Secret{
		Name: userCertCopyName, Namespace: "ns",
		Labels: map[string]string{
			labels.ClientCertificateLabelName:  "true",
			reconciler.SoftOwnerNameLabel:      previousES.Name,
			reconciler.SoftOwnerNamespaceLabel: previousES.Namespace,
			reconciler.SoftOwnerKindLabel:      esv1.Kind,
		},
		Data: map[string][]byte{certificates.CertFileName: []byte("user-cert")},
	}
	legacyTransitiveCert := &corev1.Secret{
		Name: transitiveCertName, Namespace: "ns",
		Labels: utilmaps.Merge(maps.Clone(orphanLabels), map[string]string{
			reconciler.SoftOwnerNameLabel:      "es1",
			reconciler.SoftOwnerNamespaceLabel: "es-ns",
			reconciler.SoftOwnerKindLabel:      esv1.Kind,
		}),
		Data: map[string][]byte{certificates.CertFileName: []byte("legacy-cert")},
	}

	for _, tt := range []struct {
		name              string
		agent             *agentv1alpha1.Agent
		fleetServer       *agentv1alpha1.Agent
		es                *esv1.Elasticsearch
		orphanedSecrets   []*corev1.Secret
		wantRef           *commonv1.TransitiveESRef
		wantHasError      bool
		wantNilResults    bool
		wantOrphanGone    bool
		wantCreatedSecret *types.NamespacedName
		// wantDeletedSecrets are the secrets of the agent namespace expected to be deleted.
		wantDeletedSecrets []types.NamespacedName
		// staleTrustSources lists the origins of transitive trust sources published before the call.
		staleTrustSources []string
		// interceptorFuncs makes the client fail.
		interceptorFuncs *interceptor.Funcs
		// wantTrustSources lists the trust sources expected to be published into the ES namespace.
		wantTrustSources []string
		// wantKeptUserCertCopy is the user-provided client certificate copy expected to be kept, even when previousES
		// is deleted.
		wantKeptUserCertCopy *types.NamespacedName
	}{
		{
			name: "fleet server ref not set returns nil and cleans up",
			agent: &agentv1alpha1.Agent{
				Name: "agent1", Namespace: "ns",
				Spec: agentv1alpha1.AgentSpec{Version: "8.0.0"},
			},
			orphanedSecrets: []*corev1.Secret{
				{Name: "old-cert", Namespace: "ns", Labels: orphanLabels},
			},
			wantRef:        nil,
			wantNilResults: true,
			wantOrphanGone: true,
		},
		{
			name: "fleet server has no ES refs returns nil and cleans up",
			agent: &agentv1alpha1.Agent{
				Name: "agent1", Namespace: "ns",
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				Name: "fleet1", Namespace: "fs-ns",
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
				},
			},
			wantRef:        nil,
			wantNilResults: true,
		},
		{
			name: "ES without client auth annotation returns CA-only TransitiveESRef and cleans up",
			agent: &agentv1alpha1.Agent{
				Name: "agent1", Namespace: "ns",
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				Name: "fleet1", Namespace: "fs-ns",
				Annotations: map[string]string{
					esConfAnnotationKey(esSelector): esAssocConfAnnotation(commonv1.AssociationConf{
						AuthSecretName: "-",
						CACertProvided: true,
						CASecretName:   "fleet1-es-ca",
						URL:            "https://es1-http.es-ns.svc:9200",
					}),
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{ObjectSelector: esSelector},
					},
				},
			},
			es: &esv1.Elasticsearch{
				Name: "es1", Namespace: "es-ns",
			},
			wantRef: &commonv1.TransitiveESRef{
				CASecretName: association.AdditionalSecretNamer.Suffix("agent1", "agent-fleetserver", hash.HashObject(types.NamespacedName{Name: "fleet1", Namespace: "fs-ns"}), "es-ca"),
			},
			wantNilResults: true,
		},
		{
			name: "ES has client auth and fleet server has user-provided cert",
			agent: &agentv1alpha1.Agent{
				Name: "agent1", Namespace: "ns",
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				Name: "fleet1", Namespace: "fs-ns",
				Annotations: map[string]string{
					esConfAnnotationKey(esSelector): esAssocConfAnnotation(commonv1.AssociationConf{
						AuthSecretName:       "-",
						CACertProvided:       true,
						CASecretName:         "fleet1-es-ca",
						ClientCertSecretName: "copied-user-cert",
						URL:                  "https://es1-http.es-ns.svc:9200",
					}),
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{
							ObjectSelector:              esSelector,
							ClientCertificateSecretName: "user-provided-cert"},
					},
				},
			},
			es: &esv1.Elasticsearch{
				Name: "es1", Namespace: "es-ns",
				Annotations: map[string]string{
					annotation.ClientAuthenticationRequiredAnnotation: "true",
				},
			},
			wantRef: &commonv1.TransitiveESRef{
				CASecretName:         association.AdditionalSecretNamer.Suffix("agent1", "agent-fleetserver", hash.HashObject(types.NamespacedName{Name: "fleet1", Namespace: "fs-ns"}), "es-ca"),
				ClientCertSecretName: "agent1-agent-fleetserver-" + hash.HashObject(types.NamespacedName{Name: "fleet1", Namespace: "fs-ns"}) + "-es-client-cert",
			},
		},
		{
			name: "ES has client auth and no user cert returns auto-generated secret name",
			agent: &agentv1alpha1.Agent{
				Name: "agent1", Namespace: "ns",
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				Name: "fleet1", Namespace: "fs-ns",
				Annotations: map[string]string{
					esConfAnnotationKey(esSelector): esAssocConfAnnotation(commonv1.AssociationConf{
						AuthSecretName: "-",
						CACertProvided: true,
						CASecretName:   "fleet1-es-ca",
						URL:            "https://es1-http.es-ns.svc:9200",
					}),
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{ObjectSelector: esSelector},
					},
				},
			},
			es: &esv1.Elasticsearch{
				Name: "es1", Namespace: "es-ns",
				Annotations: map[string]string{
					annotation.ClientAuthenticationRequiredAnnotation: "true",
				},
			},
			wantRef: &commonv1.TransitiveESRef{
				CASecretName:         association.AdditionalSecretNamer.Suffix("agent1", "agent-fleetserver", hash.HashObject(types.NamespacedName{Name: "fleet1", Namespace: "fs-ns"}), "es-ca"),
				ClientCertSecretName: "agent1-agent-es-" + hash.HashObject(esSelector.NamespacedName()) + "-client-cert",
			},
			wantCreatedSecret: &types.NamespacedName{
				Namespace: "ns",
				Name:      "agent1-agent-es-" + hash.HashObject(esSelector.NamespacedName()) + "-client-cert",
			},
			wantTrustSources: []string{"agent1-agent-es-" + hash.HashObject(esSelector.NamespacedName()) + "-client-cert"},
		},
		{
			name: "external ES ref skips client cert reconciliation and cleans up",
			agent: &agentv1alpha1.Agent{
				Name: "agent1", Namespace: "ns",
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				Name: "fleet1", Namespace: "fs-ns",
				Annotations: map[string]string{
					esConfAnnotationKey(commonv1.ObjectSelector{SecretName: "external-es-secret"}): esAssocConfAnnotation(commonv1.AssociationConf{
						AuthSecretName: "external-es-secret",
						URL:            "https://external-es:9200",
					}),
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{SecretName: "external-es-secret"},
					},
				},
			},
			orphanedSecrets: []*corev1.Secret{
				{Name: "old-cert", Namespace: "ns", Labels: orphanLabels},
			},
			wantRef:        nil,
			wantNilResults: true,
			wantOrphanGone: true,
		},
		{
			name: "same-namespace fleet server with client auth sets CA secret name to original",
			agent: &agentv1alpha1.Agent{
				Name: "agent1", Namespace: "fs-ns",
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				Name: "fleet1", Namespace: "fs-ns",
				Annotations: map[string]string{
					esConfAnnotationKey(esSelector): esAssocConfAnnotation(commonv1.AssociationConf{
						AuthSecretName: "-",
						CACertProvided: true,
						CASecretName:   "fleet1-es-ca",
						URL:            "https://es1-http.es-ns.svc:9200",
					}),
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{ObjectSelector: esSelector},
					},
				},
			},
			es: &esv1.Elasticsearch{
				Name: "es1", Namespace: "es-ns",
				Annotations: map[string]string{
					annotation.ClientAuthenticationRequiredAnnotation: "true",
				},
			},
			wantRef: &commonv1.TransitiveESRef{
				CASecretName:         "fleet1-es-ca",
				ClientCertSecretName: "agent1-agent-es-" + hash.HashObject(esSelector.NamespacedName()) + "-client-cert",
			},
			wantCreatedSecret: &types.NamespacedName{
				Namespace: "fs-ns",
				Name:      "agent1-agent-es-" + hash.HashObject(esSelector.NamespacedName()) + "-client-cert",
			},
			wantTrustSources: []string{"agent1-agent-es-" + hash.HashObject(esSelector.NamespacedName()) + "-client-cert"},
		},
		{
			name: "same-namespace fleet server with user-provided cert returns original client cert name",
			agent: &agentv1alpha1.Agent{
				Name: "agent1", Namespace: "fs-ns",
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				Name: "fleet1", Namespace: "fs-ns",
				Annotations: map[string]string{
					esConfAnnotationKey(esSelector): esAssocConfAnnotation(commonv1.AssociationConf{
						AuthSecretName:       "-",
						CACertProvided:       true,
						CASecretName:         "fleet1-es-ca",
						ClientCertSecretName: "user-provided-cert",
						URL:                  "https://es1-http.es-ns.svc:9200",
					}),
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{
							ObjectSelector:              esSelector,
							ClientCertificateSecretName: "user-provided-cert"},
					},
				},
			},
			es: &esv1.Elasticsearch{
				Name: "es1", Namespace: "es-ns",
				Annotations: map[string]string{
					annotation.ClientAuthenticationRequiredAnnotation: "true",
				},
			},
			wantRef: &commonv1.TransitiveESRef{
				CASecretName:         "fleet1-es-ca",
				ClientCertSecretName: "user-provided-cert",
			},
		},
		{
			name:              "legacy soft-owner labels are removed from the current transitive client certificate",
			agent:             transitiveAgent,
			fleetServer:       transitiveFleetServer(managedESConf, agentv1alpha1.Output{ObjectSelector: esSelector}),
			es:                clientAuthES(true),
			orphanedSecrets:   []*corev1.Secret{legacyTransitiveCert},
			wantRef:           &commonv1.TransitiveESRef{CASecretName: transitiveCASecretName, ClientCertSecretName: transitiveCertName},
			wantCreatedSecret: &types.NamespacedName{Namespace: "ns", Name: transitiveCertName},
			wantTrustSources:  []string{transitiveCertName},
		},
		{
			name:              "transitive trust sources are removed when the ES no longer requires client auth",
			agent:             transitiveAgent,
			fleetServer:       transitiveFleetServer(managedESConf, agentv1alpha1.Output{ObjectSelector: esSelector}),
			es:                clientAuthES(false),
			orphanedSecrets:   []*corev1.Secret{{Name: transitiveCertName, Namespace: "ns", Labels: orphanLabels}},
			staleTrustSources: []string{transitiveCertName},
			wantRef:           &commonv1.TransitiveESRef{CASecretName: transitiveCASecretName},
			wantNilResults:    true,
			wantOrphanGone:    true,
		},
		{
			name:               "a legacy transitive client certificate of a previous ES reference is removed",
			agent:              transitiveAgent,
			fleetServer:        transitiveFleetServer(managedESConf, agentv1alpha1.Output{ObjectSelector: esSelector}),
			es:                 clientAuthES(true),
			orphanedSecrets:    []*corev1.Secret{legacyGeneratedCert("agent1-agent-es-previous-ref-client-cert")},
			wantRef:            &commonv1.TransitiveESRef{CASecretName: transitiveCASecretName, ClientCertSecretName: transitiveCertName},
			wantCreatedSecret:  &types.NamespacedName{Namespace: "ns", Name: transitiveCertName},
			wantTrustSources:   []string{transitiveCertName},
			wantDeletedSecrets: []types.NamespacedName{{Namespace: "ns", Name: "agent1-agent-es-previous-ref-client-cert"}},
		},
		{
			name:               "a legacy transitive client certificate is removed when the ES no longer requires client auth",
			agent:              transitiveAgent,
			fleetServer:        transitiveFleetServer(managedESConf, agentv1alpha1.Output{ObjectSelector: esSelector}),
			es:                 clientAuthES(false),
			orphanedSecrets:    []*corev1.Secret{legacyGeneratedCert(transitiveCertName)},
			wantRef:            &commonv1.TransitiveESRef{CASecretName: transitiveCASecretName},
			wantNilResults:     true,
			wantDeletedSecrets: []types.NamespacedName{{Namespace: "ns", Name: transitiveCertName}},
		},
		{
			name:  "a legacy generated transitive client certificate is removed when the fleet server has a user-provided one",
			agent: transitiveAgent,
			fleetServer: transitiveFleetServer(
				commonv1.AssociationConf{AuthSecretName: "-", CACertProvided: true, CASecretName: "fleet1-es-ca", ClientCertSecretName: "copied-user-cert", URL: "https://es1-http.es-ns.svc:9200"},
				agentv1alpha1.Output{ObjectSelector: esSelector, ClientCertificateSecretName: "user-provided-cert"},
			),
			es:              clientAuthES(true),
			orphanedSecrets: []*corev1.Secret{legacyGeneratedCert(transitiveCertName)},
			wantRef: &commonv1.TransitiveESRef{
				CASecretName:         transitiveCASecretName,
				ClientCertSecretName: "agent1-agent-fleetserver-" + hash.HashObject(types.NamespacedName{Name: "fleet1", Namespace: "fs-ns"}) + "-es-client-cert",
			},
			wantDeletedSecrets: []types.NamespacedName{{Namespace: "ns", Name: transitiveCertName}},
		},
		{
			name:  "legacy soft-owner labels are removed from the copy of the fleet server user-provided client certificate",
			agent: transitiveAgent,
			fleetServer: transitiveFleetServer(
				commonv1.AssociationConf{AuthSecretName: "-", CACertProvided: true, CASecretName: "fleet1-es-ca", ClientCertSecretName: "copied-user-cert", URL: "https://es1-http.es-ns.svc:9200"},
				agentv1alpha1.Output{ObjectSelector: esSelector, ClientCertificateSecretName: "user-provided-cert"},
			),
			es:                   clientAuthES(true),
			orphanedSecrets:      []*corev1.Secret{legacyUserCertCopy},
			wantRef:              &commonv1.TransitiveESRef{CASecretName: transitiveCASecretName, ClientCertSecretName: userCertCopyName},
			wantKeptUserCertCopy: &types.NamespacedName{Namespace: "ns", Name: userCertCopyName},
		},
		{
			name:        "a failure to publish the trust source returns the error and no reference",
			agent:       transitiveAgent,
			fleetServer: transitiveFleetServer(managedESConf, agentv1alpha1.Output{ObjectSelector: esSelector}),
			es:          clientAuthES(true),
			interceptorFuncs: &interceptor.Funcs{
				Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
					if obj.GetLabels()[commonv1.TypeLabelName] == certificates.ClientCertTrustSourceType {
						return errors.New("simulated trust source create failure")
					}
					return c.Create(ctx, obj, opts...)
				},
			},
			wantRef:      nil,
			wantHasError: true,
		},
		{
			name: "conf is nil returns nil and cleans up",
			agent: &agentv1alpha1.Agent{
				Name: "agent1", Namespace: "ns",
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				Name: "fleet1", Namespace: "fs-ns",
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{ObjectSelector: esSelector},
					},
				},
			},
			wantRef:        nil,
			wantNilResults: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var objects []client.Object
			objects = append(objects, tt.agent)
			if tt.fleetServer != nil {
				objects = append(objects, tt.fleetServer)
			}
			if tt.es != nil {
				objects = append(objects, tt.es)
			}
			for _, s := range tt.orphanedSecrets {
				objects = append(objects, s)
			}
			builder := k8s.NewFakeClientBuilder(objects...)
			if tt.interceptorFuncs != nil {
				builder = builder.WithInterceptorFuncs(*tt.interceptorFuncs)
			}
			c := builder.Build()
			stale := clientAuthES(true)
			for _, origin := range tt.staleTrustSources {
				require.NoError(t, certificates.ReconcileClientCertTrustSource(
					t.Context(), c, stale, types.NamespacedName{Namespace: "ns", Name: origin}, []byte("cert"),
					assocMeta.Merge(metadata.Metadata{Labels: map[string]string{transitiveESClientCertLabel: "true"}}),
				))
			}
			assoc := &agentv1alpha1.AgentFleetServerAssociation{Agent: tt.agent}

			ref, results := fleetManagedAgentTransitiveESRef(context.Background(), c, assoc, assocMeta)

			// The managed transitive client certificate must be published into the ES namespace, otherwise ES does
			// not trust it.
			trustSources, err := certificates.ListClientCertTrustSources(t.Context(), c, nil)
			require.NoError(t, err)
			var gotTrustSources []string //nolint:prealloc
			for _, ts := range trustSources {
				require.Equal(t, "es-ns", ts.Namespace, "trust source %s must be published into the ES namespace", ts.Name)
				require.Equal(t, "true", ts.Labels[transitiveESClientCertLabel], "trust source %s must be marked as transitive", ts.Name)
				origin, _ := certificates.ClientCertTrustSourceOrigin(ts)
				gotTrustSources = append(gotTrustSources, origin.Name)
			}
			require.Equal(t, tt.wantTrustSources, gotTrustSources)

			if tt.wantRef == nil {
				require.Nil(t, ref)
			} else {
				require.NotNil(t, ref)
				require.Equal(t, tt.wantRef.CASecretName, ref.CASecretName)
				require.Equal(t, tt.wantRef.ClientCertSecretName, ref.ClientCertSecretName)
			}

			if tt.wantNilResults {
				require.Nil(t, results)
			} else {
				require.NotNil(t, results)
				require.Equal(t, tt.wantHasError, results.HasError())
			}

			if tt.wantOrphanGone {
				var secretList corev1.SecretList
				require.NoError(t, c.List(context.Background(), &secretList))
				for _, s := range secretList.Items {
					require.NotEqual(t, "true", s.Labels[labels.ClientCertificateLabelName],
						"orphaned client cert secret %s should have been deleted", s.Name)
				}
			}

			for _, nsn := range tt.wantDeletedSecrets {
				var deleted corev1.Secret
				require.True(t, apierrors.IsNotFound(c.Get(context.Background(), nsn, &deleted)), "secret %s must be deleted", nsn)
			}

			if tt.wantCreatedSecret != nil {
				// Verify the secret was actually persisted in the fake client, not just named.
				var secret corev1.Secret
				err := c.Get(context.Background(), *tt.wantCreatedSecret, &secret)
				require.NoError(t, err, "expected secret %s/%s to exist in the fake client", tt.wantCreatedSecret.Namespace, tt.wantCreatedSecret.Name)
				require.Equal(t, "true", secret.Labels[transitiveESClientCertLabel])
				for _, l := range []string{reconciler.SoftOwnerNameLabel, reconciler.SoftOwnerNamespaceLabel, reconciler.SoftOwnerKindLabel} {
					require.NotContains(t, secret.Labels, l, "the client certificate must not carry server soft-owner labels")
				}
			}

			if tt.wantKeptUserCertCopy != nil {
				// The deletion of the previously referenced ES garbage collects the secrets it soft-owns.
				require.NoError(t, reconciler.GarbageCollectSoftOwnedSecrets(t.Context(), c, previousES, esv1.Kind))
				var secret corev1.Secret
				require.NoError(t, c.Get(t.Context(), *tt.wantKeptUserCertCopy, &secret), "the user-provided client certificate copy must be kept")
				for _, l := range []string{reconciler.SoftOwnerNameLabel, reconciler.SoftOwnerNamespaceLabel, reconciler.SoftOwnerKindLabel} {
					require.NotContains(t, secret.Labels, l, "the client certificate must not carry server soft-owner labels")
				}
			}
		})
	}
}

// Verify that fleetManagedAgentTransitiveESRef returns non-nil results (not error) for success cases.
func TestFleetManagedAgentTransitiveESRef_ResultsNotNil(t *testing.T) {
	esSelector := commonv1.ObjectSelector{Name: "es1", Namespace: "es-ns"}
	assocMeta := metadata.Metadata{
		Labels: map[string]string{
			"agentassociation.k8s.elastic.co/name":      "agent1",
			"agentassociation.k8s.elastic.co/namespace": "ns",
			"agentassociation.k8s.elastic.co/type":      commonv1.FleetServerAssociationType,
		},
	}

	agent := &agentv1alpha1.Agent{
		Name: "agent1", Namespace: "ns",
		Spec: agentv1alpha1.AgentSpec{
			Version:        "8.0.0",
			FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
		},
	}
	fleetServer := &agentv1alpha1.Agent{
		Name: "fleet1", Namespace: "fs-ns",
		Annotations: map[string]string{
			esConfAnnotationKey(esSelector): esAssocConfAnnotation(commonv1.AssociationConf{
				AuthSecretName:       "-",
				CACertProvided:       true,
				CASecretName:         "fleet1-es-ca",
				ClientCertSecretName: "copied-user-cert",
				URL:                  "https://es1-http.es-ns.svc:9200",
			}),
		},
		Spec: agentv1alpha1.AgentSpec{
			Version:            "8.0.0",
			FleetServerEnabled: true,
			ElasticsearchRefs: []agentv1alpha1.Output{
				{
					ObjectSelector:              esSelector,
					ClientCertificateSecretName: "user-provided-cert"},
			},
		},
	}
	es := &esv1.Elasticsearch{
		Name: "es1", Namespace: "es-ns",
		Annotations: map[string]string{
			annotation.ClientAuthenticationRequiredAnnotation: "true",
		},
	}

	c := k8s.NewFakeClient(agent, fleetServer, es)
	assoc := &agentv1alpha1.AgentFleetServerAssociation{Agent: agent}

	ref, results := fleetManagedAgentTransitiveESRef(context.Background(), c, assoc, assocMeta)
	require.NotNil(t, ref)
	require.NotNil(t, results)
	require.False(t, results.HasError())
}

func TestAgentFleetServerESRef(t *testing.T) {
	esSelector := commonv1.ObjectSelector{Name: "es1", Namespace: "es-ns"}

	fleetServer := &agentv1alpha1.Agent{
		Name: "fleet1", Namespace: "fs-ns",
		Spec: agentv1alpha1.AgentSpec{
			FleetServerEnabled: true,
			ElasticsearchRefs: []agentv1alpha1.Output{
				{ObjectSelector: esSelector},
			},
		},
	}
	fleetServer.GetAssociations()[0].SetAssociationConf(&commonv1.AssociationConf{
		AuthSecretName: "-",
		CACertProvided: true,
		CASecretName:   "fleet1-es-ca",
		URL:            "https://es1-http.es-ns.svc:9200",
	})

	agent := &agentv1alpha1.Agent{
		Name: "agent1", Namespace: "agent-ns",
		Spec: agentv1alpha1.AgentSpec{
			FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
		},
	}

	agentNoRef := &agentv1alpha1.Agent{
		Name: "agent1", Namespace: "agent-ns",
	}

	for _, tt := range []struct {
		name         string
		agent        *agentv1alpha1.Agent // nil uses the default agent fixture
		objects      []client.Object
		wantFound    bool
		wantESNSName types.NamespacedName
		wantErr      bool
	}{
		{
			name:         "resolves transitive ES ref through fleet server",
			objects:      []client.Object{agent, fleetServer},
			wantFound:    true,
			wantESNSName: types.NamespacedName{Namespace: "es-ns", Name: "es1"},
		},
		{
			name:      "fleet server not found returns not-found",
			objects:   []client.Object{agent},
			wantFound: false,
		},
		{
			name:      "fleet server ref not set returns not-found",
			agent:     agentNoRef,
			wantFound: false,
		},
		{
			name: "fleet server has no ES refs returns not-found",
			objects: []client.Object{agent, func() *agentv1alpha1.Agent {
				fs := fleetServer.DeepCopy()
				fs.Spec.ElasticsearchRefs = nil
				return fs
			}()},
			wantFound: false,
		},
		{
			name: "fleet server with multiple ES refs returns error",
			objects: []client.Object{agent, func() *agentv1alpha1.Agent {
				fs := fleetServer.DeepCopy()
				fs.Spec.ElasticsearchRefs = append(fs.Spec.ElasticsearchRefs,
					agentv1alpha1.Output{Name: "es2", Namespace: "es-ns"},
				)
				return fs
			}()},
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(tt.objects...)
			a := agent
			if tt.agent != nil {
				a = tt.agent
			}
			assoc := &agentv1alpha1.AgentFleetServerAssociation{Agent: a}

			found, ref, err := agentFleetServerESRef(t.Context(), c, assoc)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantFound, found)
			if tt.wantFound {
				require.Equal(t, tt.wantESNSName, ref.NamespacedName())
			}
		})
	}
}

// denyESReviewer allows any object that is not an Elasticsearch resource.
// It simulates a scenario where the Fleet Server passes the RBAC check but
// the backing Elasticsearch cluster does not.
type denyESReviewer struct{}

func (d denyESReviewer) AccessAllowed(_ context.Context, _ string, _ string, obj runtime.Object) (bool, error) {
	_, isES := obj.(*esv1.Elasticsearch)
	return !isES, nil
}

var _ rbac.AccessReviewer = denyESReviewer{}

// denyAgentReviewer denies access to any Agent-typed object, allowing everything else.
type denyAgentReviewer struct{}

func (d denyAgentReviewer) AccessAllowed(_ context.Context, _ string, _ string, obj runtime.Object) (bool, error) {
	_, isAgent := obj.(*agentv1alpha1.Agent)
	return !isAgent, nil
}

var _ rbac.AccessReviewer = denyAgentReviewer{}

func TestAgentFleetServerTransitiveESRBAC(t *testing.T) {
	agentObj := &agentv1alpha1.Agent{
		Name: "agent1", Namespace: "agent-ns",
		Spec: agentv1alpha1.AgentSpec{
			Version:        "8.0.0",
			FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fleet-ns"}},
		},
	}

	fleetServer := &agentv1alpha1.Agent{
		Name:      "fleet1",
		Namespace: "fleet-ns",
		Annotations: map[string]string{
			esConfAnnotationKey(commonv1.ObjectSelector{Name: "es1", Namespace: "es-ns"}): esAssocConfAnnotation(commonv1.AssociationConf{
				AuthSecretName: "-",
				CACertProvided: true,
				CASecretName:   "fleet1-es-ca",
				URL:            "https://es1-http.es-ns.svc:9200",
			}),
		},
		Spec: agentv1alpha1.AgentSpec{
			Version:            "8.0.0",
			FleetServerEnabled: true,
			ElasticsearchRefs: []agentv1alpha1.Output{
				{Name: "es1", Namespace: "es-ns"},
			},
		},
	}
	fleetServer.GetAssociations()[0].SetAssociationConf(&commonv1.AssociationConf{
		AuthSecretName: "-",
		CACertProvided: true,
		CASecretName:   "fleet1-es-ca",
		URL:            "https://es1-http.es-ns.svc:9200",
	})

	es := &esv1.Elasticsearch{
		Name: "es1", Namespace: "es-ns",
		Spec: esv1.ElasticsearchSpec{Version: "8.0.0"},
	}

	// CA secret in the Fleet Server namespace — would be copied to agent-ns on success.
	fleetESCA := &corev1.Secret{
		Namespace: "fleet-ns", Name: "fleet1-es-ca",
		Data: map[string][]byte{"ca.crt": []byte("cacert"), "tls.crt": []byte("tlscert")},
	}
	fleetHTTPCerts := &corev1.Secret{
		Namespace: "fleet-ns", Name: "fleet1-agent-http-certs-public",
		Data: map[string][]byte{"ca.crt": []byte("cacert"), "tls.crt": []byte("tlscert")},
	}
	fleetService := &corev1.Service{
		Namespace: "fleet-ns", Name: "fleet1-agent-http",
		Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "https", Port: 8220}}},
	}

	// Fleet Server with no ElasticsearchRefs — manual/external setup, no transitive ES.
	fleetServerNoES := &agentv1alpha1.Agent{
		Name: "fleet1", Namespace: "fleet-ns",
		Spec: agentv1alpha1.AgentSpec{
			Version:            "8.0.0",
			FleetServerEnabled: true,
		},
	}

	// Fleet Server whose Elasticsearch is unmanaged (external secret). The RBAC check is skipped
	// for external associations, so the agent association should establish regardless of the reviewer.
	fleetServerWithExternalES := &agentv1alpha1.Agent{
		Name: "fleet1", Namespace: "fleet-ns",
		Spec: agentv1alpha1.AgentSpec{
			Version:            "8.0.0",
			FleetServerEnabled: true,
			ElasticsearchRefs: []agentv1alpha1.Output{
				{
					SecretName: "external-es-secret"},
			},
		},
	}

	// agentObjEstablished simulates an agent whose FleetServer association was previously established.
	agentObjEstablished := agentObj.DeepCopy()
	agentObjEstablished.Annotations = map[string]string{
		commonv1.FleetServerConfigAnnotationNameBase: esAssocConfAnnotation(commonv1.AssociationConf{
			AuthSecretName: commonv1.NoAuthRequiredValue,
			CACertProvided: true,
			CASecretName:   "fleet1-agent-http-certs-public",
			URL:            "https://fleet1-agent-http.fleet-ns.svc:8220",
		}),
	}

	// staleCAInAgentNs simulates a transitive ES CA copy that was placed in the agent namespace
	// during a previous reconcile where RBAC was allowed. It must be actively deleted when RBAC is revoked.
	staleCAInAgentNs := &corev1.Secret{
		Name:      "stale-es-ca",
		Namespace: "agent-ns",
		Labels: map[string]string{
			association.AdditionalSecretLabelName: "true",
			AgentAssociationLabelName:             "agent1",
			AgentAssociationLabelNamespace:        "agent-ns",
			AgentAssociationLabelType:             commonv1.FleetServerAssociationType,
			agent.NameLabelName:                   "fleet1",
			agent.NamespaceLabelName:              "fleet-ns",
		},
		Data: map[string][]byte{"ca.crt": []byte("cacert")},
	}

	// fleetServerSameNameAsES is a Fleet Server whose backing Elasticsearch shares its own
	// namespace and name ("fleet1/fleet-ns"). This is the minimal fixture to trigger the
	// kind-guard regression: without the directRefIsES check, NamespacedName equality
	// between esAssocRef and assocRef silently skips the Fleet Server RBAC check.
	fleetServerSameNameAsES := &agentv1alpha1.Agent{
		Name: "fleet1", Namespace: "fleet-ns",
		Spec: agentv1alpha1.AgentSpec{
			Version:            "8.0.0",
			FleetServerEnabled: true,
			ElasticsearchRefs: []agentv1alpha1.Output{
				{Name: "fleet1", Namespace: "fleet-ns"},
			},
		},
	}
	esSameName := &esv1.Elasticsearch{
		Name: "fleet1", Namespace: "fleet-ns",
		Spec: esv1.ElasticsearchSpec{Version: "8.0.0"},
	}

	for _, tt := range []struct {
		name            string
		accessReviewer  rbac.AccessReviewer
		rbacMode        operator.RBACOnRefsMode
		runtimeObjs     []client.Object
		wantStatus      commonv1.AssociationStatus
		wantCAInAgentNs bool
		wantConfCleared bool
	}{
		{
			name:            "transitive ES RBAC allowed: association established and CA copied",
			accessReviewer:  rbac.NewPermissiveAccessReviewer(),
			rbacMode:        operator.RBACOnRefsModeAll,
			runtimeObjs:     []client.Object{agentObj.DeepCopy(), fleetServer, es, fleetESCA, fleetHTTPCerts, fleetService},
			wantStatus:      commonv1.AssociationEstablished,
			wantCAInAgentNs: true,
		},
		{
			name:            "transitive ES RBAC denied: association pending and CA not copied",
			accessReviewer:  denyESReviewer{},
			rbacMode:        operator.RBACOnRefsModeAll,
			runtimeObjs:     []client.Object{agentObj.DeepCopy(), fleetServer, es, fleetESCA, fleetHTTPCerts, fleetService},
			wantStatus:      commonv1.AssociationPending,
			wantCAInAgentNs: false,
		},
		{
			name:            "fleet server has no ES ref (manual setup): association establishes",
			accessReviewer:  rbac.NewPermissiveAccessReviewer(),
			rbacMode:        operator.RBACOnRefsModeAll,
			runtimeObjs:     []client.Object{agentObj.DeepCopy(), fleetServerNoES, fleetHTTPCerts, fleetService},
			wantStatus:      commonv1.AssociationEstablished,
			wantCAInAgentNs: false,
		},
		{
			name:            "established association is unbound when transitive ES RBAC is revoked",
			accessReviewer:  denyESReviewer{},
			runtimeObjs:     []client.Object{agentObjEstablished, fleetServer, es, fleetESCA, fleetHTTPCerts, fleetService, staleCAInAgentNs},
			wantStatus:      commonv1.AssociationPending,
			wantCAInAgentNs: false,
			wantConfCleared: true,
		},
		{
			name:            "transitive ES not found yet: association pending and CA not copied",
			accessReviewer:  rbac.NewPermissiveAccessReviewer(),
			runtimeObjs:     []client.Object{agentObj.DeepCopy(), fleetServer, fleetHTTPCerts, fleetService},
			wantStatus:      commonv1.AssociationPending,
			wantCAInAgentNs: false,
		},
		{
			name:            "fleet server absent: association pending and CA not copied",
			accessReviewer:  rbac.NewPermissiveAccessReviewer(),
			runtimeObjs:     []client.Object{agentObj.DeepCopy()},
			wantStatus:      commonv1.AssociationPending,
			wantCAInAgentNs: false,
		},
		{
			// denyESReviewer is used deliberately: external ES skips the RBAC check entirely,
			// so the association must establish even though the reviewer would deny ES access.
			name:            "fleet server ES is external (unmanaged): RBAC check skipped, association establishes",
			accessReviewer:  denyESReviewer{},
			runtimeObjs:     []client.Object{agentObj.DeepCopy(), fleetServerWithExternalES, fleetHTTPCerts, fleetService},
			wantStatus:      commonv1.AssociationEstablished,
			wantCAInAgentNs: false,
		},
		{
			name:            "fleet server removes ES ref (switches to manual setup): stale CA copy cleaned up",
			accessReviewer:  rbac.NewPermissiveAccessReviewer(),
			runtimeObjs:     []client.Object{agentObjEstablished, fleetServerNoES, fleetHTTPCerts, fleetService, staleCAInAgentNs},
			wantStatus:      commonv1.AssociationEstablished,
			wantCAInAgentNs: false,
		},
		{
			name:            "transitive ES RBAC denied in true mode: association pending",
			accessReviewer:  denyESReviewer{},
			rbacMode:        operator.RBACOnRefsModeTrue,
			runtimeObjs:     []client.Object{agentObj.DeepCopy(), fleetServer, es, fleetESCA, fleetHTTPCerts, fleetService},
			wantStatus:      commonv1.AssociationPending,
			wantCAInAgentNs: false,
		},
		{
			name:            "transitive ES RBAC denied in legacy mode: association pending",
			accessReviewer:  denyESReviewer{},
			rbacMode:        operator.RBACOnRefsModeLegacy,
			runtimeObjs:     []client.Object{agentObj.DeepCopy(), fleetServer, es, fleetESCA, fleetHTTPCerts, fleetService},
			wantStatus:      commonv1.AssociationPending,
			wantCAInAgentNs: false,
		},
		{
			name:            "primary ref denied in all mode: association pending",
			accessReviewer:  denyAgentReviewer{},
			rbacMode:        operator.RBACOnRefsModeAll,
			runtimeObjs:     []client.Object{agentObj.DeepCopy(), fleetServer, es, fleetESCA, fleetHTTPCerts, fleetService},
			wantStatus:      commonv1.AssociationPending,
			wantCAInAgentNs: false,
		},
		{
			name:            "primary ref denied in true mode: association established with warning",
			accessReviewer:  denyAgentReviewer{},
			rbacMode:        operator.RBACOnRefsModeTrue,
			runtimeObjs:     []client.Object{agentObj.DeepCopy(), fleetServer, es, fleetESCA, fleetHTTPCerts, fleetService},
			wantStatus:      commonv1.AssociationEstablished,
			wantCAInAgentNs: true,
		},
		{
			name:            "primary ref denied in legacy mode: association established with warning",
			accessReviewer:  denyAgentReviewer{},
			rbacMode:        operator.RBACOnRefsModeLegacy,
			runtimeObjs:     []client.Object{agentObj.DeepCopy(), fleetServer, es, fleetESCA, fleetHTTPCerts, fleetService},
			wantStatus:      commonv1.AssociationEstablished,
			wantCAInAgentNs: true,
		},
		{
			name:            "kind-guard: Fleet Server and ES share namespace/name, Fleet Server denied in all mode: association pending",
			accessReviewer:  denyAgentReviewer{},
			rbacMode:        operator.RBACOnRefsModeAll,
			runtimeObjs:     []client.Object{agentObj.DeepCopy(), fleetServerSameNameAsES, esSameName, fleetHTTPCerts, fleetService},
			wantStatus:      commonv1.AssociationPending,
			wantCAInAgentNs: false,
		},
		{
			name:            "kind-guard: Fleet Server and ES share namespace/name, Fleet Server denied in true mode: association established with warning",
			accessReviewer:  denyAgentReviewer{},
			rbacMode:        operator.RBACOnRefsModeTrue,
			runtimeObjs:     []client.Object{agentObj.DeepCopy(), fleetServerSameNameAsES, esSameName, fleetHTTPCerts, fleetService},
			wantStatus:      commonv1.AssociationEstablished,
			wantCAInAgentNs: false,
		},
		{
			name:            "kind-guard: Fleet Server and ES share namespace/name, Fleet Server denied in legacy mode: association established with warning",
			accessReviewer:  denyAgentReviewer{},
			rbacMode:        operator.RBACOnRefsModeLegacy,
			runtimeObjs:     []client.Object{agentObj.DeepCopy(), fleetServerSameNameAsES, esSameName, fleetHTTPCerts, fleetService},
			wantStatus:      commonv1.AssociationEstablished,
			wantCAInAgentNs: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := association.NewTestAssociationReconcilerWithReviewer(
				agentFleetServerAssociationInfo(),
				tt.accessReviewer,
				tt.rbacMode,
				tt.runtimeObjs...,
			)

			_, err := r.Reconcile(t.Context(), reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(agentObj)})
			require.NoError(t, err)

			var updatedAgent agentv1alpha1.Agent
			require.NoError(t, r.Get(t.Context(), k8s.ExtractNamespacedName(agentObj), &updatedAgent))
			require.Equal(t, tt.wantStatus, updatedAgent.Status.FleetServerAssociationStatus)

			if tt.wantConfCleared {
				_, hasConf := updatedAgent.Annotations[commonv1.FleetServerConfigAnnotationNameBase]
				require.False(t, hasConf, "FleetServer association conf annotation must be removed when transitive ES RBAC is revoked")
			}

			var copiedSecrets corev1.SecretList
			require.NoError(t, r.List(t.Context(), &copiedSecrets,
				client.InNamespace("agent-ns"),
				client.MatchingLabels{association.AdditionalSecretLabelName: "true"},
			))
			if tt.wantCAInAgentNs {
				require.NotEmpty(t, copiedSecrets.Items, "transitive ES CA must be present in agent namespace when RBAC is allowed")
				require.Contains(t, copiedSecrets.Items[0].Data, "ca.crt")
			} else {
				require.Empty(t, copiedSecrets.Items, "transitive ES CA must not be copied when transitive ES RBAC is denied")
			}
		})
	}
}

func TestAgentFleetServerReconcileThenUnbind(t *testing.T) {
	agentObj := &agentv1alpha1.Agent{
		Name: "agent1", Namespace: "agent-ns",
		Spec: agentv1alpha1.AgentSpec{
			Version:        "8.0.0",
			FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fleet-ns"}},
		},
	}
	fleetServer := &agentv1alpha1.Agent{
		Name:      "fleet1",
		Namespace: "fleet-ns",
		Annotations: map[string]string{
			esConfAnnotationKey(commonv1.ObjectSelector{Name: "es1", Namespace: "es-ns"}): esAssocConfAnnotation(commonv1.AssociationConf{
				AuthSecretName: "-",
				CACertProvided: true,
				CASecretName:   "fleet1-es-ca",
				URL:            "https://es1-http.es-ns.svc:9200",
			}),
		},
		Spec: agentv1alpha1.AgentSpec{
			Version:            "8.0.0",
			FleetServerEnabled: true,
			ElasticsearchRefs: []agentv1alpha1.Output{
				{Name: "es1", Namespace: "es-ns"},
			},
		},
	}
	fleetServer.GetAssociations()[0].SetAssociationConf(&commonv1.AssociationConf{
		AuthSecretName: "-",
		CACertProvided: true,
		CASecretName:   "fleet1-es-ca",
		URL:            "https://es1-http.es-ns.svc:9200",
	})
	es := &esv1.Elasticsearch{
		Name: "es1", Namespace: "es-ns",
		Spec: esv1.ElasticsearchSpec{Version: "8.0.0"},
	}
	fleetESCA := &corev1.Secret{
		Namespace: "fleet-ns", Name: "fleet1-es-ca",
		Data: map[string][]byte{"ca.crt": []byte("cacert"), "tls.crt": []byte("tlscert")},
	}
	fleetHTTPCerts := &corev1.Secret{
		Namespace: "fleet-ns", Name: "fleet1-agent-http-certs-public",
		Data: map[string][]byte{"ca.crt": []byte("cacert"), "tls.crt": []byte("tlscert")},
	}
	fleetService := &corev1.Service{
		Namespace: "fleet-ns", Name: "fleet1-agent-http",
		Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "https", Port: 8220}}},
	}

	r := association.NewTestAssociationReconcilerWithReviewer(
		agentFleetServerAssociationInfo(),
		rbac.NewPermissiveAccessReviewer(),
		operator.RBACOnRefsModeOff,
		agentObj, fleetServer, es, fleetESCA, fleetHTTPCerts, fleetService,
	)

	// Run reconcile — establishes the association and creates secrets in agent-ns.
	_, err := r.Reconcile(t.Context(), reconcile.Request{NamespacedName: k8s.ExtractNamespacedName(agentObj)})
	require.NoError(t, err)

	var updatedAgent agentv1alpha1.Agent
	require.NoError(t, r.Get(t.Context(), k8s.ExtractNamespacedName(agentObj), &updatedAgent))
	require.Equal(t, commonv1.AssociationEstablished, updatedAgent.Status.FleetServerAssociationStatus)

	// Verify at least the Fleet Server CA copy and the transitive ES CA copy are present.
	var secretsBefore corev1.SecretList
	require.NoError(t, r.List(t.Context(), &secretsBefore, client.InNamespace("agent-ns")))
	require.NotEmpty(t, secretsBefore.Items, "reconcile must create association secrets in agent-ns")

	// Unbind must delete every association secret in agent-ns.
	require.NoError(t, r.Unbind(t.Context(), updatedAgent.GetAssociations()[0]))

	var secretsAfter corev1.SecretList
	require.NoError(t, r.List(t.Context(), &secretsAfter, client.InNamespace("agent-ns")))
	require.Empty(t, secretsAfter.Items, "Unbind must delete all association secrets from agent-ns")
}
