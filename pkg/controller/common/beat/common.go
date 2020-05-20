// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/beat/health"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/reconciler"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
)

type Type string

type Driver interface {
	Reconcile() DriverResults
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

type DriverResults struct {
	*reconciler.Results
	Status *DriverStatus
}

func NewDriverResults(ctx context.Context) DriverResults {
	return DriverResults{
		Status:  nil,
		Results: reconciler.NewResult(ctx),
	}
}

type DriverStatus struct {
	ExpectedNodes  int32
	AvailableNodes int32
	Health         health.BeatHealth
	Association    commonv1.AssociationStatus
}
