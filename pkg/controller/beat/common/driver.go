// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"hash/fnv"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/association"
	commonassociation "github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

type Type string

type Driver interface {
	Reconcile() (*reconciler.Results, *beatv1beta1.BeatStatus)
}

type DriverParams struct {
	Context context.Context
	Logger  logr.Logger

	Client        k8s.Client
	EventRecorder record.EventRecorder
	Watches       watches.DynamicWatches

	Status *beatv1beta1.BeatStatus
	Beat   beatv1beta1.Beat

	monitoringAssociationConnectionInfo *association.UnmanagedAssociationConnectionInfo
}

func (dp DriverParams) K8sClient() k8s.Client {
	return dp.Client
}

func (dp DriverParams) Recorder() record.EventRecorder {
	return dp.EventRecorder
}

func (dp DriverParams) DynamicWatches() watches.DynamicWatches {
	return dp.Watches
}

func (dp *DriverParams) GetPodTemplate() corev1.PodTemplateSpec {
	spec := dp.Beat.Spec
	switch {
	case spec.DaemonSet != nil:
		return spec.DaemonSet.PodTemplate
	case spec.Deployment != nil:
		return spec.Deployment.PodTemplate
	}

	return corev1.PodTemplateSpec{}
}

var _ driver.Interface = DriverParams{}

func Reconcile(
	params DriverParams,
	managedConfig *settings.CanonicalConfig,
	defaultImage container.Image,
) (*reconciler.Results, *beatv1beta1.BeatStatus) {
	results := reconciler.NewResult(params.Context)

	beatVersion, err := version.Parse(params.Beat.Spec.Version)
	if err != nil {
		return results.WithError(err), params.Status
	}

	assocAllowed, err := association.AllowVersion(beatVersion, &params.Beat, params.Logger, params.Recorder())
	if err != nil {
		return results.WithError(err), params.Status
	}
	if !assocAllowed {
		return results, params.Status // will eventually retry
	}

	configHash := fnv.New32a()
	params.Logger.Info("about to reconcileConfig")
	if err := reconcileConfig(params, managedConfig, configHash); err != nil {
		return results.WithError(err), params.Status
	}
	params.Logger.Info("end about to reconcileConfig")

	params.Logger.Info("about to WriteAssocsToConfigHash")
	// we need to deref the secret here (if any) to include it in the configHash otherwise Beat will not be rolled on content changes
	if err := commonassociation.WriteAssocsToConfigHash(params.Client, params.Beat.GetAssociations(), configHash); err != nil {
		return results.WithError(err), params.Status
	}
	params.Logger.Info("end about to WriteAssocsToConfigHash")

	params.Logger.Info("about to buildPodTemplate")
	podTemplate, err := buildPodTemplate(params, defaultImage, configHash)
	if err != nil {
		return results.WithError(err), params.Status
	}
	params.Logger.Info("end about to buildPodTemplate")
	var reconcileResults *reconciler.Results
	params.Logger.Info("about to reconcilePodVehicle")
	reconcileResults, params.Status = reconcilePodVehicle(podTemplate, params)
	params.Logger.Info("end about to reconcilePodVehicle")
	results.WithResults(reconcileResults)
	return results, params.Status
}
