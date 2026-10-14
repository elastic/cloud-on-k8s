// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package rolemapping

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestRetrieveRequests(t *testing.T) {
	es := esv1.Elasticsearch{Namespace: "es-ns", Name: "es"}
	request := &corev1.Secret{
		Namespace: es.Namespace,
		Name:      "kb-pki-role-mapping",
		Labels:    RequestLabels(es),
		Data: map[string][]byte{
			PublicKeyFingerprintField: []byte("fingerprint"),
			RolesField:                []byte("kibana_system"),
			ResourceUIDField:          []byte("uid-abc-123"),
			AssociationNameField:      []byte("kb-es"),
			ResourceNamespaceField:    []byte("kb-ns"),
			ResourceNameField:         []byte("kb"),
		},
	}
	otherESRequest := request.DeepCopy()
	otherESRequest.Name = "other"
	otherESRequest.Labels = map[string]string{
		label.ClusterNameLabelName: "other-es",
		commonv1.TypeLabelName:     PKIRoleMappingRequestType,
	}

	requests, err := RetrieveRequests(t.Context(), k8s.NewFakeClient(request, otherESRequest), es)
	require.NoError(t, err)
	require.Equal(t, []Request{{
		SecretNamespace:      es.Namespace,
		SecretName:           request.Name,
		PublicKeyFingerprint: "fingerprint",
		Roles:                "kibana_system",
		ResourceUID:          "uid-abc-123",
		AssociationName:      "kb-es",
		ResourceNamespace:    "kb-ns",
		ResourceName:         "kb",
	}}, requests)
}

func TestParseRequestSecret(t *testing.T) {
	allFields := map[string][]byte{
		RolesField:                []byte("kibana_system"),
		PublicKeyFingerprintField: []byte("fp"),
		ResourceUIDField:          []byte("uid"),
		AssociationNameField:      []byte("kb-es"),
		// optional fields: their absence must not make the request invalid
		ResourceNamespaceField: []byte("kb-ns"),
		ResourceNameField:      []byte("kb"),
	}
	tests := []struct {
		name          string
		omit          []string
		wantErrFields []string
	}{
		{name: "all fields present — no error"},
		{name: "missing optional resource namespace and name — no error", omit: []string{ResourceNamespaceField, ResourceNameField}},
		{name: "missing roles", omit: []string{RolesField}, wantErrFields: []string{RolesField}},
		{name: "missing fingerprint", omit: []string{PublicKeyFingerprintField}, wantErrFields: []string{PublicKeyFingerprintField}},
		{name: "missing resourceUID", omit: []string{ResourceUIDField}, wantErrFields: []string{ResourceUIDField}},
		{name: "missing associationName", omit: []string{AssociationNameField}, wantErrFields: []string{AssociationNameField}},
		{
			name:          "all fields missing — all field names reported",
			omit:          []string{RolesField, PublicKeyFingerprintField, ResourceUIDField, AssociationNameField},
			wantErrFields: []string{RolesField, PublicKeyFingerprintField, ResourceUIDField, AssociationNameField},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make(map[string][]byte, len(allFields))
			maps.Copy(data, allFields)
			for _, k := range tt.omit {
				delete(data, k)
			}
			_, err := parseRequestSecret(corev1.Secret{Data: data})
			require.Equal(t, len(tt.wantErrFields) > 0, err != nil)
			for _, field := range tt.wantErrFields {
				require.ErrorContains(t, err, field)
			}
		})
	}
}

func TestEvaluation(t *testing.T) {
	tests := []struct {
		name        string
		annotation  string
		fingerprint string
		roles       string
		wantParsed  bool
		wantMatches bool
	}{
		{
			name:        "missing annotation",
			fingerprint: "fp",
			roles:       "kibana_system",
		},
		{
			name:        "invalid annotation",
			annotation:  "not-json",
			fingerprint: "fp",
			roles:       "kibana_system",
		},
		{
			name:        "evaluation of the request",
			annotation:  `{"fingerprint":"fp","roles":"kibana_system"}`,
			fingerprint: "fp",
			roles:       " kibana_system ",
			wantParsed:  true,
			wantMatches: true,
		},
		{
			name:        "evaluation of a previous fingerprint",
			annotation:  `{"fingerprint":"previous-fp","roles":"kibana_system"}`,
			fingerprint: "fp",
			roles:       "kibana_system",
			wantParsed:  true,
		},
		{
			name:        "evaluation of previous roles",
			annotation:  `{"fingerprint":"fp","roles":"superuser","conflict":"withheld"}`,
			fingerprint: "fp",
			roles:       "kibana_system",
			wantParsed:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluation, parsed := ParseEvaluation(tt.annotation)
			require.Equal(t, tt.wantParsed, parsed)
			require.Equal(t, tt.wantMatches, evaluation.Matches(tt.fingerprint, tt.roles))
		})
	}
}
