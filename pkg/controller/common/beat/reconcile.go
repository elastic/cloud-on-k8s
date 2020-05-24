// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/beat/health"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/daemonset"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
)

func reconcilePodVehicle(podTemplate corev1.PodTemplateSpec, params DriverParams) (DriverStatus, error) {
	var reconciliationFunc func(params ReconciliationParams) (int32, int32, error)

	name := params.Namer.Name(params.Type, params.Owner.GetName())
	var toDelete runtime.Object
	switch {
	case params.DaemonSet != nil:
		{
			reconciliationFunc = reconcileDaemonSet
			toDelete = &v1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: params.Owner.GetNamespace(),
				},
			}
		}
	case params.Deployment != nil:
		{
			reconciliationFunc = reconcileDeployment
			toDelete = &v1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: params.Owner.GetNamespace(),
				},
			}
		}
	}

	ready, desired, err := reconciliationFunc(ReconciliationParams{
		client:      params.Client,
		name:        name,
		podTemplate: podTemplate,
		owner:       params.Owner,
		labels:      params.Labels,
		selectors:   params.Selectors,
		replicas:    params.GetReplicas(),
	})
	if err != nil {
		return DriverStatus{}, err
	}

	// clean up the other one
	if err := params.Client.Delete(toDelete); err != nil && !apierrors.IsNotFound(err) {
		return DriverStatus{}, err
	}

	return DriverStatus{
		ExpectedNodes:  desired,
		AvailableNodes: ready,
		Health:         health.CalculateHealth(params.Associated, ready, desired),
		Association:    params.Associated.AssociationStatus(),
	}, nil
}

type ReconciliationParams struct {
	client      k8s.Client
	name        string
	podTemplate corev1.PodTemplateSpec
	owner       metav1.Object
	labels      map[string]string
	selectors   map[string]string
	replicas    *int32
}

func reconcileDeployment(rp ReconciliationParams) (int32, int32, error) {
	d := deployment.New(deployment.Params{
		Name:            rp.name,
		Namespace:       rp.owner.GetNamespace(),
		Selector:        rp.selectors,
		Labels:          rp.labels,
		PodTemplateSpec: rp.podTemplate,
		Replicas:        pointer.Int32OrDefault(rp.replicas, int32(1)),
	})
	if err := controllerutil.SetControllerReference(rp.owner, &d, scheme.Scheme); err != nil {
		return 0, 0, err
	}

	reconciled, err := deployment.Reconcile(rp.client, d, rp.owner)
	if err != nil {
		return 0, 0, err
	}

	return reconciled.Status.ReadyReplicas, reconciled.Status.Replicas, nil
}

func reconcileDaemonSet(rp ReconciliationParams) (int32, int32, error) {
	ds := daemonset.New(rp.podTemplate, rp.name, rp.labels, rp.owner, rp.selectors)

	if err := controllerutil.SetControllerReference(rp.owner, &ds, scheme.Scheme); err != nil {
		return 0, 0, err
	}

	reconciled, err := daemonset.Reconcile(rp.client, ds, rp.owner)
	if err != nil {
		return 0, 0, err
	}

	return reconciled.Status.NumberReady, reconciled.Status.DesiredNumberScheduled, nil
}
