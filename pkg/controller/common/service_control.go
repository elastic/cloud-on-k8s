// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"reflect"

	"go.elastic.co/apm/v2"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/compare"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/maps"
)

func ReconcileService(
	ctx context.Context,
	c k8s.Client,
	expected *corev1.Service,
	owner client.Object,
) (*corev1.Service, error) {
	span, _ := apm.StartSpan(ctx, "reconcile_service", tracing.SpanTypeApp)
	defer span.End()

	reconciled := &corev1.Service{}
	err := reconciler.ReconcileResource(reconciler.Params{
		Context:    ctx,
		Client:     c,
		Owner:      owner,
		Expected:   expected,
		Reconciled: reconciled,
		NeedsRecreate: func() bool {
			return needsRecreate(expected, reconciled)
		},
		NeedsUpdate: func() bool {
			return needsUpdate(expected, reconciled)
		},
		UpdateReconciled: func() {
			reconciled.Annotations = expected.Annotations
			reconciled.Labels = expected.Labels
			reconciled.Spec = expected.Spec
		},
	})
	return reconciled, err
}

func needsRecreate(expected, reconciled *corev1.Service) bool {
	applyServerSideValues(expected, reconciled)

	// IPFamilies is immutable
	if expected.Spec.IPFamilies != nil {
		if len(expected.Spec.IPFamilies) != len(reconciled.Spec.IPFamilies) {
			return true
		}

		for i := 0; i < len(expected.Spec.IPFamilies); i++ {
			if expected.Spec.IPFamilies[i] != reconciled.Spec.IPFamilies[i] {
				return true
			}
		}
	}

	// ClusterIP is immutable
	if expected.Spec.ClusterIP != reconciled.Spec.ClusterIP {
		return true
	}

	// LoadBalancerClass is immutable once set if target type is load balancer
	if expected.Spec.Type == corev1.ServiceTypeLoadBalancer && !reflect.DeepEqual(expected.Spec.LoadBalancerClass, reconciled.Spec.LoadBalancerClass) {
		return true
	}

	return false
}

func needsUpdate(expected *corev1.Service, reconciled *corev1.Service) bool {
	applyServerSideValues(expected, reconciled)
	// if the specs, labels, or annotations differ, the object should be updated
	return !(reflect.DeepEqual(expected.Spec, reconciled.Spec) &&
		compare.LabelsAndAnnotationsAreEqual(expected.ObjectMeta, reconciled.ObjectMeta))
}

// applyServerSideValues applies any default that may have been set from the reconciled version.
func applyServerSideValues(expected, reconciled *corev1.Service) {
	// Type may be defaulted by the api server
	if expected.Spec.Type == "" {
		expected.Spec.Type = reconciled.Spec.Type
	}
	// ClusterIPs might not exist in the expected service,
	// but might have been set after creation by k8s on the actual resource.
	// In such case, we want to use these values for comparison.
	// But only if we are not changing the type of service and the api server has assigned an IP
	if expected.Spec.Type == reconciled.Spec.Type {
		if expected.Spec.ClusterIP == "" {
			expected.Spec.ClusterIP = reconciled.Spec.ClusterIP
		}
		if len(expected.Spec.ClusterIPs) == 0 {
			expected.Spec.ClusterIPs = reconciled.Spec.ClusterIPs
		}
	}

	// SessionAffinity may be defaulted by the api server
	if expected.Spec.SessionAffinity == "" {
		expected.Spec.SessionAffinity = reconciled.Spec.SessionAffinity
	}

	// same for the target port and node port
	if len(expected.Spec.Ports) == len(reconciled.Spec.Ports) {
		for i := range expected.Spec.Ports {
			if expected.Spec.Ports[i].TargetPort.IntValue() == 0 {
				expected.Spec.Ports[i].TargetPort = reconciled.Spec.Ports[i].TargetPort
			}
			// check if NodePort makes sense for this service type
			if hasNodePort(expected.Spec.Type) && expected.Spec.Ports[i].NodePort == 0 {
				expected.Spec.Ports[i].NodePort = reconciled.Spec.Ports[i].NodePort
			}
		}
	}

	if expected.Spec.HealthCheckNodePort == 0 {
		expected.Spec.HealthCheckNodePort = reconciled.Spec.HealthCheckNodePort
	}

	expected.Annotations = maps.MergePreservingExistingKeys(expected.Annotations, reconciled.Annotations)
	expected.Labels = maps.MergePreservingExistingKeys(expected.Labels, reconciled.Labels)

	// IPFamily is immutable and cannot be modified so we should retain the existing value from the server if there's no explicit override.
	if expected.Spec.IPFamilies == nil {
		expected.Spec.IPFamilies = reconciled.Spec.IPFamilies
	}

	// IPFamilyPolicy is immutable and cannot be modified so we should retain the existing value from the server if there's no explicit override.
	if expected.Spec.IPFamilyPolicy == nil {
		expected.Spec.IPFamilyPolicy = reconciled.Spec.IPFamilyPolicy
	}

	// InternalTrafficPolicy may be defaulted by the api server starting K8S v1.22
	if expected.Spec.InternalTrafficPolicy == nil {
		expected.Spec.InternalTrafficPolicy = reconciled.Spec.InternalTrafficPolicy
	}

	if expected.Spec.ExternalTrafficPolicy == "" {
		expected.Spec.ExternalTrafficPolicy = reconciled.Spec.ExternalTrafficPolicy
	}

	// LoadBalancerClass may be defaulted by the API server starting K8s v.1.24
	if expected.Spec.Type == corev1.ServiceTypeLoadBalancer && expected.Spec.LoadBalancerClass == nil {
		expected.Spec.LoadBalancerClass = reconciled.Spec.LoadBalancerClass
	}

	if expected.Spec.AllocateLoadBalancerNodePorts == nil {
		expected.Spec.AllocateLoadBalancerNodePorts = reconciled.Spec.AllocateLoadBalancerNodePorts
	}
}

// hasNodePort returns for a given service type, if the service ports have a NodePort or not.
func hasNodePort(svcType corev1.ServiceType) bool {
	return svcType == corev1.ServiceTypeNodePort || svcType == corev1.ServiceTypeLoadBalancer
}
