// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"reflect"

	pkgerrors "github.com/pkg/errors"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/daemonset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/pointer"
)

func reconcilePodVehicle(podTemplate corev1.PodTemplateSpec, params DriverParams) (*reconciler.Results, *beatv1beta1.BeatStatus) {
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
		ctx:         params.Context,
		client:      params.Client,
		beat:        params.Beat,
		podTemplate: podTemplate,
	})
	if err != nil {
		return results.WithError(err), params.Status
	}

	// clean up the other one
	if err := params.Client.Get(params.Context, types.NamespacedName{
		Namespace: params.Beat.Namespace,
		Name:      name,
	}, toDelete); err == nil {
		results.WithError(params.Client.Delete(params.Context, toDelete))
	} else if !apierrors.IsNotFound(err) {
		results.WithError(err)
	}

	params.Status, err = newStatus(params, ready, desired)
	if err != nil {
		err = pkgerrors.Wrapf(err, "while updating status")
	}

	return results.WithError(err), params.Status
}

type ReconciliationParams struct {
	ctx         context.Context
	client      k8s.Client
	beat        beatv1beta1.Beat
	podTemplate corev1.PodTemplateSpec
}

func reconcileDeployment(rp ReconciliationParams) (int32, int32, error) {
	d := deployment.New(deployment.Params{
		Name:                 Name(rp.beat.Name, rp.beat.Spec.Type),
		Namespace:            rp.beat.Namespace,
		Selector:             rp.beat.GetIdentityLabels(),
		Labels:               rp.beat.GetIdentityLabels(),
		PodTemplateSpec:      rp.podTemplate,
		RevisionHistoryLimit: rp.beat.Spec.RevisionHistoryLimit,
		Replicas:             pointer.Int32OrDefault(rp.beat.Spec.Deployment.Replicas, int32(1)),
		Strategy:             rp.beat.Spec.Deployment.Strategy,
	})
	if err := controllerutil.SetControllerReference(&rp.beat, &d, scheme.Scheme); err != nil {
		return 0, 0, err
	}

	reconciled, err := deployment.Reconcile(rp.ctx, rp.client, d, &rp.beat)
	if err != nil {
		return 0, 0, err
	}

	return reconciled.Status.ReadyReplicas, reconciled.Status.Replicas, nil
}

func reconcileDaemonSet(rp ReconciliationParams) (int32, int32, error) {
	ds := daemonset.New(daemonset.Params{
		PodTemplate:          rp.podTemplate,
		Name:                 Name(rp.beat.Name, rp.beat.Spec.Type),
		Owner:                &rp.beat,
		Labels:               rp.beat.GetIdentityLabels(),
		RevisionHistoryLimit: rp.beat.Spec.RevisionHistoryLimit,
		Selectors:            rp.beat.GetIdentityLabels(),
		Strategy:             rp.beat.Spec.DaemonSet.UpdateStrategy,
	})

	if err := controllerutil.SetControllerReference(&rp.beat, &ds, scheme.Scheme); err != nil {
		return 0, 0, err
	}

	reconciled, err := daemonset.Reconcile(rp.ctx, rp.client, ds, &rp.beat)
	if err != nil {
		return 0, 0, err
	}

	return reconciled.Status.NumberReady, reconciled.Status.DesiredNumberScheduled, nil
}

// newStatus will calculate a new status from the state of the pods within the k8s cluster
// and returns any error encountered.
func newStatus(params DriverParams, ready, desired int32) (*beatv1beta1.BeatStatus, error) {
	beat := params.Beat
	status := params.Status

	pods, err := k8s.PodsMatchingLabels(params.K8sClient(), beat.Namespace, map[string]string{NameLabelName: beat.Name})
	if err != nil {
		return status, err
	}
	status.Version = common.LowestVersionFromPods(params.Context, beat.Status.Version, pods, VersionLabelName)
	status.AvailableNodes = ready
	status.ExpectedNodes = desired
	status.Health, err = calculateHealth(beat.GetAssociations(), ready, desired)
	if err != nil {
		return status, err
	}

	return status, nil
}

// UpdateStatus will update the Elastic Beat's status within the k8s cluster, using the given Elastic Beat and status.
func UpdateStatus(ctx context.Context, beat beatv1beta1.Beat, client client.Client, status *beatv1beta1.BeatStatus) error {
	if status == nil || reflect.DeepEqual(beat.Status, *status) {
		return nil
	}
	beat.Status = *status
	return common.UpdateStatus(ctx, client, &beat)
}
