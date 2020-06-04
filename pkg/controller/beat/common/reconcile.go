// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/daemonset"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
)

func reconcilePodVehicle(podTemplate corev1.PodTemplateSpec, params DriverParams) (DriverStatus, error) {
	var reconciliationFunc func(params ReconciliationParams) (int32, int32, error)

	spec := params.Beat.Spec
	name := Name(spec.Type, params.Beat.Name)
	var toDelete runtime.Object
	switch {
	case spec.DaemonSet != nil:
		reconciliationFunc = reconcileDaemonSet
		toDelete = &v1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: params.Beat.Namespace,
			},
		}
	case spec.Deployment != nil:
		reconciliationFunc = reconcileDeployment
		toDelete = &v1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: params.Beat.Namespace,
			},
		}
	}

	ready, desired, err := reconciliationFunc(ReconciliationParams{
		client: params.Client,
		beat:   params.Beat,
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
		Health:         CalculateHealth(&params.Beat, ready, desired),
		Association:    params.Beat.AssociationStatus(),
	}, nil
}

type ReconciliationParams struct {
	client k8s.Client
	beat   beatv1beta1.Beat
}

func reconcileDeployment(rp ReconciliationParams) (int32, int32, error) {
	d := deployment.New(deployment.Params{
		Name:            rp.beat.Name,
		Namespace:       rp.beat.Namespace,
		Selector:        NewLabels(rp.beat),
		Labels:          NewLabels(rp.beat),
		PodTemplateSpec: rp.beat.Spec.Deployment.PodTemplate,
		Replicas:        pointer.Int32OrDefault(rp.beat.Spec.Deployment.Replicas, int32(1)),
	})
	if err := controllerutil.SetControllerReference(&rp.beat, &d, scheme.Scheme); err != nil {
		return 0, 0, err
	}

	reconciled, err := deployment.Reconcile(rp.client, d, &rp.beat)
	if err != nil {
		return 0, 0, err
	}

	return reconciled.Status.ReadyReplicas, reconciled.Status.Replicas, nil
}

func reconcileDaemonSet(rp ReconciliationParams) (int32, int32, error) {
	ds := daemonset.New(rp.beat.Spec.DaemonSet.PodTemplate, rp.beat.Name, NewLabels(rp.beat), &rp.beat, NewLabels(rp.beat))

	if err := controllerutil.SetControllerReference(&rp.beat, &ds, scheme.Scheme); err != nil {
		return 0, 0, err
	}

	reconciled, err := daemonset.Reconcile(rp.client, ds, &rp.beat)
	if err != nil {
		return 0, 0, err
	}

	return reconciled.Status.NumberReady, reconciled.Status.DesiredNumberScheduled, nil
}
