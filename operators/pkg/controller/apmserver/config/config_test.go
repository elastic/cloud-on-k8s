// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package config

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/apm/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// getDefaultConfigWithOuput returns a configuration with an Output containing the provided parameters.
func getDefaultConfigWithOuput(hosts []string, username, password string) *Config {
	return &Config{
		Name: "${POD_NAME}",
		ApmServer: ApmServerConfig{
			Host:               fmt.Sprintf(":%d", DefaultHTTPPort),
			SecretToken:        "${SECRET_TOKEN}",
			ReadTimeout:        3600,
			ShutdownTimeout:    "30s",
			Rum:                RumConfig{Enabled: true, RateLimit: 10},
			ConcurrentRequests: 1,
			MaxUnzippedSize:    5242880,
			SSL: TLSConfig{
				Enabled: false,
			},
		},
		XPackMonitoringEnabled: true,

		Logging: LoggingConfig{
			JSON:           true,
			MetricsEnabled: true,
		},
		Queue: QueueConfig{
			Mem: QueueMemConfig{
				Events: 2000,
				Flush: FlushConfig{
					MinEvents: 267,
					Timeout:   "1s",
				},
			},
		},
		SetupTemplateSettingsIndex: SetupTemplateSettingsIndex{
			NumberOfShards:     1,
			NumberOfReplicas:   1,
			AutoExpandReplicas: "0-2",
		},
		Output: OutputConfig{
			Elasticsearch: ElasticsearchOutputConfig{
				Worker:           5,
				MaxBulkSize:      267,
				CompressionLevel: 5,
				Hosts:            hosts,
				Username:         username,
				Password:         password,
				SSL: TLSConfig{
					Enabled:                true,
					CertificateAuthorities: []string{"config/elasticsearch-certs/ca.pem"},
				},
			},
		},
	}
}

func TestFromResourceSpec(t *testing.T) {
	type args struct {
		c  k8s.Client
		as v1alpha1.ApmServer
	}
	tests := []struct {
		name    string
		args    args
		want    *Config
		wantErr bool
	}{
		{
			name: "Test output configuration with a SecretKeyRef",
			args: args{
				c: k8s.WrapClient(fake.NewFakeClient(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "apmelasticsearchassociation-sample-elastic-internal-apm",
						Namespace: "default",
					},
					Data: map[string][]byte{"elastic-internal-apm": []byte("a2s1Nmt0N3Nwdmg4cmpqdDlucWhsN3cy")},
				})),
				as: v1alpha1.ApmServer{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "apm-server-sample",
						Namespace: "default",
					},
					Spec: v1alpha1.ApmServerSpec{
						Output: v1alpha1.Output{
							Elasticsearch: v1alpha1.ElasticsearchOutput{
								Hosts: []string{"https://elasticsearch-sample-es.default.svc.cluster.local:9200"},
								Auth: v1alpha1.ElasticsearchAuth{
									SecretKeyRef: &corev1.SecretKeySelector{
										Key: "elastic-internal-apm",
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "apmelasticsearchassociation-sample-elastic-internal-apm",
										},
									},
								},
							},
						},
					},
				},
			},
			want: getDefaultConfigWithOuput(
				[]string{"https://elasticsearch-sample-es.default.svc.cluster.local:9200"},
				"elastic-internal-apm",
				"a2s1Nmt0N3Nwdmg4cmpqdDlucWhsN3cy"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FromResourceSpec(tt.args.c, tt.args.as)
			if (err != nil) != tt.wantErr {
				t.Errorf("FromResourceSpec() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FromResourceSpec() = %v, want %v", got, tt.want)
			}
		})
	}
}
