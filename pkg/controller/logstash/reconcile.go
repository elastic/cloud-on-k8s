// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package logstash

import (
	"context"
	"reflect"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/logstash/sset"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/pkg/errors"

	logstashv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func reconcileStatefulSet(params Params, podTemplate corev1.PodTemplateSpec) (*reconciler.Results, logstashv1alpha1.LogstashStatus) {
	defer tracing.Span(&params.Context)()
	results := reconciler.NewResult(params.Context)

	s := sset.New(sset.Params{
		Name:                 logstashv1alpha1.Name(params.Logstash.Name),
		Namespace:            params.Logstash.Namespace,
		ServiceName:          logstashv1alpha1.HTTPServiceName(params.Logstash.Name),
		Selector:             params.Logstash.GetIdentityLabels(),
		Labels:               params.Logstash.GetIdentityLabels(),
		PodTemplateSpec:      podTemplate,
		Replicas:             params.Logstash.Spec.Count,
		RevisionHistoryLimit: params.Logstash.Spec.RevisionHistoryLimit,
	})
	if err := controllerutil.SetControllerReference(&params.Logstash, &s, scheme.Scheme); err != nil {
		return results.WithError(err), params.Status
	}

	reconciled, err := sset.Reconcile(params.Context, params.Client, s, &params.Logstash)
	if err != nil {
		return results.WithError(err), params.Status
	}

	var status logstashv1alpha1.LogstashStatus
	if status, err = calculateStatus(&params, reconciled.Status.ReadyReplicas, reconciled.Status.Replicas); err != nil {
		err = errors.Wrap(err, "while calculating status")
	}

	return results.WithError(err), status
}

// calculateStatus will calculate a new status from the state of the pods within the k8s cluster
// and will return any error encountered.
func calculateStatus(params *Params, ready, desired int32) (logstashv1alpha1.LogstashStatus, error) {
	logstash := params.Logstash
	status := params.Status

	pods, err := k8s.PodsMatchingLabels(params.Client, logstash.Namespace, map[string]string{NameLabelName: logstash.Name})
	if err != nil {
		return status, err
	}

	status.Version = common.LowestVersionFromPods(params.Context, status.Version, pods, VersionLabelName)
	status.AvailableNodes = ready
	status.ExpectedNodes = desired
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
