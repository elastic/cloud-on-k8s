// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kibanav1alpha1 "github.com/elastic/cloud-on-k8s/operators/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/kibana/pod"
)

func NewService(kb kibanav1alpha1.Kibana) *corev1.Service {
	var svc = corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   kb.Namespace,
			Name:        PseudoNamespacedResourceName(kb),
			Labels:      label.NewLabels(kb.Name),
			Annotations: kb.Spec.HTTP.Service.Metadata.Annotations,
		},
		Spec: corev1.ServiceSpec{
			Selector: label.NewLabels(kb.Name),
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Protocol: corev1.ProtocolTCP,
					Port:     pod.HTTPPort,
				},
			},
			SessionAffinity: corev1.ServiceAffinityNone,
			// TODO: proper ingress forwarding
			Type: common.GetServiceType(kb.Spec.HTTP.Service.Spec.Type),
		},
	}
	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		svc.Spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyTypeCluster
	}
	return &svc
}
