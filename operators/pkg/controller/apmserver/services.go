// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	v1alpha1 "github.com/elastic/k8s-operators/operators/pkg/apis/apm/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewService(as v1alpha1.ApmServer) *corev1.Service {
	var svc = corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: as.Namespace,
			Name:      PseudoNamespacedResourceName(as),
			Labels:    NewLabels(as.Name),
			Annotations: as.Spec.HTTP.Service.Metadata.Annotations,
		},
		Spec: corev1.ServiceSpec{
			Selector: NewLabels(as.Name),
			Ports: []corev1.ServicePort{
				{
					Protocol: corev1.ProtocolTCP,
					Port:     HTTPPort,
				},
			},
			SessionAffinity: corev1.ServiceAffinityNone,
			Type:            common.GetServiceType(as.Spec.HTTP.Service.Spec.Type),
		},
	}
	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		svc.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyTypeCluster
	}
	return &svc
}
