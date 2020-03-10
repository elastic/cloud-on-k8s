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

	params, err := r.deploymentParams(ents, configHash)
	if err != nil {
		return state, err
	}

	deploy := deployment.New(params)
	result, err := deployment.Reconcile(r.K8sClient(), r.Scheme(), deploy, &ents)
	if err != nil {
		return state, err
	}
	state.UpdateEnterpriseSearchState(result)
	return state, nil
}


func (r *ReconcileEnterpriseSearch) deploymentParams(ents entsv1beta1.EnterpriseSearch, configHash string) (deployment.Params, error) {
	podSpec, err := newPodSpec(ents, configHash)
	if err != nil {
		return deployment.Params{}, err
	}
	podLabels := NewLabels(ents.Name)
	podSpec.Labels = maps.MergePreservingExistingKeys(podSpec.Labels, podLabels)

	return deployment.Params{
		Name:            entsname.Deployment(ents.Name),
		Namespace:       ents.Namespace,
		Replicas:        ents.Spec.Count,
		Selector:        NewLabels(ents.Name),
		Labels:          NewLabels(ents.Name),
		PodTemplateSpec: podSpec,
		Strategy:        appsv1.RollingUpdateDeploymentStrategyType,
	}, nil
}

