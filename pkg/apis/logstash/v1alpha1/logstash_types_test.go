// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.
package v1alpha1

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
)

func TestLogstashEsAssociation_AssociationConfAnnotationName(t *testing.T) {
	for _, tt := range []struct {
		name    string
		cluster ElasticsearchCluster
		want    string
	}{
		{
			name: "average length names",
			cluster: ElasticsearchCluster{
				ObjectSelector: commonv1.ObjectSelector{Namespace: "namespace1", Name: "elasticsearch1"},
				ClusterName:    "test",
			},
			want: "association.k8s.elastic.co/es-conf-2150608354",
		},
		{
			name: "max length namespace and name (63 and 36 respectively)",
			cluster: ElasticsearchCluster{
				ObjectSelector: commonv1.ObjectSelector{
					Namespace: "longnamespacelongnamespacelongnamespacelongnamespacelongnamespa",
					Name:      "elasticsearch1elasticsearch1elastics"},
				ClusterName: "test",
			},
			want: "association.k8s.elastic.co/es-conf-3419573237",
		},
		{
			name: "secret name gives a different hash",
			cluster: ElasticsearchCluster{
				ObjectSelector: commonv1.ObjectSelector{Namespace: "namespace1", SecretName: "elasticsearch1"},
				ClusterName:    "test",
			},
			want: "association.k8s.elastic.co/es-conf-851285294",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			aea := LogstashESAssociation{ElasticsearchCluster: tt.cluster}
			got := aea.AssociationConfAnnotationName()

			require.Equal(t, tt.want, got)
			tokens := strings.Split(got, "/")
			require.Equal(t, 2, len(tokens))
			require.LessOrEqual(t, len(tokens[0]), 253)
			require.LessOrEqual(t, len(tokens[1]), 63)
		})
	}
}

func TestLogstashMonitoringAssociation_AssociationConfAnnotationName(t *testing.T) {
	for _, tt := range []struct {
		name string
		ref  commonv1.ObjectSelector
		want string
	}{
		{
			name: "average length names",
			ref:  commonv1.ObjectSelector{Namespace: "namespace1", Name: "elasticsearch1"},
			want: "association.k8s.elastic.co/es-conf-2150608354-sm",
		},
		{
			name: "max length namespace and name (63 and 36 respectively)",
			ref: commonv1.ObjectSelector{
				Namespace: "longnamespacelongnamespacelongnamespacelongnamespacelongnamespa",
				Name:      "elasticsearch1elasticsearch1elastics"},
			want: "association.k8s.elastic.co/es-conf-3419573237-sm",
		},
		{
			name: "secret name gives a different hash",
			ref:  commonv1.ObjectSelector{Namespace: "namespace1", SecretName: "elasticsearch1"},
			want: "association.k8s.elastic.co/es-conf-851285294-sm",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			aea := LogstashMonitoringAssociation{ref: tt.ref}
			got := aea.AssociationConfAnnotationName()

			require.Equal(t, tt.want, got)
			tokens := strings.Split(got, "/")
			require.Equal(t, 2, len(tokens))
			require.LessOrEqual(t, len(tokens[0]), 253)
			require.LessOrEqual(t, len(tokens[1]), 63)
		})
	}
}

func TestLogstash_APIServerTLSOptions(t *testing.T) {
	for _, tt := range []struct {
		name     string
		logstash Logstash
		want     bool
	}{
		{
			name: "default no service config enable TLS",
			logstash: Logstash{
				Spec: LogstashSpec{},
			},
			want: true,
		},
		{
			name: "api service disable TLS",
			logstash: Logstash{
				Spec: LogstashSpec{
					Services: []LogstashService{{
						Name: "api",
						TLS: commonv1.TLSOptions{
							SelfSignedCertificate: &commonv1.SelfSignedCertificate{
								Disabled: true,
							},
						},
					}},
				},
			},
			want: false,
		},
		{
			name: "take api service from services",
			logstash: Logstash{
				Spec: LogstashSpec{
					Services: []LogstashService{
						{
							Name: "strong_svc",
							TLS: commonv1.TLSOptions{
								SelfSignedCertificate: &commonv1.SelfSignedCertificate{
									Disabled: false,
								},
							},
						},
						{
							Name: "api",
							TLS: commonv1.TLSOptions{
								SelfSignedCertificate: &commonv1.SelfSignedCertificate{
									Disabled: true,
								},
							},
						},
					},
				},
			},
			want: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.logstash.APIServerTLSOptions().Enabled())
		})
	}
}
