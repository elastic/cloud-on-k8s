// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/network"
)

// reconcileServices reconcile Services defined in spec
// When Services are empty, a default Service for port 9600 is created.
// If api.http.port is customized, user is expected to config Services.
// When Services exist, the port 9600 does not attach to any of Service.
func reconcileServices(params Params) ([]corev1.Service, error) {
	if len(params.Logstash.Spec.Services) == 0 {
		svc := newDefaultService(params.Logstash)

		if err := reconcileService(params, svc); err != nil {
			return []corev1.Service{}, err
		}

		return []corev1.Service{*svc}, nil
	}

	svcs := make([]corev1.Service, len(params.Logstash.Spec.Services))
	for i, service := range params.Logstash.Spec.Services {
		svc := newService(service, params.Logstash)

		if err := reconcileService(params, svc); err != nil {
			return []corev1.Service{}, err
		}

		svcs[i] = *svc
	}

	return svcs, nil
}

func reconcileService(params Params, service *corev1.Service) error {
	_, err := common.ReconcileService(params.Context, params.Client, service, &params.Logstash)
	if err != nil {
		return err
	}
	return nil
}

func newService(service logstashv1alpha1.LogstashService, logstash logstashv1alpha1.Logstash) *corev1.Service {
	svc := corev1.Service{
		ObjectMeta: service.Service.ObjectMeta,
		Spec:       service.Service.Spec,
	}

	svc.ObjectMeta.Namespace = logstash.Namespace
	svc.ObjectMeta.Name = logstashv1alpha1.UserServiceName(logstash.Name, service.Name)

	labels := NewLabels(logstash)

	svc.Labels = labels

	if svc.Spec.Selector == nil {
		svc.Spec.Selector = labels
	}

	return &svc
}

func newDefaultService(logstash logstashv1alpha1.Logstash) *corev1.Service {
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{},
		Spec:       corev1.ServiceSpec{},
	}

	svc.ObjectMeta.Namespace = logstash.Namespace
	svc.ObjectMeta.Name = logstashv1alpha1.HTTPServiceName(logstash.Name)

	labels := NewLabels(logstash)
	ports := []corev1.ServicePort{
		{
			Name:     "metrics",
			Protocol: corev1.ProtocolTCP,
			Port:     network.HTTPPort,
		},
	}
	return defaults.SetServiceDefaults(&svc, labels, labels, ports)
}
