// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"k8s.io/apimachinery/pkg/util/validation/field"

	lsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/logstash/v1alpha1"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
)

func TestCheckNameLength(t *testing.T) {
	testCases := []struct {
		name         string
		logstashName string
		wantErr      bool
		wantErrMsg   string
	}{
		{
			name:         "valid configuration",
			logstashName: "test-logstash",
			wantErr:      false,
		},
		{
			name:         "long Logstash name",
			logstashName: "extremely-long-winded-and-unnecessary-name-for-logstash",
			wantErr:      true,
			wantErrMsg:   "name exceeds maximum allowed length",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ls := lsv1alpha1.Logstash{
				Name:      tc.logstashName,
				Namespace: "test",
				Spec:      lsv1alpha1.LogstashSpec{},
			}

			errList := checkNameLength(&ls)
			assert.Equal(t, tc.wantErr, len(errList) > 0)
		})
	}
}

func TestCheckNoUnknownFields(t *testing.T) {
	type args struct {
		prev *lsv1alpha1.Logstash
		curr *lsv1alpha1.Logstash
	}
	tests := []struct {
		name string
		args args
		want field.ErrorList
	}{
		{
			name: "No downgrade",
			args: args{
				prev: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{Version: "7.17.0"}},
				curr: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{Version: "8.6.1"}},
			},
			want: nil,
		},
		{
			name: "Downgrade NOK",
			args: args{
				prev: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{Version: "8.6.1"}},
				curr: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{Version: "8.5.0"}},
			},
			want: field.ErrorList{&field.Error{Type: field.ErrorTypeForbidden, Field: "spec.version", BadValue: "", Detail: "Version downgrades are not supported"}},
		},
		{
			name: "Downgrade with override OK",
			args: args{
				prev: &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{Version: "8.6.1"}},
				curr: &lsv1alpha1.Logstash{Annotations: map[string]string{
					commonv1.DisableDowngradeValidationAnnotation: "true",
				}, Spec: lsv1alpha1.LogstashSpec{Version: "8.5.0"}},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, checkNoDowngrade(tt.args.prev, tt.args.curr), "checkNoDowngrade(%v, %v)", tt.args.prev, tt.args.curr)
		})
	}
}

func Test_checkSingleConfigSource(t *testing.T) {
	tests := []struct {
		name     string
		logstash lsv1alpha1.Logstash
		wantErr  bool
	}{
		{
			name: "configRef absent, config present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					Config: &commonv1.Config{},
				},
			},
			wantErr: false,
		},
		{
			name: "config absent, configRef present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					ConfigRef: &commonv1.ConfigSource{},
				},
			},
			wantErr: false,
		},
		{
			name: "neither present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{},
			},
			wantErr: false,
		},
		{
			name: "both present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					Config:    &commonv1.Config{},
					ConfigRef: &commonv1.ConfigSource{},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := checkSingleConfigSource(&tc.logstash)
			assert.Equal(t, tc.wantErr, len(got) > 0)
		})
	}
}

func Test_checkSinglePipelineSource(t *testing.T) {
	tests := []struct {
		name     string
		logstash lsv1alpha1.Logstash
		wantErr  bool
	}{
		{
			name: "pipelinesRef absent, pipelines present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					Pipelines: []commonv1.Config{},
				},
			},
			wantErr: false,
		},
		{
			name: "pipelines absent, pipelinesRef present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					PipelinesRef: &commonv1.ConfigMapOrSecretSource{},
				},
			},
			wantErr: false,
		},
		{
			name: "neither present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{},
			},
			wantErr: false,
		},
		{
			name: "both present",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					Pipelines:    []commonv1.Config{},
					PipelinesRef: &commonv1.ConfigMapOrSecretSource{},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := checkSinglePipelineSource(&tc.logstash)
			assert.Equal(t, tc.wantErr, len(got) > 0)
		})
	}
}

func Test_checkPipelinesRefSource(t *testing.T) {
	tests := []struct {
		name     string
		logstash lsv1alpha1.Logstash
		wantErr  bool
	}{
		{
			name:    "nil pipelinesRef",
			wantErr: false,
		},
		{
			name: "secretName only",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					PipelinesRef: &commonv1.ConfigMapOrSecretSource{
						SecretName: "my-secret",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "configMapName only",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					PipelinesRef: &commonv1.ConfigMapOrSecretSource{
						ConfigMapName: "my-cm",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "both secretName and configMapName",
			logstash: lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					PipelinesRef: &commonv1.ConfigMapOrSecretSource{
						SecretName:    "my-secret",
						ConfigMapName: "my-cm",
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := checkPipelinesRefSource(&tc.logstash)
			assert.Equal(t, tc.wantErr, len(got) > 0)
		})
	}
}

func Test_checkSupportedVersion(t *testing.T) {
	for _, tt := range []struct {
		name    string
		version string
		wantErr bool
	}{
		{
			name:    "below min supported",
			version: "8.5.0",
			wantErr: true,
		},
		{
			name:    "above max supported",
			version: "42.0.0",
			wantErr: true,
		},
		{
			name:    "above min supported",
			version: "8.12.0",
			wantErr: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := lsv1alpha1.Logstash{
				Spec: lsv1alpha1.LogstashSpec{
					Version: tt.version,
				},
			}
			got := checkSupportedVersion(&a)
			assert.Equal(t, tt.wantErr, len(got) > 0)
		})
	}
}

func Test_checkEsRefsAssociations(t *testing.T) {
	type args struct {
		b *lsv1alpha1.Logstash
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "no ref: OK",
			args: args{
				b: &lsv1alpha1.Logstash{},
			},
			wantErr: false,
		},
		{
			name: "mix secret named and named refs: OK",
			args: args{
				b: &lsv1alpha1.Logstash{
					Spec: lsv1alpha1.LogstashSpec{
						ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
							{
								SecretName:  "bla",
								ClusterName: "test",
							},
							{
								Name: "bla", Namespace: "blub",
								ClusterName: "test2",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "secret named ref with a name: NOK",
			args: args{
				b: &lsv1alpha1.Logstash{
					Spec: lsv1alpha1.LogstashSpec{
						ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
							{
								SecretName: "bla", Name: "bla",
								ClusterName: "test",
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "no name or secret name with namespace: NOK",
			args: args{
				b: &lsv1alpha1.Logstash{
					Spec: lsv1alpha1.LogstashSpec{
						ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
							{
								Namespace:   "blub",
								ClusterName: "test",
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "no name or secret name with serviceName: NOK",
			args: args{
				b: &lsv1alpha1.Logstash{
					Spec: lsv1alpha1.LogstashSpec{
						ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
							{
								ServiceName: "ble",
								ClusterName: "test",
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkAssociations(tt.args.b)
			assert.Equal(t, tt.wantErr, len(got) > 0)
		})
	}
}

func Test_checkESRefsNamed(t *testing.T) {
	type args struct {
		b *lsv1alpha1.Logstash
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "no ref: OK",
			args: args{
				b: &lsv1alpha1.Logstash{},
			},
			wantErr: false,
		},
		{
			name: "one ref, missing clusterName: NOK",
			args: args{
				b: &lsv1alpha1.Logstash{
					Spec: lsv1alpha1.LogstashSpec{
						ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
							{
								Name: "bla", Namespace: "blub",
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "multiple refs, each with clusterName: OK",
			args: args{
				b: &lsv1alpha1.Logstash{
					Spec: lsv1alpha1.LogstashSpec{
						ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
							{
								Name: "bla", Namespace: "blub",
								ClusterName: "bla",
							},
							{
								Name: "bla", Namespace: "blub",
								ClusterName: "blub",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "multiple refs, one missing clusterName: NOK",
			args: args{
				b: &lsv1alpha1.Logstash{
					Spec: lsv1alpha1.LogstashSpec{
						ElasticsearchRefs: []lsv1alpha1.ElasticsearchCluster{
							{
								Name: "bla", Namespace: "blub",
								ClusterName: "",
							},
							{
								Name: "bla", Namespace: "blub",
								ClusterName: "default",
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkESRefsNamed(tt.args.b)
			assert.Equal(t, tt.wantErr, len(got) > 0)
		})
	}
}

func Test_checkPauseOrchestrationAnnotation(t *testing.T) {
	testCases := []struct {
		name    string
		ls      *lsv1alpha1.Logstash
		wantErr bool
	}{
		{
			name: "pause-orchestration false",
			ls: &lsv1alpha1.Logstash{
				Annotations: map[string]string{commonv1.PauseOrchestrationAnnotation: "false"},
			},
			wantErr: false,
		},
		{
			name: "pause-orchestration true",
			ls: &lsv1alpha1.Logstash{
				Annotations: map[string]string{commonv1.PauseOrchestrationAnnotation: "true"},
			},
			wantErr: false,
		},
		{
			name: "pause-orchestration invalid",
			ls: &lsv1alpha1.Logstash{
				Annotations: map[string]string{commonv1.PauseOrchestrationAnnotation: "True"},
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errList := commonv1.CheckPauseOrchestrationAnnotation(tc.ls)
			assert.Equal(t, tc.wantErr, len(errList) > 0)
		})
	}
}

func Test_checkServicesSubjectAltNames(t *testing.T) {
	selfSigned := func(sans ...commonv1.SubjectAlternativeName) commonv1.TLSOptions {
		return commonv1.TLSOptions{SelfSignedCertificate: &commonv1.SelfSignedCertificate{SubjectAlternativeNames: sans}}
	}
	tests := []struct {
		name       string
		services   []lsv1alpha1.LogstashService
		wantFields []string
	}{
		{
			name: "no service: OK",
		},
		{
			name: "services without self-signed certificate options: OK",
			services: []lsv1alpha1.LogstashService{
				{Name: "api"},
				{Name: "beats", TLS: commonv1.TLSOptions{Certificate: commonv1.SecretRef{SecretName: "my-cert"}}},
			},
		},
		{
			name: "valid DNS, IPv4 and IPv6 SANs: OK",
			services: []lsv1alpha1.LogstashService{
				{Name: "api", TLS: selfSigned(commonv1.SubjectAlternativeName{IP: "10.0.0.1"})},
				{Name: "beats", TLS: selfSigned(
					commonv1.SubjectAlternativeName{DNS: "logstash.example.com"},
					commonv1.SubjectAlternativeName{IP: "1.2.3.4"},
					commonv1.SubjectAlternativeName{IP: "2001:db8::1"},
				)},
			},
		},
		{
			name: "invalid IP SAN in a custom service: NOK",
			services: []lsv1alpha1.LogstashService{
				{Name: "api"},
				{Name: "beats", TLS: selfSigned(
					commonv1.SubjectAlternativeName{DNS: "logstash.example.com"},
					commonv1.SubjectAlternativeName{IP: "not-an-ip"},
				)},
			},
			wantFields: []string{"spec.services[1].tls.selfSignedCertificate.subjectAltNames[1].ip"},
		},
		{
			name: "invalid IP SANs in the API service and in a custom service: NOK",
			services: []lsv1alpha1.LogstashService{
				{Name: "api", TLS: selfSigned(commonv1.SubjectAlternativeName{IP: "1.2.3"})},
				{Name: "beats", TLS: selfSigned(commonv1.SubjectAlternativeName{IP: "1.2.3.4.5"})},
			},
			wantFields: []string{
				"spec.services[0].tls.selfSignedCertificate.subjectAltNames[0].ip",
				"spec.services[1].tls.selfSignedCertificate.subjectAltNames[0].ip",
			},
		},
		{
			name: "invalid IP SANs in disabled self-signed certificates are ignored: OK",
			services: []lsv1alpha1.LogstashService{
				{Name: "api", TLS: commonv1.TLSOptions{SelfSignedCertificate: &commonv1.SelfSignedCertificate{
					Disabled:                true,
					SubjectAlternativeNames: []commonv1.SubjectAlternativeName{{IP: "invalid"}},
				}}},
				{Name: "beats", TLS: commonv1.TLSOptions{SelfSignedCertificate: &commonv1.SelfSignedCertificate{
					Disabled:                true,
					SubjectAlternativeNames: []commonv1.SubjectAlternativeName{{IP: "invalid"}},
				}}},
			},
		},
		{
			name: "invalid IP SAN in a disabled self-signed certificate of the API service with a certificate Secret: NOK",
			services: []lsv1alpha1.LogstashService{
				{Name: "api", TLS: commonv1.TLSOptions{
					Certificate: commonv1.SecretRef{SecretName: "my-ca"},
					SelfSignedCertificate: &commonv1.SelfSignedCertificate{
						Disabled:                true,
						SubjectAlternativeNames: []commonv1.SubjectAlternativeName{{IP: "invalid"}},
					},
				}},
				{Name: "beats", TLS: commonv1.TLSOptions{
					Certificate: commonv1.SecretRef{SecretName: "my-cert"},
					SelfSignedCertificate: &commonv1.SelfSignedCertificate{
						Disabled:                true,
						SubjectAlternativeNames: []commonv1.SubjectAlternativeName{{IP: "invalid"}},
					},
				}},
			},
			wantFields: []string{"spec.services[0].tls.selfSignedCertificate.subjectAltNames[0].ip"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ls := &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{Services: tt.services}}
			var gotFields []string
			for _, err := range checkServicesSubjectAltNames(ls) {
				assert.Equal(t, field.ErrorTypeInvalid, err.Type)
				assert.Equal(t, invalidSanIPErrMsg, err.Detail)
				gotFields = append(gotFields, err.Field)
			}
			assert.Equal(t, tt.wantFields, gotFields)
		})
	}
}

func Test_servicesTLSWarnings(t *testing.T) {
	sans := []commonv1.SubjectAlternativeName{{DNS: "logstash.example.com"}}
	customCert := commonv1.TLSOptions{Certificate: commonv1.SecretRef{SecretName: "my-cert"}}
	tests := []struct {
		name     string
		services []lsv1alpha1.LogstashService
		// wantWarnings holds, for each expected warning, the field which the warning must start with.
		wantWarnings []string
	}{
		{
			name: "no service: no warning",
		},
		{
			name: "custom certificate and certificate authorities on the API service: no warning",
			services: []lsv1alpha1.LogstashService{
				{Name: "api", TLS: commonv1.TLSOptions{
					Certificate:            commonv1.SecretRef{SecretName: "my-cert"},
					CertificateAuthorities: commonv1.ConfigMapRef{ConfigMapName: "my-ca"},
				}},
			},
		},
		{
			name: "SANs on a custom service with a self-signed API service: no warning",
			services: []lsv1alpha1.LogstashService{
				{Name: "beats", TLS: commonv1.TLSOptions{
					SelfSignedCertificate: &commonv1.SelfSignedCertificate{SubjectAlternativeNames: sans},
				}},
			},
		},
		{
			name: "SANs on a custom service with TLS disabled on the API service: no warning",
			services: []lsv1alpha1.LogstashService{
				{Name: "api", TLS: commonv1.TLSOptions{SelfSignedCertificate: &commonv1.SelfSignedCertificate{Disabled: true}}},
				{Name: "beats", TLS: commonv1.TLSOptions{
					SelfSignedCertificate: &commonv1.SelfSignedCertificate{SubjectAlternativeNames: sans},
				}},
			},
		},
		{
			name: "custom certificate on a custom service: warning",
			services: []lsv1alpha1.LogstashService{
				{Name: "api"},
				{Name: "beats", TLS: customCert},
			},
			wantWarnings: []string{"spec.services[1].tls.certificate is ignored"},
		},
		{
			name: "certificate authorities on a custom service: warning",
			services: []lsv1alpha1.LogstashService{
				{Name: "beats", TLS: commonv1.TLSOptions{CertificateAuthorities: commonv1.ConfigMapRef{ConfigMapName: "my-ca"}}},
			},
			wantWarnings: []string{"spec.services[0].tls.certificateAuthorities is ignored"},
		},
		{
			name: "SANs on a custom service with a custom certificate on the API service: warning",
			services: []lsv1alpha1.LogstashService{
				{Name: "beats", TLS: commonv1.TLSOptions{
					SelfSignedCertificate: &commonv1.SelfSignedCertificate{SubjectAlternativeNames: sans},
				}},
				{Name: "api", TLS: customCert},
			},
			wantWarnings: []string{
				"spec.services[0].tls.selfSignedCertificate.subjectAltNames is used only if the `tls.certificate` Secret of the `api` Service contains a CA (`ca.crt`, `ca.key`). " +
					"It is ignored if this Secret contains a certificate (`tls.crt`, `tls.key`)",
			},
		},
		{
			name: "custom service without TLS options or without SAN, with a custom certificate on the API service: no warning",
			services: []lsv1alpha1.LogstashService{
				{Name: "api", TLS: customCert},
				{Name: "beats", TLS: commonv1.TLSOptions{SelfSignedCertificate: &commonv1.SelfSignedCertificate{}}},
				{Name: "tcp"},
				{Name: "udp", TLS: commonv1.TLSOptions{SelfSignedCertificate: &commonv1.SelfSignedCertificate{Disabled: true}}},
			},
		},
		{
			name: "custom service without TLS options with a self-signed API service: no warning",
			services: []lsv1alpha1.LogstashService{
				{Name: "beats"},
			},
		},
		{
			name: "SANs on a custom service with a disabled self-signed certificate: warning",
			services: []lsv1alpha1.LogstashService{
				{Name: "beats", TLS: commonv1.TLSOptions{
					SelfSignedCertificate: &commonv1.SelfSignedCertificate{Disabled: true, SubjectAlternativeNames: sans},
				}},
			},
			wantWarnings: []string{"spec.services[0].tls.selfSignedCertificate.subjectAltNames is ignored"},
		},
		{
			name: "SANs on a custom service with a disabled self-signed certificate and a custom certificate on the API service: one warning",
			services: []lsv1alpha1.LogstashService{
				{Name: "api", TLS: customCert},
				{Name: "tcp", TLS: commonv1.TLSOptions{
					SelfSignedCertificate: &commonv1.SelfSignedCertificate{Disabled: true, SubjectAlternativeNames: sans},
				}},
			},
			wantWarnings: []string{"spec.services[1].tls.selfSignedCertificate.subjectAltNames is ignored: the self-signed certificate is disabled"},
		},
		{
			name: "SANs on the API service with a disabled self-signed certificate: no warning",
			services: []lsv1alpha1.LogstashService{
				{Name: "api", TLS: commonv1.TLSOptions{
					SelfSignedCertificate: &commonv1.SelfSignedCertificate{Disabled: true, SubjectAlternativeNames: sans},
				}},
			},
		},
		{
			name: "all the ignored fields on one custom service: one warning for each field",
			services: []lsv1alpha1.LogstashService{
				{Name: "api", TLS: customCert},
				{Name: "beats", TLS: commonv1.TLSOptions{
					SelfSignedCertificate:  &commonv1.SelfSignedCertificate{SubjectAlternativeNames: sans},
					Certificate:            commonv1.SecretRef{SecretName: "my-cert"},
					CertificateAuthorities: commonv1.ConfigMapRef{ConfigMapName: "my-ca"},
				}},
			},
			wantWarnings: []string{
				"spec.services[1].tls.certificate is ignored",
				"spec.services[1].tls.certificateAuthorities is ignored",
				"spec.services[1].tls.selfSignedCertificate.subjectAltNames is used only if",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ls := &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{Services: tt.services}}
			ls.Name = "ls"
			got := servicesTLSWarnings(ls)
			if assert.Len(t, got, len(tt.wantWarnings)) {
				for i, want := range tt.wantWarnings {
					assert.Contains(t, got[i], want)
					// the API server may truncate a warning to 256 characters, leave room for a larger Service index
					assert.Less(t, len(got[i]), 250)
				}
			}
		})
	}
}

func TestValidateLogstash_servicesTLS(t *testing.T) {
	newLogstash := func(services ...lsv1alpha1.LogstashService) *lsv1alpha1.Logstash {
		ls := &lsv1alpha1.Logstash{Spec: lsv1alpha1.LogstashSpec{Version: "8.12.0", Services: services}}
		ls.Name = "ls"
		ls.Namespace = "ns"
		return ls
	}

	t.Run("ignored TLS options are reported as warnings and do not reject the resource", func(t *testing.T) {
		warnings, err := ValidateLogstash(newLogstash(lsv1alpha1.LogstashService{
			Name: "beats",
			TLS:  commonv1.TLSOptions{Certificate: commonv1.SecretRef{SecretName: "my-cert"}},
		}))
		assert.NoError(t, err)
		if assert.Len(t, warnings, 1) {
			assert.Contains(t, warnings[0], "spec.services[0].tls.certificate is ignored")
		}
	})

	t.Run("SANs on a custom service are accepted without warning", func(t *testing.T) {
		warnings, err := ValidateLogstash(newLogstash(lsv1alpha1.LogstashService{
			Name: "beats",
			TLS: commonv1.TLSOptions{SelfSignedCertificate: &commonv1.SelfSignedCertificate{
				SubjectAlternativeNames: []commonv1.SubjectAlternativeName{{DNS: "logstash.example.com"}, {IP: "1.2.3.4"}},
			}},
		}))
		assert.NoError(t, err)
		assert.Empty(t, warnings)
	})

	t.Run("an invalid IP SAN rejects the resource", func(t *testing.T) {
		_, err := ValidateLogstash(newLogstash(lsv1alpha1.LogstashService{
			Name: "beats",
			TLS: commonv1.TLSOptions{SelfSignedCertificate: &commonv1.SelfSignedCertificate{
				SubjectAlternativeNames: []commonv1.SubjectAlternativeName{{IP: "not-an-ip"}},
			}},
		}))
		if assert.Error(t, err) {
			assert.Contains(t, err.Error(), "spec.services[0].tls.selfSignedCertificate.subjectAltNames[0].ip")
			assert.Contains(t, err.Error(), invalidSanIPErrMsg)
		}
	})

	t.Run("an invalid IP SAN on a service with a disabled self-signed certificate is a warning, not an error", func(t *testing.T) {
		warnings, err := ValidateLogstash(newLogstash(lsv1alpha1.LogstashService{
			Name: "beats",
			TLS: commonv1.TLSOptions{SelfSignedCertificate: &commonv1.SelfSignedCertificate{
				Disabled:                true,
				SubjectAlternativeNames: []commonv1.SubjectAlternativeName{{IP: "not-an-ip"}},
			}},
		}))
		assert.NoError(t, err)
		if assert.Len(t, warnings, 1) {
			assert.Contains(t, warnings[0], "spec.services[0].tls.selfSignedCertificate.subjectAltNames is ignored")
		}
	})
}
