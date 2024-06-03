// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package services

import (
	"errors"
	"testing"

	"github.com/go-test/deep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	commonv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/network"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/compare"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func TestExternalServiceURL(t *testing.T) {
	type args struct {
		es esv1.Elasticsearch
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "A service URL",
			args: args{es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "an-es-name",
					Namespace: "default",
				},
			}},
			want: "https://an-es-name-es-http.default.svc:9200",
		},
		{
			name: "Another Service URL",
			args: args{es: esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "another-es-name",
					Namespace: "default",
				},
			}},
			want: "https://another-es-name-es-http.default.svc:9200",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExternalServiceURL(tt.args.es)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewExternalService(t *testing.T) {
	testCases := []struct {
		name     string
		httpConf commonv1.HTTPConfig
		wantSvc  func() corev1.Service
	}{
		{
			name: "no TLS",
			httpConf: commonv1.HTTPConfig{
				TLS: commonv1.TLSOptions{
					SelfSignedCertificate: &commonv1.SelfSignedCertificate{
						Disabled: true,
					},
				},
			},
			wantSvc: mkHTTPService,
		},
		{
			name: "self-signed certificate",
			httpConf: commonv1.HTTPConfig{
				TLS: commonv1.TLSOptions{
					SelfSignedCertificate: &commonv1.SelfSignedCertificate{
						SubjectAlternativeNames: []commonv1.SubjectAlternativeName{
							{
								DNS: "elasticsearch-test.local",
							},
						},
					},
				},
			},
			wantSvc: func() corev1.Service {
				svc := mkHTTPService()
				svc.Spec.Ports[0].Name = "https"
				return svc
			},
		},
		{
			name: "user-provided certificate",
			httpConf: commonv1.HTTPConfig{
				TLS: commonv1.TLSOptions{
					Certificate: commonv1.SecretRef{
						SecretName: "my-cert",
					},
				},
			},
			wantSvc: func() corev1.Service {
				svc := mkHTTPService()
				svc.Spec.Ports[0].Name = "https"
				return svc
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			es := mkElasticsearch(tc.httpConf)
			haveSvc := NewExternalService(es)
			compare.JSONEqual(t, tc.wantSvc(), haveSvc)
		})
	}
}

func TestNewInternalService(t *testing.T) {
	testCases := []struct {
		name     string
		httpConf commonv1.HTTPConfig
		wantSvc  func() corev1.Service
	}{
		{
			name: "user supplied selector is not applied to internal service",
			httpConf: commonv1.HTTPConfig{
				Service: commonv1.ServiceTemplate{
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{
							"app": "coordinating-node",
						},
					},
				},
			},
			wantSvc: func() corev1.Service {
				svc := mkHTTPService()
				svc.Spec.Type = corev1.ServiceTypeClusterIP
				svc.Spec.Ports[0].Name = "https"
				svc.Name = "elasticsearch-test-es-internal-http"
				return svc
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			es := mkElasticsearch(tc.httpConf)
			haveSvc := NewInternalService(es)
			compare.JSONEqual(t, tc.wantSvc(), haveSvc)
		})
	}
}

func mkHTTPService() corev1.Service {
	return corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "elasticsearch-test-es-http",
			Namespace: "test",
			Labels: map[string]string{
				label.ClusterNameLabelName: "elasticsearch-test",
				commonv1.TypeLabelName:     label.Type,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     "http",
					Protocol: corev1.ProtocolTCP,
					Port:     network.HTTPPort,
				},
			},
			Selector: map[string]string{
				label.ClusterNameLabelName: "elasticsearch-test",
				commonv1.TypeLabelName:     label.Type,
			},
		},
	}
}

func mkTransportService() corev1.Service {
	return corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "elasticsearch-test-es-transport",
			Namespace: "test",
			Labels: map[string]string{
				label.ClusterNameLabelName: "elasticsearch-test",
				commonv1.TypeLabelName:     label.Type,
			},
		},
		Spec: corev1.ServiceSpec{
			PublishNotReadyAddresses: true,
			ClusterIP:                "None",
			Type:                     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:     "tls-transport",
					Protocol: corev1.ProtocolTCP,
					Port:     network.TransportPort,
				},
			},
			Selector: map[string]string{
				label.ClusterNameLabelName: "elasticsearch-test",
				commonv1.TypeLabelName:     label.Type,
			},
		},
	}
}

func mkElasticsearch(httpConf commonv1.HTTPConfig) esv1.Elasticsearch {
	return esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "elasticsearch-test",
			Namespace: "test",
		},
		Spec: esv1.ElasticsearchSpec{
			HTTP: httpConf,
		},
	}
}

func TestNewTransportService(t *testing.T) {
	tests := []struct {
		name         string
		transportCfg esv1.TransportConfig
		want         func() corev1.Service
	}{
		{
			name: "Sets defaults",
			want: mkTransportService,
		},
		{
			name: "Respects user provided template",
			transportCfg: esv1.TransportConfig{
				Service: commonv1.ServiceTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"my-custom": "annotation",
						},
					},
					Spec: corev1.ServiceSpec{
						Type: corev1.ServiceTypeLoadBalancer,
					},
				},
			},
			want: func() corev1.Service {
				svc := mkTransportService()
				svc.ObjectMeta.Annotations = map[string]string{
					"my-custom": "annotation",
				}
				svc.Spec.Type = corev1.ServiceTypeLoadBalancer
				svc.Spec.ClusterIP = ""
				return svc
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := esv1.Elasticsearch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "elasticsearch-test",
					Namespace: "test",
				},
				Spec: esv1.ElasticsearchSpec{
					Transport: tt.transportCfg,
				},
			}
			want := tt.want()
			got := NewTransportService(es)
			require.Nil(t, deep.Equal(*got, want))
		})
	}
}

func Test_urlProvider_PodURL(t *testing.T) {
	type fields struct {
		pods   func() ([]corev1.Pod, error)
		svcURL string
	}
	tests := []struct {
		name    string
		fields  fields
		want    []string
		wantErr bool
	}{
		{
			name: "fetch failure",
			fields: fields{
				pods: func() ([]corev1.Pod, error) {
					return nil, errors.New("failed to fetch pods")
				},
			},
			wantErr: true,
		},
		{
			name: "no pods or error fetching pods: fall back to svc url",
			fields: fields{
				pods: func() ([]corev1.Pod, error) {
					return nil, nil
				},
				svcURL: "svc.url",
			},
			want: []string{"svc.url"},
		},
		{
			name: "ready and running pods: prefer ready",
			fields: fields{
				pods: func() ([]corev1.Pod, error) {
					return []corev1.Pod{
						//     name   running ready
						mkPod("sset-0", true, true),
						mkPod("sset-1", true, false),
						mkPod("sset-2", true, true),
						mkPod("sset-3", false, false),
					}, nil
				},
			},
			want: []string{"http://sset-0.sset.test:9200", "http://sset-2.sset.test:9200"},
		},
		{
			name: "only running pods: allow running ",
			fields: fields{
				pods: func() ([]corev1.Pod, error) {
					return []corev1.Pod{
						//     name   running ready
						mkPod("sset-0", true, false),
						mkPod("sset-1", true, false),
						mkPod("sset-2", false, false),
					}, nil
				},
			},
			want: []string{"http://sset-0.sset.test:9200", "http://sset-1.sset.test:9200"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &urlProvider{
				pods:   tt.fields.pods,
				svcURL: tt.fields.svcURL,
			}
			url, err := u.URL()
			if (err != nil) != tt.wantErr {
				t.Errorf("urlProvider.URL() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				require.Contains(t, tt.want, url, "must contain one of expected url")
			}
		})
	}
}

func mkPod(name string, running bool, ready bool) corev1.Pod {
	phase := corev1.PodPending
	if running {
		phase = corev1.PodRunning
	}
	var conditions []corev1.PodCondition
	if ready {
		conditions = append(conditions,
			corev1.PodCondition{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}, corev1.PodCondition{
				Type:   corev1.ContainersReady,
				Status: corev1.ConditionTrue,
			},
		)
	}
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Name:      name,
			Labels: map[string]string{
				label.HTTPSchemeLabelName:      "http",
				label.StatefulSetNameLabelName: "sset",
				label.ClusterNameLabelName:     "elasticsearch-test",
			},
		},
		Status: corev1.PodStatus{
			Phase:      phase,
			Conditions: conditions,
		},
	}
}

func TestNewElasticsearchURLProvider(t *testing.T) {
	type args struct {
		es     esv1.Elasticsearch
		client k8s.Client
	}
	tests := []struct {
		name         string
		args         args
		wantPodNames []string
		wantErr      bool
	}{
		{
			name: "cache failures are returned to the caller",
			args: args{
				es:     mkElasticsearch(commonv1.HTTPConfig{}),
				client: k8s.NewFailingClient(errors.New("boom")),
			},
			wantErr: true,
		},
		{
			name: "list pods from cache",
			args: args{
				es: mkElasticsearch(commonv1.HTTPConfig{}),
				client: k8s.NewFakeClient(
					ptr.To(mkPod("sset-0", true, true)),
					ptr.To(mkPod("sset-1", true, false)),
					&corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "test",
							Name:      "unrelated-0",
							Labels: map[string]string{
								label.HTTPSchemeLabelName:      "http",
								label.StatefulSetNameLabelName: "unrelated",
								label.ClusterNameLabelName:     "unrelated",
							},
						},
					},
				),
			},
			wantPodNames: []string{"sset-0", "sset-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewElasticsearchURLProvider(tt.args.es, tt.args.client)

			providerImpl, ok := provider.(*urlProvider)
			require.True(t, ok, "must be the urlProvider impl")

			got, err := providerImpl.pods()
			if (err != nil) != tt.wantErr {
				t.Errorf("NewElasticsearchURLProvider.URL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			require.ElementsMatch(t, k8s.PodNames(got), tt.wantPodNames)
		})
	}
}

func Test_urlProvider_Equals(t *testing.T) {
	type fields struct {
		svcURL string
	}
	type args struct {
		other client.URLProvider
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{
			name: "svc url is used as the identity",
			fields: fields{
				svcURL: "http://elastic.co",
			},
			args: args{
				other: &urlProvider{
					svcURL: "http://elastic.co",
				},
			},
			want: true,
		},
		{
			name: "different impl with same URL is not equal",
			fields: fields{
				svcURL: "http://k8s.io",
			},
			args: args{
				other: client.NewStaticURLProvider("http://k8s.io"),
			},
			want: false,
		},
		{
			name: "different URLs are not equal",
			fields: fields{
				svcURL: "http://a.com",
			},
			args: args{
				other: &urlProvider{
					svcURL: "http://b.com",
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &urlProvider{
				pods: func() ([]corev1.Pod, error) {
					return nil, nil
				},
				svcURL: tt.fields.svcURL,
			}
			if got := u.Equals(tt.args.other); got != tt.want {
				t.Errorf("urlProvider.Equals() = %v, want %v", got, tt.want)
			}
		})
	}
}
