// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package enterprisesearch

import (
	"context"

	"go.elastic.co/apm"
	appsv1 "k8s.io/api/apps/v1"

	entv1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
)

func (r *ReconcileEnterpriseSearch) reconcileDeployment(
	ctx context.Context,
	ent entv1.EnterpriseSearch,
	configHash string,
) (appsv1.Deployment, error) {
	span, _ := apm.StartSpan(ctx, "reconcile_deployment", tracing.SpanTypeApp)
	defer span.End()

	deployParams, err := r.deploymentParams(ent, configHash)
	if err != nil {
		return appsv1.Deployment{}, err
	}
	deploy := deployment.New(deployParams)
	return deployment.Reconcile(r.K8sClient(), deploy, &ent)
}

func (r *ReconcileEnterpriseSearch) deploymentParams(ent entv1.EnterpriseSearch, configHash string) (deployment.Params, error) {
	podSpec, err := newPodSpec(ent, configHash)
	if err != nil {
		return deployment.Params{}, err
	}

	deploymentLabels := Labels(ent.Name)

	podLabels := maps.Merge(Labels(ent.Name), VersionLabels(ent))
	// merge with user-provided labels
	podSpec.Labels = maps.MergePreservingExistingKeys(podSpec.Labels, podLabels)

	return deployment.Params{
		Name:            DeploymentName(ent.Name),
		Namespace:       ent.Namespace,
		Replicas:        ent.Spec.Count,
		Selector:        deploymentLabels,
		Labels:          deploymentLabels,
		PodTemplateSpec: podSpec,
		Strategy:        appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType},
	}, nil
}
