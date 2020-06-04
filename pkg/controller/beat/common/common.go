// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"context"
	"crypto/sha256"
	"fmt"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	commonassociation "github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

type Type string

type Driver interface {
	Reconcile() (*DriverStatus, *reconciler.Results)
}

type DriverParams struct {
	Client  k8s.Client
	Context context.Context
	Logger  logr.Logger

	Beat beatv1beta1.Beat
}

func (dp *DriverParams) GetReplicas() *int32 {
	if dp.Beat.Spec.Deployment == nil {
		return nil
	}

	return dp.Beat.Spec.Deployment.Replicas
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

func (dp *DriverParams) Validate() error {
	spec := dp.Beat.Spec
	if (spec.DaemonSet == nil && spec.Deployment == nil) || (spec.DaemonSet != nil && spec.Deployment != nil) {
		return fmt.Errorf("either daemonset or deployment has to be specified")
	}
	return nil
}

type DriverStatus struct {
	ExpectedNodes  int32
	AvailableNodes int32
	Health         beatv1beta1.BeatHealth
	Association    commonv1.AssociationStatus
}

func Reconcile(
	params DriverParams,
	defaultConfig *settings.CanonicalConfig,
	defaultImage container.Image,
	modifyPodFunc func(builder *defaults.PodTemplateBuilder)) (*DriverStatus, *reconciler.Results) {
	results := reconciler.NewResult(params.Context)

	if err := params.Validate(); err != nil {
		return nil, results.WithError(err)
	}

	if err := ReconcileAutodiscoverRBAC(params.Context, params.Logger, params.Client, params.Beat); err != nil {
		results.WithError(err)
	}

	configHash := sha256.New224()
	if err := reconcileConfig(params, defaultConfig, configHash); err != nil {
		return nil, results.WithError(err)
	}

	// we need to deref the secret here (if any) to include it in the configHash otherwise Beat will not be rolled on content changes
	if err := commonassociation.WriteAssocSecretToConfigHash(params.Client, &params.Beat, configHash); err != nil {
		return nil, results.WithError(err)
	}

	podTemplate := buildPodTemplate(params, defaultImage, modifyPodFunc, configHash)
	driverStatus, err := reconcilePodVehicle(podTemplate, params)
	if err != nil {
		if apierrors.IsConflict(err) {
			params.Logger.V(1).Info("Conflict while updating")
			return nil, results.WithResult(reconcile.Result{Requeue: true})
		}
		return nil, results.WithError(err)
	}

	return &driverStatus, results
}
