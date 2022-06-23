// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package defaults

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/compare"
)

func TestSetServiceDefaults(t *testing.T) {
	testCases := []struct {
		name            string
		inSvc           func() *corev1.Service
		defaultLabels   map[string]string
		defaultSelector map[string]string
		defaultPorts    []corev1.ServicePort
		wantSvc         func() *corev1.Service
	}{
		{
			name: "defaults are applied to empty service",
			inSvc: func() *corev1.Service {
				return &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{"bar": "baz"},
					},
				}
			},
			defaultLabels:   map[string]string{"foo": "bar"},
			defaultSelector: map[string]string{"foo": "bar"},
			defaultPorts:    []corev1.ServicePort{{Name: "https", Port: 443}},
			wantSvc:         mkService,
		},
		{
			name:  "existing values take precedence over defaults",
			inSvc: mkService,
			defaultLabels: map[string]string{
				"foo": "foo", // should be ignored
				"bar": "baz", // should be added
			},
			defaultSelector: map[string]string{"foo": "foo", "bar": "bar"},     // should be completely ignored
			defaultPorts:    []corev1.ServicePort{{Name: "https", Port: 8443}}, // should be completely ignored
			wantSvc: func() *corev1.Service {
				svc := mkService()
				svc.Labels["bar"] = "baz"
				return svc
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			haveSvc := SetServiceDefaults(tc.inSvc(), tc.defaultLabels, tc.defaultSelector, tc.defaultPorts)
			compare.JSONEqual(t, tc.wantSvc(), haveSvc)
		})
	}
}

func mkService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      map[string]string{"foo": "bar"},
			Annotations: map[string]string{"bar": "baz"},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"foo": "bar"},
			Ports: []corev1.ServicePort{
				{Name: "https", Port: 443},
			},
		},
	}
}
