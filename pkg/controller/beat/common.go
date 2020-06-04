// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"context"
	"crypto/sha256"
	"fmt"

	commonassociation "github.com/elastic/cloud-on-k8s/pkg/controller/common/association"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/beat/health"
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

type DaemonSetSpec struct {
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`
}

type DeploymentSpec struct {
	PodTemplate corev1.PodTemplateSpec `json:"podTemplate,omitempty"`
	Replicas    *int32                 `json:"replicas,omitempty"`
}

type DriverParams struct {
	Client  k8s.Client
	Context context.Context
	Logger  logr.Logger

	Owner      metav1.Object
	Associated commonv1.Associated
	Namer      Namer

	Type               string
	Version            string
	ElasticsearchRef   commonv1.ObjectSelector
	Image              string
	Config             *commonv1.Config
	ServiceAccountName string

	Labels map[string]string

	DaemonSet  *DaemonSetSpec
	Deployment *DeploymentSpec
	Selectors  map[string]string
}

func (dp *DriverParams) GetReplicas() *int32 {
	if dp.Deployment == nil {
		return nil
	}

	return dp.Deployment.Replicas
}

func (dp *DriverParams) GetPodTemplate() corev1.PodTemplateSpec {
	switch {
	case dp.DaemonSet != nil:
		return dp.DaemonSet.PodTemplate
	case dp.Deployment != nil:
		return dp.Deployment.PodTemplate
	}

	return corev1.PodTemplateSpec{}
}

func (dp *DriverParams) Validate() error {
	if (dp.DaemonSet == nil && dp.Deployment == nil) || (dp.DaemonSet != nil && dp.Deployment != nil) {
		return fmt.Errorf("either daemonset or deployment has to be specified")
	}
	return nil
}

type DriverStatus struct {
	ExpectedNodes  int32
	AvailableNodes int32
	Health         health.BeatHealth
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

	if err := SetupAutodiscoverRBAC(params.Context, params.Logger, params.Client, params.Owner, params.Labels); err != nil {
		results.WithError(err)
	}

	configHash := sha256.New224()
	if err := reconcileConfig(params, defaultConfig, configHash); err != nil {
		return nil, results.WithError(err)
	}

	// we need to deref the secret here (if any) to include it in the configHash otherwise Beat will not be rolled on content changes
	if err := commonassociation.WriteAssocSecretToConfigHash(params.Client, params.Associated, configHash); err != nil {
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
