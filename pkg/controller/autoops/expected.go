package autoops

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	autoopsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/autoops/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/container"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	common_name "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/name"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/pointer"
)

var (
	// ESNAutoOpsNamer is a Namer that generates names for AutoOps deployments
	// according to the associated Elasticsearch cluster name.
	AutoOpsNamer    = common_name.NewNamer("autoops")
	basePodTemplate = corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "autoops-agent",
				},
			},
		},
	}
)

type ExpectedResources struct {
	deployment appsv1.Deployment
}

func (r *ReconcileAutoOpsAgentPolicy) generateExpectedResources(ctx context.Context, autoops autoopsv1alpha1.AutoOpsAgentPolicy, es esv1.Elasticsearch) (ExpectedResources, error) {
	deployment, err := r.deploymentParams(autoops, es)
	if err != nil {
		return ExpectedResources{}, err
	}
	return ExpectedResources{
		deployment: deployment,
	}, nil
}

func (r *ReconcileAutoOpsAgentPolicy) deploymentParams(autoops autoopsv1alpha1.AutoOpsAgentPolicy, es esv1.Elasticsearch) (appsv1.Deployment, error) {
	var deployment appsv1.Deployment
	v, err := version.Parse(autoops.Spec.Version)
	if err != nil {
		return appsv1.Deployment{}, err
	}
	labels := map[string]string{
		commonv1.TypeLabelName:        "autoops-agent",
		"autoops.k8s.elastic.co/name": autoops.Name,
	}
	deployment.ObjectMeta = metav1.ObjectMeta{
		Name:   AutoOpsNamer.Suffix(es.Name, "agent"),
		Labels: labels,
	}
	deployment.Spec = appsv1.DeploymentSpec{
		Replicas: pointer.Int32(1),
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"autoops.k8s.elastic.co/name": autoops.Name,
			},
		},
	}
	meta := metadata.Propagate(&autoops, metadata.Metadata{Labels: labels, Annotations: nil})
	podTemplateSpec := defaults.NewPodTemplateBuilder(basePodTemplate, "autoops-agent").
		WithLabels(meta.Labels).
		WithAnnotations(meta.Annotations).
		WithDockerImage(container.ImageRepository(container.AutoOpsAgentImage, v), v.String()).
		PodTemplate
	deployment.Spec.Template = podTemplateSpec
	return deployment, nil
}

func (r *ReconcileAutoOpsAgentPolicy) reconcileExpectedResources(ctx context.Context, es esv1.Elasticsearch, expectedResources ExpectedResources) error {
	return nil
}
