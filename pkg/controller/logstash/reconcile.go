// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"fmt"
	"reflect"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/pkg/errors"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/labels"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/sset"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/volume"

	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

func reconcileStatefulSet(params Params, podTemplate corev1.PodTemplateSpec) (*reconciler.Results, logstashv1alpha1.LogstashStatus) {
	defer tracing.Span(&params.Context)()
	results := reconciler.NewResult(params.Context)

	ok, _, err := params.expectationsSatisfied(params.Context)
	if err != nil {
		return results.WithError(err), params.Status
	}

	if !ok {
		return results.WithResult(reconcile.Result{Requeue: true}), params.Status
	}

	expected := sset.New(sset.Params{
		Name:                 logstashv1alpha1.Name(params.Logstash.Name),
		Namespace:            params.Logstash.Namespace,
		ServiceName:          logstashv1alpha1.APIServiceName(params.Logstash.Name),
		Selector:             params.Logstash.GetIdentityLabels(),
		Labels:               params.Logstash.GetIdentityLabels(),
		PodTemplateSpec:      podTemplate,
		Replicas:             params.Logstash.Spec.Count,
		RevisionHistoryLimit: params.Logstash.Spec.RevisionHistoryLimit,
		UpdateStrategy:       params.Logstash.Spec.UpdateStrategy,
		VolumeClaimTemplates: params.Logstash.Spec.VolumeClaimTemplates,
	})

	recreations, err := volume.RecreateStatefulSets(params.Context, params.Client, params.Logstash)

	if err != nil {
		if apierrors.IsConflict(err) {
			ulog.FromContext(params.Context).V(1).Info("Conflict while recreating stateful set, requeueing", "message", err)
			return results.WithResult(reconcile.Result{Requeue: true}), params.Status
		}
		return results.WithError(fmt.Errorf("StatefulSet recreation: %w", err)), params.Status
	}

	if recreations > 0 {
		// Statefulset is in the process of being recreated to handle PVC expansion:
		// it is safer to requeue until the re-creation is done.
		// Otherwise, some operation could be performed with wrong assumptions:
		// the sset doesn't exist (was just deleted), but the Pods do actually exist.
		ulog.FromContext(params.Context).V(1).Info("StatefulSets recreation in progress, re-queueing after 30 seconds.", "namespace", params.Logstash.Namespace, "ls_name", params.Logstash.Name,
			"status", params.Status)
		return results.WithResult(reconcile.Result{RequeueAfter: 30 * time.Second}), params.Status
	}

	actualStatefulSet, err := retrieveActualStatefulSet(params.Client, params.Logstash)

	notFound := apierrors.IsNotFound(err)
	if err != nil && !notFound {
		return results.WithError(err), params.Status
	}

	if !notFound {
		recreateSset, err := volume.HandleVolumeExpansion(params.Context, params.Client, params.Logstash, expected, actualStatefulSet, true)
		if err != nil {
			return results.WithError(err), params.Status
		}
		if recreateSset {
			return results.WithResult(reconcile.Result{Requeue: true}), params.Status
		}
	}

	if err := controllerutil.SetControllerReference(&params.Logstash, &expected, scheme.Scheme); err != nil {
		return results.WithError(err), params.Status
	}
	reconciled, err := sset.Reconcile(params.Context, params.Client, expected, params.Logstash, params.Expectations)

	if err != nil {
		return results.WithError(err), params.Status
	}

	var status logstashv1alpha1.LogstashStatus

	if status, err = calculateStatus(&params, reconciled); err != nil {
		results.WithError(errors.Wrap(err, "while calculating status"))
	}
	return results, status
}

// calculateStatus will calculate a new status from the state of the pods within the k8s cluster
// and will return any error encountered.
func calculateStatus(params *Params, sset appsv1.StatefulSet) (logstashv1alpha1.LogstashStatus, error) {
	logstash := params.Logstash
	status := params.Status
	pods, err := k8s.PodsMatchingLabels(params.Client, logstash.Namespace, map[string]string{labels.NameLabelName: logstash.Name})
	if err != nil {
		return status, err
	}

	if sset.Spec.Selector != nil {
		selector, err := metav1.LabelSelectorAsSelector(sset.Spec.Selector)
		if err != nil {
			return logstashv1alpha1.LogstashStatus{}, err
		}
		status.Selector = selector.String()
	}
	status.Version = common.LowestVersionFromPods(params.Context, status.Version, pods, VersionLabelName)
	status.AvailableNodes = sset.Status.ReadyReplicas
	status.ExpectedNodes = sset.Status.Replicas

	health, err := CalculateHealth(logstash.GetAssociations(), status.AvailableNodes, status.ExpectedNodes)
	if err != nil {
		return status, err
	}
	status.Health = health

	return status, nil
}

// updateStatus will update the Elastic Logstash's status within the k8s cluster, using the given Elastic Logstash and status.
func updateStatus(ctx context.Context, logstash logstashv1alpha1.Logstash, client client.Client, status logstashv1alpha1.LogstashStatus) error {
	if reflect.DeepEqual(logstash.Status, status) {
		return nil
	}
	logstash.Status = status
	return common.UpdateStatus(ctx, client, &logstash)
}

// retrieveActualStatefulSet returns the StatefulSet for the given ls cluster.
func retrieveActualStatefulSet(c k8s.Client, ls logstashv1alpha1.Logstash) (appsv1.StatefulSet, error) {
	var sset appsv1.StatefulSet
	err := c.Get(context.Background(), types.NamespacedName{Name: logstashv1alpha1.Name(ls.Name), Namespace: ls.Namespace}, &sset)
	if err != nil {
		return appsv1.StatefulSet{}, err
	}

	return sset, nil
}
