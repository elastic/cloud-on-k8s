// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"context"
	"net"
	"reflect"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/utils/compare"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
	"go.elastic.co/apm"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("common")

func ReconcileService(
	ctx context.Context,
	c k8s.Client,
	expected *corev1.Service,
	owner metav1.Object,
) (*corev1.Service, error) {
	span, _ := apm.StartSpan(ctx, "reconcile_service", tracing.SpanTypeApp)
	defer span.End()

	reconciled := &corev1.Service{}
	err := reconciler.ReconcileResource(reconciler.Params{
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
	// ClusterIP is an immutable field
	return expected.Spec.ClusterIP != reconciled.Spec.ClusterIP
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
	// ClusterIP might not exist in the expected service,
	// but might have been set after creation by k8s on the actual resource.
	// In such case, we want to use these values for comparison.
	// But only if we are not changing the type of service and the api server has assigned an IP
	if expected.Spec.Type == reconciled.Spec.Type && expected.Spec.ClusterIP == "" && net.ParseIP(reconciled.Spec.ClusterIP) != nil {
		expected.Spec.ClusterIP = reconciled.Spec.ClusterIP
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
}

// hasNodePort returns for a given service type, if the service ports have a NodePort or not.
func hasNodePort(svcType corev1.ServiceType) bool {
	return svcType == corev1.ServiceTypeNodePort || svcType == corev1.ServiceTypeLoadBalancer
}
