// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"reflect"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/daemonset"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
)

func reconcilePodVehicle(podTemplate corev1.PodTemplateSpec, params DriverParams) *reconciler.Results {
	results := reconciler.NewResult(params.Context)
	spec := params.Beat.Spec
	name := Name(params.Beat.Name, spec.Type)

	var toDelete client.Object
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
	if err := params.Client.Get(context.Background(), types.NamespacedName{
		Namespace: params.Beat.Namespace,
		Name:      name,
	}, toDelete); err == nil {
		results.WithError(params.Client.Delete(context.Background(), toDelete))
	} else if !apierrors.IsNotFound(err) {
		results.WithError(err)
	}

	err = calculateStatus(params, ready, desired, params.Status)
	if err != nil {
		params.Logger.V(1).Info(
			"Error while calculating new status",
			"namespace", params.Beat.Namespace,
			"beat_name", params.Beat.Name)
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
		Strategy:        rp.beat.Spec.Deployment.Strategy,
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
		Strategy:    rp.beat.Spec.DaemonSet.UpdateStrategy,
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

// calculateStatus will calculate a new status from the state of the pods within the k8s cluster
// and will return any error encountered.
func calculateStatus(params DriverParams, ready, desired int32, status *beatv1beta1.BeatStatus) error {
	beat := params.Beat

	pods, err := k8s.PodsMatchingLabels(params.K8sClient(), beat.Namespace, map[string]string{NameLabelName: beat.Name})
	if err != nil {
		return err
	}
	status.AvailableNodes = ready
	status.ExpectedNodes = desired
	status.Health = CalculateHealth(beat.GetAssociations(), ready, desired)
	status.Version = common.LowestVersionFromPods(beat.Status.Version, pods, VersionLabelName)

	return nil
}

// UpdateStatus will update the Elastic Beat's status within the k8s cluster, using the given Elastic Beat and status.
func UpdateStatus(beat beatv1beta1.Beat, client client.Client, status *beatv1beta1.BeatStatus) error {
	if status == nil || reflect.DeepEqual(beat.Status, *status) {
		return nil
	}
	beat.Status = *status
	return common.UpdateStatus(client, &beat)
}
