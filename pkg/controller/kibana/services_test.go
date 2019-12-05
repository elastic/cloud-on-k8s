// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"testing"

	commonv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/pod"
	"github.com/elastic/cloud-on-k8s/pkg/utils/compare"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewService(t *testing.T) {
	testCases := []struct {
		name     string
		httpConf commonv1beta1.HTTPConfig
		wantSvc  func() corev1.Service
	}{
		{
			name: "no TLS",
			httpConf: commonv1beta1.HTTPConfig{
				TLS: commonv1beta1.TLSOptions{
					SelfSignedCertificate: &commonv1beta1.SelfSignedCertificate{
						Disabled: true,
					},
				},
			},
			wantSvc: mkService,
		},
		{
			name: "self-signed certificate",
			httpConf: commonv1beta1.HTTPConfig{
				TLS: commonv1beta1.TLSOptions{
					SelfSignedCertificate: &commonv1beta1.SelfSignedCertificate{
						SubjectAlternativeNames: []commonv1beta1.SubjectAlternativeName{
							{
								DNS: "kibana-test.local",
							},
						},
					},
				},
			},
			wantSvc: func() corev1.Service {
				svc := mkService()
				svc.Spec.Ports[0].Name = "https"
				return svc
			},
		},
		{
			name: "user-provided certificate",
			httpConf: commonv1beta1.HTTPConfig{
				TLS: commonv1beta1.TLSOptions{
					Certificate: commonv1beta1.SecretRef{
						SecretName: "my-cert",
					},
				},
			},
			wantSvc: func() corev1.Service {
				svc := mkService()
				svc.Spec.Ports[0].Name = "https"
				return svc
			},
		},
		{
			name: "service template",
			httpConf: commonv1beta1.HTTPConfig{
				Service: commonv1beta1.ServiceTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      map[string]string{"foo": "bar"},
						Annotations: map[string]string{"bar": "baz"},
					},
				},
			},
			wantSvc: func() corev1.Service {
				svc := mkService()
				svc.Labels["foo"] = "bar"
				svc.Annotations = map[string]string{"bar": "baz"}
				svc.Spec.Ports[0].Name = "https"
				return svc
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			kb := mkKibana(tc.httpConf)
			haveSvc := NewService(kb)
			compare.JSONEqual(t, tc.wantSvc(), haveSvc)
		})
	}
}

func mkService() corev1.Service {
	return corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kibana-test-kb-http",
			Namespace: "test",
			Labels: map[string]string{
				label.KibanaNameLabelName: "kibana-test",
				common.TypeLabelName:      label.Type,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     "http",
					Protocol: corev1.ProtocolTCP,
					Port:     pod.HTTPPort,
				},
			},
			Selector: map[string]string{
				label.KibanaNameLabelName: "kibana-test",
				common.TypeLabelName:      label.Type,
			},
		},
	}
}

func mkKibana(httpConf commonv1beta1.HTTPConfig) v1beta1.Kibana {
	return v1beta1.Kibana{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kibana-test",
			Namespace: "test",
		},
		Spec: v1beta1.KibanaSpec{
			HTTP: httpConf,
		},
	}
}
