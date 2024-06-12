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
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/network"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
)

const (
	LogstashAPIServiceName = "api"
)

// reconcileServices reconcile Services defined in spec, return Services, the API Service, error
//
// When a service is defined that matches the API service name, then that service is used to define
// the service for the logstash API. If not, then a default service is created for the API service.
func reconcileServices(params Params) ([]corev1.Service, corev1.Service, error) {
	var apiSvc corev1.Service
	createdAPIService := false

	svcs := make([]corev1.Service, 0, len(params.Logstash.Spec.Services)+1)
	for _, service := range params.Logstash.Spec.Services {
		logstash := params.Logstash
		svc := newService(service, params.Logstash)
		if err := reconcileService(params, svc); err != nil {
			return []corev1.Service{}, corev1.Service{}, err
		}
		if logstashv1alpha1.UserServiceName(logstash.Name, service.Name) == logstashv1alpha1.APIServiceName(logstash.Name) {
			createdAPIService = true
			apiSvc = *svc
		}
		svcs = append(svcs, *svc)
	}
	if !createdAPIService {
		svc := newAPIService(params.Logstash)
		if err := reconcileService(params, svc); err != nil {
			return []corev1.Service{}, corev1.Service{}, err
		}
		apiSvc = *svc
		svcs = append(svcs, *svc)
	}

	return svcs, apiSvc, nil
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

	labels := labels.NewLabels(logstash)
	svc.Labels = maps.MergePreservingExistingKeys(svc.Labels, labels)

	if svc.Spec.Selector == nil {
		svc.Spec.Selector = labels
	}

	return &svc
}

func newAPIService(logstash logstashv1alpha1.Logstash) *corev1.Service {
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{},
		Spec:       corev1.ServiceSpec{ClusterIP: "None"},
	}

	svc.ObjectMeta.Namespace = logstash.Namespace
	svc.ObjectMeta.Name = logstashv1alpha1.APIServiceName(logstash.Name)

	labels := labels.NewLabels(logstash)
	ports := []corev1.ServicePort{
		{
			Name:     LogstashAPIServiceName,
			Protocol: corev1.ProtocolTCP,
			Port:     network.HTTPPort,
		},
	}
	return defaults.SetServiceDefaults(&svc, labels, labels, ports)
}
