// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/rand"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	beatcommon "github.com/elastic/cloud-on-k8s/pkg/controller/beat/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

// Builder to create a Beat
type Builder struct {
	Beat           beatv1beta1.Beat
	Validations    []ValidationFunc
	SecurityConfig *SecurityConfig
}

type SecurityConfig struct {
	PspName         string
	ClusterRoleName string
}

func NewBuilderWithoutSuffix(name string, typ beatcommon.Type) Builder {
	return newBuilder(name, typ, "")
}

func NewBuilder(name string, typ beatcommon.Type) Builder {
	return newBuilder(name, typ, rand.String(4))
}

func newBuilder(name string, typ beatcommon.Type, suffix string) Builder {
	meta := metav1.ObjectMeta{
		Name:      name,
		Namespace: test.Ctx().ManagedNamespace(0),
		Labels:    map[string]string{run.TestNameLabel: name},
	}

	return Builder{
		Beat: beatv1beta1.Beat{
			ObjectMeta: meta,
			Spec: beatv1beta1.BeatSpec{
				Type:    string(typ),
				Version: test.Ctx().ElasticStackVersion,
			},
		},
	}.
		WithSuffix(suffix).
		WithLabel(run.TestNameLabel, name).
		WithPsp()
}

type ValidationFunc func(client.Client) error

func (b Builder) WithESValidations(validations ...ValidationFunc) Builder {
	b.Validations = append(b.Validations, validations...)

	return b
}

func (b Builder) WithElasticsearchRef(ref commonv1.ObjectSelector) Builder {
	b.Beat.Spec.ElasticsearchRef = ref
	return b
}

func (b Builder) WithPreset(preset beatv1beta1.PresetName) Builder {
	b.Beat.Spec.Preset = preset
	return b
}

func (b Builder) WithConfig(config *commonv1.Config) Builder {
	b.Beat.Spec.Config = config
	return b
}

func (b Builder) WithImage(image string) Builder {
	b.Beat.Spec.Image = image
	return b
}

func (b Builder) WithSuffix(suffix string) Builder {
	if suffix != "" {
		b.Beat.ObjectMeta.Name = b.Beat.ObjectMeta.Name + "-" + suffix
	}
	return b
}

func (b Builder) WithNamespace(namespace string) Builder {
	b.Beat.ObjectMeta.Namespace = namespace
	return b
}

func (b Builder) WithRestrictedSecurityContext() Builder {
	if b.Beat.Spec.DaemonSet != nil {
		b.Beat.Spec.DaemonSet.PodTemplate.Spec.SecurityContext = test.DefaultSecurityContext()
	}
	if b.Beat.Spec.Deployment != nil {
		b.Beat.Spec.Deployment.PodTemplate.Spec.SecurityContext = test.DefaultSecurityContext()
	}

	return b
}

func (b Builder) WithSecurityContext(podSecurityContext corev1.PodSecurityContext) Builder {
	if b.Beat.Spec.DaemonSet != nil {
		b.Beat.Spec.DaemonSet.PodTemplate.Spec.SecurityContext = &podSecurityContext
	}
	if b.Beat.Spec.Deployment != nil {
		b.Beat.Spec.Deployment.PodTemplate.Spec.SecurityContext = &podSecurityContext
	}

	return b
}

func (b Builder) WithLabel(key, value string) Builder {
	if b.Beat.Labels == nil {
		b.Beat.Labels = make(map[string]string)
	}
	b.Beat.Labels[key] = value

	return b
}

func (b Builder) WithPodLabel(key, value string) Builder {
	var podSpecs []corev1.PodTemplateSpec

	if b.Beat.Spec.DaemonSet != nil {
		podSpecs = append(podSpecs, b.Beat.Spec.DaemonSet.PodTemplate)
	}
	if b.Beat.Spec.Deployment != nil {
		podSpecs = append(podSpecs, b.Beat.Spec.Deployment.PodTemplate)
	}

	for _, podSpec := range podSpecs {
		if podSpec.Labels == nil {
			podSpec.Labels = make(map[string]string)
		}
		podSpec.Labels[key] = value
	}

	return b
}

func (b Builder) WithPsp() Builder {
	if b.SecurityConfig == nil {
		b.SecurityConfig = &SecurityConfig{
			PspName:         "elastic.beat.restricted",
			ClusterRoleName: "elastic-beat-restricted",
		}
	}

	return b
}

func (b Builder) RuntimeObjects() []runtime.Object {
	return []runtime.Object{&b.Beat}
}

var _ test.Builder = Builder{}
