// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/events"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

type Type string

type PodTemplateFunc func(nsName types.NamespacedName, builder *defaults.PodTemplateBuilder)

type Preset struct {
	PodTemplateFunc PodTemplateFunc
	RoleNames       []string
	Config          *settings.CanonicalConfig
}

type Driver interface {
	Reconcile() *reconciler.Results
}

type DriverParams struct {
	Client   k8s.Client
	Context  context.Context
	Logger   logr.Logger
	Recorder record.EventRecorder

	Beat beatv1beta1.Beat
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

func validateBeatSpec(spec beatv1beta1.BeatSpec) error {
	if (spec.DaemonSet == nil && spec.Deployment == nil) || (spec.DaemonSet != nil && spec.Deployment != nil) {
		return fmt.Errorf("exactly one of daemonset, deployment has to be specified")
	}
	return nil
}

func Reconcile(
	defaultImage container.Image,
	params DriverParams,
	preset Preset,
) *reconciler.Results {
	results := reconciler.NewResult(params.Context)

	if err := validateBeatSpec(params.Beat.Spec); err != nil {
		k8s.EmitErrorEvent(params.Recorder, err, &params.Beat, events.EventReasonValidation, err.Error())
		return results.WithError(err)
	}

	cfgBytes, err := buildBeatConfig(params.Logger, params.Client, params.Beat, preset.Config)
	if err != nil {
		return results.WithError(err)
	}
	configHash := sha256.New224()
	_, _ = configHash.Write(cfgBytes)

	if err := reconcileRBAC(cfgBytes, preset.RoleNames, params); err != nil {
		results.WithError(err)
	}

	if err := reconcileConfig(cfgBytes, params); err != nil {
		return results.WithError(err)
	}

	// we need to deref the secret here (if any) to include it in the configHash otherwise Beat will not be rolled on content changes
	if err := association.WriteAssocToConfigHash(params.Client, &params.Beat, configHash); err != nil {
		results.WithError(err)
	}

	podTemplate := buildPodTemplate(params, defaultImage, preset.PodTemplateFunc, configHash)
	results.WithResults(reconcilePodVehicle(podTemplate, params))
	return results
}
