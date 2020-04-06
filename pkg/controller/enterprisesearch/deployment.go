// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package enterprisesearch

import (
	"context"

	"go.elastic.co/apm"
	appsv1 "k8s.io/api/apps/v1"

	entsv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/enterprisesearch/v1beta1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/deployment"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	entsname "github.com/elastic/cloud-on-k8s/pkg/controller/enterprisesearch/name"
	"github.com/elastic/cloud-on-k8s/pkg/utils/maps"
)

func (r *ReconcileEnterpriseSearch) reconcileDeployment(
	ctx context.Context,
	state State,
	ents entsv1beta1.EnterpriseSearch,
	configHash string,
) (State, error) {
	span, _ := apm.StartSpan(ctx, "reconcile_deployment", tracing.SpanTypeApp)
	defer span.End()

	deploy := deployment.New(r.deploymentParams(ents, configHash))
	result, err := deployment.Reconcile(r.K8sClient(), deploy, &ents)
	if err != nil {
		return state, err
	}
	state.UpdateEnterpriseSearchState(result)
	return state, nil
}

func (r *ReconcileEnterpriseSearch) deploymentParams(ents entsv1beta1.EnterpriseSearch, configHash string) deployment.Params {
	podSpec := newPodSpec(ents, configHash)

	deploymentLabels := Labels(ents.Name)

	podLabels := maps.Merge(Labels(ents.Name), VersionLabels(ents))
	// merge with user-provided labels
	podSpec.Labels = maps.MergePreservingExistingKeys(podSpec.Labels, podLabels)

	return deployment.Params{
		Name:            entsname.Deployment(ents.Name),
		Namespace:       ents.Namespace,
		Replicas:        ents.Spec.Count,
		Selector:        deploymentLabels,
		Labels:          deploymentLabels,
		PodTemplateSpec: podSpec,
		Strategy:        appsv1.RollingUpdateDeploymentStrategyType,
	}
}
