// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	corev1 "k8s.io/api/core/v1"

	kibanav1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/label"
	kbname "github.com/elastic/cloud-on-k8s/pkg/controller/kibana/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/kibana/pod"
)

func NewService(kb kibanav1alpha1.Kibana) *corev1.Service {
	svc := corev1.Service{
		ObjectMeta: kb.Spec.HTTP.Service.ObjectMeta,
		Spec:       kb.Spec.HTTP.Service.Spec,
	}

	svc.ObjectMeta.Namespace = kb.Namespace
	svc.ObjectMeta.Name = kbname.HTTPService(kb.Name)

	labels := label.NewLabels(kb.Name)
	ports := []corev1.ServicePort{
		{
			Protocol: corev1.ProtocolTCP,
			Port:     pod.HTTPPort,
		},
	}

	return defaults.SetServiceDefaults(&svc, labels, labels, ports)
}
