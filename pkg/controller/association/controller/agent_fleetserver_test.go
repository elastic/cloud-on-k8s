// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package controller

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/agent"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/association"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/annotation"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/hash"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/labels"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
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
		ObjectMeta: metav1.ObjectMeta{Name: "my-agent", Namespace: "ns"},
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
				ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns"},
				Spec:       agentv1alpha1.AgentSpec{Version: "8.0.0"},
			},
			wantSecrets: nil,
		},
		{
			name: "fleet server has no ES refs returns nil",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "fleet1", Namespace: "fs-ns"},
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
				ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version: "8.0.0",
					// Namespace intentionally omitted — ECK treats this as same namespace.
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fleet1", Namespace: "ns",
					Annotations: map[string]string{
						esConfAnnotationKey(esSelector): esAssocConfAnnotation(commonv1.AssociationConf{
							AuthSecretName: "-",
							CACertProvided: true,
							CASecretName:   "fleet1-es-ca",
							URL:            "https://es1-http.es-ns.svc:9200",
						}),
					},
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: esSelector}},
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
				ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fleet1", Namespace: "ns",
					Annotations: map[string]string{
						esConfAnnotationKey(esSelector): esAssocConfAnnotation(commonv1.AssociationConf{
							AuthSecretName: "-",
							CACertProvided: true,
							CASecretName:   "fleet1-es-ca",
							URL:            "https://es1-http.es-ns.svc:9200",
						}),
					},
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: esSelector}},
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
				ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fleet1", Namespace: "fs-ns",
					Annotations: map[string]string{
						esConfAnnotationKey(esSelector): esAssocConfAnnotation(commonv1.AssociationConf{
							AuthSecretName: "-",
							CACertProvided: true,
							CASecretName:   "fleet1-es-ca",
							URL:            "https://es1-http.es-ns.svc:9200",
						}),
					},
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: esSelector}},
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
				ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
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
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{ElasticsearchSelector: commonv1.ElasticsearchSelector{
							ObjectSelector:              esSelector,
							ClientCertificateSecretName: "user-provided-cert",
						}},
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
				ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fleet1", Namespace: "fs-ns",
					Annotations: map[string]string{
						esConfAnnotationKey(esSelector): esAssocConfAnnotation(commonv1.AssociationConf{
							AuthSecretName: "-",
							CACertProvided: false,
							CASecretName:   "fleet1-es-ca",
							URL:            "https://es1-http.es-ns.svc:9200",
						}),
					},
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: esSelector}},
					},
				},
			},
			wantSecrets: []association.AdditionalSecret{},
		},
		{
			name: "client cert removed from ESRef but still in conf: only CA returned, GC removes stale client cert copy",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
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
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						// ClientCertificateSecretName deliberately absent — user removed the ref
						{ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: esSelector}},
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
				ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
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
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: esSelector}},
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
		ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns"},
	}

	matchingLabels := map[string]string{
		"agentassociation.k8s.elastic.co/name":      "agent1",
		"agentassociation.k8s.elastic.co/namespace": "ns",
		"agentassociation.k8s.elastic.co/type":      commonv1.FleetServerAssociationType,
		labels.ClientCertificateLabelName:           "true",
		reconciler.SoftOwnerKindLabel:               esv1.Kind,
	}

	for _, tt := range []struct {
		name              string
		existingObjects   []client.Object
		currentSecretName string
		wantRemaining     []string
	}{
		{
			name:          "no matching secrets is a no-op",
			wantRemaining: nil,
		},
		{
			name: "deletes all matching secrets when currentSecretName is empty",
			existingObjects: []client.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "old-cert-1", Namespace: "ns", Labels: matchingLabels}},
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "old-cert-2", Namespace: "ns", Labels: matchingLabels}},
			},
			currentSecretName: "",
			wantRemaining:     nil,
		},
		{
			name: "preserves current secret and deletes others",
			existingObjects: []client.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "current-cert", Namespace: "ns", Labels: matchingLabels}},
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "orphaned-cert", Namespace: "ns", Labels: matchingLabels}},
			},
			currentSecretName: "current-cert",
			wantRemaining:     []string{"current-cert"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := k8s.NewFakeClient(tt.existingObjects...)

			err := deleteOrphanedTransitiveESClientCertSecrets(context.Background(), c, assocMeta, associated, tt.currentSecretName)
			require.NoError(t, err)

			var secretList corev1.SecretList
			require.NoError(t, c.List(context.Background(), &secretList))
			var remaining []string //nolint:prealloc
			for _, s := range secretList.Items {
				remaining = append(remaining, s.Name)
			}
			require.Equal(t, tt.wantRemaining, remaining)
		})
	}
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
		reconciler.SoftOwnerKindLabel:               esv1.Kind,
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
	}{
		{
			name: "fleet server ref not set returns nil and cleans up",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns"},
				Spec:       agentv1alpha1.AgentSpec{Version: "8.0.0"},
			},
			orphanedSecrets: []*corev1.Secret{
				{ObjectMeta: metav1.ObjectMeta{Name: "old-cert", Namespace: "ns", Labels: orphanLabels}},
			},
			wantRef:        nil,
			wantNilResults: true,
			wantOrphanGone: true,
		},
		{
			name: "fleet server has no ES refs returns nil and cleans up",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "fleet1", Namespace: "fs-ns"},
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
				ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fleet1", Namespace: "fs-ns",
					Annotations: map[string]string{
						esConfAnnotationKey(esSelector): esAssocConfAnnotation(commonv1.AssociationConf{
							AuthSecretName: "-",
							CACertProvided: true,
							CASecretName:   "fleet1-es-ca",
							URL:            "https://es1-http.es-ns.svc:9200",
						}),
					},
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: esSelector}},
					},
				},
			},
			es: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{Name: "es1", Namespace: "es-ns"},
			},
			wantRef: &commonv1.TransitiveESRef{
				CASecretName: association.AdditionalSecretNamer.Suffix("agent1", "agent-fleetserver", hash.HashObject(types.NamespacedName{Name: "fleet1", Namespace: "fs-ns"}), "es-ca"),
			},
			wantNilResults: true,
		},
		{
			name: "ES has client auth and fleet server has user-provided cert",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
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
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{ElasticsearchSelector: commonv1.ElasticsearchSelector{
							ObjectSelector:              esSelector,
							ClientCertificateSecretName: "user-provided-cert",
						}},
					},
				},
			},
			es: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name: "es1", Namespace: "es-ns",
					Annotations: map[string]string{
						annotation.ClientAuthenticationRequiredAnnotation: "true",
					},
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
				ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fleet1", Namespace: "fs-ns",
					Annotations: map[string]string{
						esConfAnnotationKey(esSelector): esAssocConfAnnotation(commonv1.AssociationConf{
							AuthSecretName: "-",
							CACertProvided: true,
							CASecretName:   "fleet1-es-ca",
							URL:            "https://es1-http.es-ns.svc:9200",
						}),
					},
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: esSelector}},
					},
				},
			},
			es: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name: "es1", Namespace: "es-ns",
					Annotations: map[string]string{
						annotation.ClientAuthenticationRequiredAnnotation: "true",
					},
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
		},
		{
			name: "external ES ref skips client cert reconciliation and cleans up",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fleet1", Namespace: "fs-ns",
					Annotations: map[string]string{
						esConfAnnotationKey(commonv1.ObjectSelector{SecretName: "external-es-secret"}): esAssocConfAnnotation(commonv1.AssociationConf{
							AuthSecretName: "external-es-secret",
							URL:            "https://external-es:9200",
						}),
					},
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{SecretName: "external-es-secret"}}},
					},
				},
			},
			orphanedSecrets: []*corev1.Secret{
				{ObjectMeta: metav1.ObjectMeta{Name: "old-cert", Namespace: "ns", Labels: orphanLabels}},
			},
			wantRef:        nil,
			wantNilResults: true,
			wantOrphanGone: true,
		},
		{
			name: "same-namespace fleet server with client auth sets CA secret name to original",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "fs-ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fleet1", Namespace: "fs-ns",
					Annotations: map[string]string{
						esConfAnnotationKey(esSelector): esAssocConfAnnotation(commonv1.AssociationConf{
							AuthSecretName: "-",
							CACertProvided: true,
							CASecretName:   "fleet1-es-ca",
							URL:            "https://es1-http.es-ns.svc:9200",
						}),
					},
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: esSelector}},
					},
				},
			},
			es: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name: "es1", Namespace: "es-ns",
					Annotations: map[string]string{
						annotation.ClientAuthenticationRequiredAnnotation: "true",
					},
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
		},
		{
			name: "same-namespace fleet server with user-provided cert returns original client cert name",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "fs-ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
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
				},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{ElasticsearchSelector: commonv1.ElasticsearchSelector{
							ObjectSelector:              esSelector,
							ClientCertificateSecretName: "user-provided-cert",
						}},
					},
				},
			},
			es: &esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name: "es1", Namespace: "es-ns",
					Annotations: map[string]string{
						annotation.ClientAuthenticationRequiredAnnotation: "true",
					},
				},
			},
			wantRef: &commonv1.TransitiveESRef{
				CASecretName:         "fleet1-es-ca",
				ClientCertSecretName: "user-provided-cert",
			},
		},
		{
			name: "conf is nil returns nil and cleans up",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:        "8.0.0",
					FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
				},
			},
			fleetServer: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "fleet1", Namespace: "fs-ns"},
				Spec: agentv1alpha1.AgentSpec{
					Version:            "8.0.0",
					FleetServerEnabled: true,
					ElasticsearchRefs: []agentv1alpha1.Output{
						{ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: esSelector}},
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
			c := k8s.NewFakeClient(objects...)
			assoc := &agentv1alpha1.AgentFleetServerAssociation{Agent: tt.agent}

			ref, results := fleetManagedAgentTransitiveESRef(context.Background(), c, assoc, assocMeta)

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

			if tt.wantCreatedSecret != nil {
				// Verify the secret was actually persisted in the fake client, not just named.
				var secret corev1.Secret
				err := c.Get(context.Background(), *tt.wantCreatedSecret, &secret)
				require.NoError(t, err, "expected secret %s/%s to exist in the fake client", tt.wantCreatedSecret.Namespace, tt.wantCreatedSecret.Name)
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
		ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "ns"},
		Spec: agentv1alpha1.AgentSpec{
			Version:        "8.0.0",
			FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
		},
	}
	fleetServer := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
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
		},
		Spec: agentv1alpha1.AgentSpec{
			Version:            "8.0.0",
			FleetServerEnabled: true,
			ElasticsearchRefs: []agentv1alpha1.Output{
				{ElasticsearchSelector: commonv1.ElasticsearchSelector{
					ObjectSelector:              esSelector,
					ClientCertificateSecretName: "user-provided-cert",
				}},
			},
		},
	}
	es := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name: "es1", Namespace: "es-ns",
			Annotations: map[string]string{
				annotation.ClientAuthenticationRequiredAnnotation: "true",
			},
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
		ObjectMeta: metav1.ObjectMeta{Name: "fleet1", Namespace: "fs-ns"},
		Spec: agentv1alpha1.AgentSpec{
			FleetServerEnabled: true,
			ElasticsearchRefs: []agentv1alpha1.Output{
				{ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: esSelector}},
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
		ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "agent-ns"},
		Spec: agentv1alpha1.AgentSpec{
			FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fs-ns"}},
		},
	}

	agentNoRef := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "agent-ns"},
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
					agentv1alpha1.Output{ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "es2", Namespace: "es-ns"}}},
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

func TestAgentFleetServerTransitiveESRBAC(t *testing.T) {
	agentObj := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "agent1", Namespace: "agent-ns"},
		Spec: agentv1alpha1.AgentSpec{
			Version:        "8.0.0",
			FleetServerRef: commonv1.FleetServerSelector{ObjectSelector: commonv1.ObjectSelector{Name: "fleet1", Namespace: "fleet-ns"}},
		},
	}

	fleetServer := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
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
		},
		Spec: agentv1alpha1.AgentSpec{
			Version:            "8.0.0",
			FleetServerEnabled: true,
			ElasticsearchRefs: []agentv1alpha1.Output{
				{ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Name: "es1", Namespace: "es-ns"}}},
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
		ObjectMeta: metav1.ObjectMeta{Name: "es1", Namespace: "es-ns"},
		Spec:       esv1.ElasticsearchSpec{Version: "8.0.0"},
	}

	// CA secret in the Fleet Server namespace — would be copied to agent-ns on success.
	fleetESCA := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "fleet-ns", Name: "fleet1-es-ca"},
		Data:       map[string][]byte{"ca.crt": []byte("cacert"), "tls.crt": []byte("tlscert")},
	}
	fleetHTTPCerts := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "fleet-ns", Name: "fleet1-agent-http-certs-public"},
		Data:       map[string][]byte{"ca.crt": []byte("cacert"), "tls.crt": []byte("tlscert")},
	}
	fleetService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "fleet-ns", Name: "fleet1-agent-http"},
		Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "https", Port: 8220}}},
	}

	// Fleet Server with no ElasticsearchRefs — manual/external setup, no transitive ES.
	fleetServerNoES := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet1", Namespace: "fleet-ns"},
		Spec: agentv1alpha1.AgentSpec{
			Version:            "8.0.0",
			FleetServerEnabled: true,
		},
	}

	// Fleet Server whose Elasticsearch is unmanaged (external secret). The RBAC check is skipped
	// for external associations, so the agent association should establish regardless of the reviewer.
	fleetServerWithExternalES := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet1", Namespace: "fleet-ns"},
		Spec: agentv1alpha1.AgentSpec{
			Version:            "8.0.0",
			FleetServerEnabled: true,
			ElasticsearchRefs: []agentv1alpha1.Output{
				{ElasticsearchSelector: commonv1.ElasticsearchSelector{
					ObjectSelector: commonv1.ObjectSelector{SecretName: "external-es-secret"},
				}},
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
		ObjectMeta: metav1.ObjectMeta{
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
		},
		Data: map[string][]byte{"ca.crt": []byte("cacert")},
	}

	for _, tt := range []struct {
		name            string
		accessReviewer  rbac.AccessReviewer
		runtimeObjs     []client.Object
		wantStatus      commonv1.AssociationStatus
		wantCAInAgentNs bool
		wantConfCleared bool
	}{
		{
			name:            "transitive ES RBAC allowed: association established and CA copied",
			accessReviewer:  rbac.NewPermissiveAccessReviewer(),
			runtimeObjs:     []client.Object{agentObj.DeepCopy(), fleetServer, es, fleetESCA, fleetHTTPCerts, fleetService},
			wantStatus:      commonv1.AssociationEstablished,
			wantCAInAgentNs: true,
		},
		{
			name:            "transitive ES RBAC denied: association pending and CA not copied",
			accessReviewer:  denyESReviewer{},
			runtimeObjs:     []client.Object{agentObj.DeepCopy(), fleetServer, es, fleetESCA, fleetHTTPCerts, fleetService},
			wantStatus:      commonv1.AssociationPending,
			wantCAInAgentNs: false,
		},
		{
			name:            "fleet server has no ES ref (manual setup): association establishes",
			accessReviewer:  rbac.NewPermissiveAccessReviewer(),
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
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := association.NewTestAssociationReconcilerWithReviewer(
				agentFleetServerAssociationInfo(),
				tt.accessReviewer,
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
