// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"errors"
	"hash/fnv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/association"
	beat_stackmon "github.com/elastic/cloud-on-k8s/v2/pkg/controller/beat/common/stackmon"
	commonassociation "github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/driver"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/watches"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
	ulog "github.com/elastic/cloud-on-k8s/v2/pkg/utils/log"
)

type Type string

type Driver interface {
	Reconcile() (*reconciler.Results, *beatv1beta1.BeatStatus)
}

type DriverParams struct {
	Context context.Context

	Client        k8s.Client
	EventRecorder record.EventRecorder
	Watches       watches.DynamicWatches

	Status *beatv1beta1.BeatStatus
	Beat   beatv1beta1.Beat
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

	assocAllowed, err := association.AllowVersion(beatVersion, &params.Beat, ulog.FromContext(params.Context), params.Recorder())
	if err != nil {
		return results.WithError(err), params.Status
	}
	if !assocAllowed {
		return results, params.Status // will eventually retry
	}

	configHash := fnv.New32a()
	if err := reconcileConfig(params, managedConfig, configHash); err != nil {
		return results.WithError(err), params.Status
	}

	// we need to deref the secret here (if any) to include it in the configHash otherwise Beat will not be rolled on content changes
	if err := commonassociation.WriteAssocsToConfigHash(params.Client, params.Beat.GetAssociations(), configHash); err != nil {
		return results.WithError(err), params.Status
	}

	podTemplate, err := buildPodTemplate(params, defaultImage, configHash)
	if err != nil {
		if errors.Is(err, beat_stackmon.ErrMonitoringClusterUUIDUnavailable) {
			results.WithReconciliationState(reconciler.RequeueAfter(10 * time.Second).WithReason("ElasticsearchRef UUID unavailable while configuring Beats stack monitoring"))
		} else {
			results.WithError(err)
		}
		return results, params.Status
	}
	var reconcileResults *reconciler.Results
	reconcileResults, params.Status = reconcilePodVehicle(podTemplate, params)
	results.WithResults(reconcileResults)
	return results, params.Status
}
