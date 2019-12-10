// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package apmserver

import (
	apmv1 "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/apmserver/labels"
	apmname "github.com/elastic/cloud-on-k8s/pkg/controller/apmserver/name"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	corev1 "k8s.io/api/core/v1"
)

func NewService(as apmv1.ApmServer) *corev1.Service {
	svc := corev1.Service{
		ObjectMeta: as.Spec.HTTP.Service.ObjectMeta,
		Spec:       as.Spec.HTTP.Service.Spec,
	}

	svc.ObjectMeta.Namespace = as.Namespace
	svc.ObjectMeta.Name = apmname.HTTPService(as.Name)

	labels := labels.NewLabels(as.Name)
	ports := []corev1.ServicePort{
		{
			Name:     as.Spec.HTTP.Protocol(),
			Protocol: corev1.ProtocolTCP,
			Port:     HTTPPort,
		},
	}

	return defaults.SetServiceDefaults(&svc, labels, labels, ports)
}
