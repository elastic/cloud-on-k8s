// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1beta1"
	commonv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver/labels"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
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
								DNS: "apm-test.local",
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			apm := mkAPMServer(tc.httpConf)
			haveSvc := NewService(apm)
			compare.JSONEqual(t, tc.wantSvc(), haveSvc)
		})
	}
}

func mkService() corev1.Service {
	return corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apm-test-apm-http",
			Namespace: "test",
			Labels: map[string]string{
				labels.ApmServerNameLabelName: "apm-test",
				common.TypeLabelName:          labels.Type,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     "http",
					Protocol: corev1.ProtocolTCP,
					Port:     HTTPPort,
				},
			},
			Selector: map[string]string{
				labels.ApmServerNameLabelName: "apm-test",
				common.TypeLabelName:          labels.Type,
			},
		},
	}
}

func mkAPMServer(httpConf commonv1beta1.HTTPConfig) v1beta1.ApmServer {
	return v1beta1.ApmServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apm-test",
			Namespace: "test",
		},
		Spec: v1beta1.ApmServerSpec{
			HTTP: httpConf,
		},
	}
}
