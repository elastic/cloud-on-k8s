// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/daemonset"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
)

func reconcilePodVehicle(podTemplate corev1.PodTemplateSpec, params DriverParams) *reconciler.Results {
	results := reconciler.NewResult(params.Context)
	if err := ValidateBeatSpec(params.Beat.Spec); err != nil {
		return results.WithError(err)
	}

	spec := params.Beat.Spec
	name := Name(params.Beat.Name, spec.Type)

	var toDelete runtime.Object
	var reconciliationFunc func(params ReconciliationParams) (int32, int32, error)
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
		client:      params.Client,
		beat:        params.Beat,
		podTemplate: podTemplate,
	})
	if err != nil {
		return results.WithError(err)
	}

	// clean up the other one
	if err := params.Client.Delete(toDelete); err != nil && !apierrors.IsNotFound(err) {
		return results.WithError(err)
	}

	err = updateStatus(params, ready, desired)
	if err != nil && apierrors.IsConflict(err) {
		params.Logger.V(1).Info(
			"Conflict while updating status",
			"namespace", params.Beat.Namespace,
			"beat_name", params.Beat.Name)
		return results.WithResult(reconcile.Result{Requeue: true})
	}

	return results.WithError(err)
}

type ReconciliationParams struct {
	client      k8s.Client
	beat        beatv1beta1.Beat
	podTemplate corev1.PodTemplateSpec
}

func reconcileDeployment(rp ReconciliationParams) (int32, int32, error) {
	d := deployment.New(deployment.Params{
		Name:            Name(rp.beat.Name, rp.beat.Spec.Type),
		Namespace:       rp.beat.Namespace,
		Selector:        NewLabels(rp.beat),
		Labels:          NewLabels(rp.beat),
		PodTemplateSpec: rp.podTemplate,
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
	ds := daemonset.New(daemonset.Params{
		PodTemplate: rp.podTemplate,
		Name:        Name(rp.beat.Name, rp.beat.Spec.Type),
		Owner:       &rp.beat,
		Labels:      NewLabels(rp.beat),
		Selectors:   NewLabels(rp.beat),
	})

	if err := controllerutil.SetControllerReference(&rp.beat, &ds, scheme.Scheme); err != nil {
		return 0, 0, err
	}

	reconciled, err := daemonset.Reconcile(rp.client, ds, &rp.beat)
	if err != nil {
		return 0, 0, err
	}

	return reconciled.Status.NumberReady, reconciled.Status.DesiredNumberScheduled, nil
}

func updateStatus(params DriverParams, ready, desired int32) error {
	beat := params.Beat

	beat.Status.AvailableNodes = ready
	beat.Status.ExpectedNodes = desired
	beat.Status.Health = CalculateHealth(&beat, ready, desired)
	beat.Status.Association = beat.AssociationStatus()

	return params.Client.Status().Update(&beat)
}
