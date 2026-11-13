// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.
package v1alpha1

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
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
				ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Namespace: "namespace1", Name: "elasticsearch1"}},
				ClusterName:           "test",
			},
			want: "association.k8s.elastic.co/es-conf-2150608354",
		},
		{
			name: "max length namespace and name (63 and 36 respectively)",
			cluster: ElasticsearchCluster{
				ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{
					Namespace: "longnamespacelongnamespacelongnamespacelongnamespacelongnamespa",
					Name:      "elasticsearch1elasticsearch1elastics",
				}},
				ClusterName: "test",
			},
			want: "association.k8s.elastic.co/es-conf-3419573237",
		},
		{
			name: "secret name gives a different hash",
			cluster: ElasticsearchCluster{
				ElasticsearchSelector: commonv1.ElasticsearchSelector{ObjectSelector: commonv1.ObjectSelector{Namespace: "namespace1", SecretName: "elasticsearch1"}},
				ClusterName:           "test",
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
				Name:      "elasticsearch1elasticsearch1elastics",
			},
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

func TestLogstash_CertificateContributingServices(t *testing.T) {
	selfSigned := func(disabled bool, sans ...commonv1.SubjectAlternativeName) commonv1.TLSOptions {
		return commonv1.TLSOptions{
			SelfSignedCertificate: &commonv1.SelfSignedCertificate{Disabled: disabled, SubjectAlternativeNames: sans},
		}
	}
	for _, tt := range []struct {
		name         string
		services     []LogstashService
		wantServices []string
		wantSANs     []commonv1.SubjectAlternativeName
	}{
		{
			name: "no service",
		},
		{
			name: "service without TLS options does not contribute",
			services: []LogstashService{
				{Name: "beats"},
			},
		},
		{
			name: "service with a custom certificate only does not contribute",
			services: []LogstashService{
				{Name: "beats", TLS: commonv1.TLSOptions{Certificate: commonv1.SecretRef{SecretName: "my-cert"}}},
			},
		},
		{
			name: "service with a disabled self-signed certificate does not contribute",
			services: []LogstashService{
				{Name: "beats", TLS: selfSigned(true, commonv1.SubjectAlternativeName{DNS: "beats.example.com"})},
			},
		},
		{
			name: "API service does not contribute",
			services: []LogstashService{
				{Name: "api", TLS: selfSigned(false, commonv1.SubjectAlternativeName{DNS: "api.example.com"})},
			},
		},
		{
			name: "service with an empty self-signed certificate contributes without SAN",
			services: []LogstashService{
				{Name: "beats", TLS: selfSigned(false)},
			},
			wantServices: []string{"beats"},
		},
		{
			name: "SANs are returned in the order of the spec and without duplicates",
			services: []LogstashService{
				{Name: "tcp", TLS: selfSigned(false,
					commonv1.SubjectAlternativeName{DNS: "tcp.example.com"},
					commonv1.SubjectAlternativeName{IP: "1.2.3.4"},
				)},
				{Name: "api", TLS: selfSigned(false, commonv1.SubjectAlternativeName{DNS: "api.example.com"})},
				{Name: "ignored", TLS: selfSigned(true, commonv1.SubjectAlternativeName{DNS: "ignored.example.com"})},
				{Name: "no-tls"},
				{Name: "beats", TLS: selfSigned(false,
					commonv1.SubjectAlternativeName{DNS: "beats.example.com"},
					commonv1.SubjectAlternativeName{DNS: "tcp.example.com"},
					commonv1.SubjectAlternativeName{IP: "1.2.3.4"},
					commonv1.SubjectAlternativeName{DNS: "beats.example.com"},
				)},
			},
			wantServices: []string{"tcp", "beats"},
			wantSANs: []commonv1.SubjectAlternativeName{
				{DNS: "tcp.example.com"},
				{IP: "1.2.3.4"},
				{DNS: "beats.example.com"},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			logstash := Logstash{
				Spec: LogstashSpec{Services: tt.services},
				Name: "ls",
			}

			svc := logstash.CertificateContributingServices()
			gotServices := make([]string, 0, len(svc))
			for _, service := range svc {
				gotServices = append(gotServices, service.Name)
			}
			if tt.wantServices == nil {
				tt.wantServices = []string{}
			}
			require.Equal(t, tt.wantServices, gotServices)
			require.Equal(t, tt.wantSANs, logstash.ExtraCertificateSANs())
		})
	}
}

func TestLogstash_IsAPIService(t *testing.T) {
	logstash := Logstash{
		Name: "ls",
	}
	require.True(t, logstash.IsAPIService(LogstashService{Name: "api"}))
	require.False(t, logstash.IsAPIService(LogstashService{Name: "beats"}))
	require.False(t, logstash.IsAPIService(LogstashService{}))
}
