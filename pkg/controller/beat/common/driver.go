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

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	commonassociation "github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

type Type string

type Driver interface {
	Reconcile() *reconciler.Results
}

type DriverParams struct {
	Client  k8s.Client
	Context context.Context
	Logger  logr.Logger

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

func ValidateBeatSpec(spec beatv1beta1.BeatSpec) error {
	if (spec.DaemonSet == nil && spec.Deployment == nil) || (spec.DaemonSet != nil && spec.Deployment != nil) {
		return fmt.Errorf("either daemonset or deployment has to be specified")
	}
	return nil
}

func Reconcile(
	params DriverParams,
	defaultConfig *settings.CanonicalConfig,
	defaultImage container.Image,
	modifyPodFunc func(builder *defaults.PodTemplateBuilder),
) *reconciler.Results {
	results := reconciler.NewResult(params.Context)

	if err := ValidateBeatSpec(params.Beat.Spec); err != nil {
		return results.WithError(err)
	}

	if err := ReconcileAutodiscoverRBAC(params.Context, params.Logger, params.Client, params.Beat); err != nil {
		results.WithError(err)
	}

	configHash := sha256.New224()
	if err := reconcileConfig(params, defaultConfig, configHash); err != nil {
		return results.WithError(err)
	}

	// we need to deref the secret here (if any) to include it in the configHash otherwise Beat will not be rolled on content changes
	if err := commonassociation.WriteAssocSecretToConfigHash(params.Client, &params.Beat, configHash); err != nil {
		return results.WithError(err)
	}

	podTemplate := buildPodTemplate(params, defaultImage, modifyPodFunc, configHash)
	results.WithResults(reconcilePodVehicle(podTemplate, params))
	return results
}
