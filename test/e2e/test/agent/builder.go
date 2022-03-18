// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package agent

import (
	"fmt"
	"testing"

	ghodssyaml "github.com/ghodss/yaml"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

const (
	PSPClusterRoleName = "elastic-agent-restricted"

	AgentFleetModeRoleName = "elastic-agent-fleet"

	// FleetServerPseudoKind is a lookup key for a version definition.
	// FleetServer has the same CRD as Agent but for testing purposes we want to be able to configure a different image.
	FleetServerPseudoKind = "FleetServer"
)

// Builder to create an Agent
type Builder struct {
	Agent              agentv1alpha1.Agent
	Validations        []ValidationFunc
	ValidationsOutputs []string
	AdditionalObjects  []k8sclient.Object

	// PodTemplate points to the PodTemplate in spec.DaemonSet or spec.Deployment
	PodTemplate *corev1.PodTemplateSpec

	// Suffix is the suffix that is added to e2e test resources
	Suffix string
}

func (b Builder) SkipTest() bool {
	supportedVersions := version.SupportedAgentVersions
	if b.Agent.Spec.FleetModeEnabled() {
		supportedVersions = version.SupportedFleetModeAgentVersions
	}

	ver := version.MustParse(b.Agent.Spec.Version)
	return supportedVersions.WithinRange(ver) != nil
}

// NewBuilderFromAgent creates an Agent builder from an existing Agent config. Sets all additional Builder fields
// appropriately.
func NewBuilderFromAgent(agent *agentv1alpha1.Agent) Builder {
	var podTemplate *corev1.PodTemplateSpec
	if agent.Spec.DaemonSet != nil {
		podTemplate = &agent.Spec.DaemonSet.PodTemplate
	} else if agent.Spec.Deployment != nil {
		podTemplate = &agent.Spec.Deployment.PodTemplate
	}

	return Builder{
		Agent:       *agent,
		PodTemplate: podTemplate,
	}
}

func NewBuilder(name string) Builder {
	suffix := rand.String(4)
	meta := metav1.ObjectMeta{
		Name:      name,
		Namespace: test.Ctx().ManagedNamespace(0),
		Labels:    map[string]string{run.TestNameLabel: name},
	}

	def := test.Ctx().ImageDefinitionFor(agentv1alpha1.Kind)
	return Builder{
		Agent: agentv1alpha1.Agent{
			ObjectMeta: meta,
			Spec: agentv1alpha1.AgentSpec{
				Version: def.Version,
			},
		},
		Suffix: suffix,
	}.
		WithImage(def.Image).
		WithSuffix(suffix).
		WithLabel(run.TestNameLabel, name).
		WithDaemonSet()
}

type ValidationFunc func(client.Client) error

func (b Builder) WithVersion(version string) Builder {
	b.Agent.Spec.Version = version
	return b
}

func (b Builder) WithDaemonSet() Builder {
	b.Agent.Spec.DaemonSet = &agentv1alpha1.DaemonSetSpec{}

	// if it exists, move PodTemplate from Deployment to DaemonSet
	if b.Agent.Spec.Deployment != nil {
		b.Agent.Spec.DaemonSet.PodTemplate = b.Agent.Spec.Deployment.PodTemplate
		b.Agent.Spec.Deployment = nil
	}

	b.PodTemplate = &b.Agent.Spec.DaemonSet.PodTemplate

	return b
}

func (b Builder) WithDeployment() Builder {
	b.Agent.Spec.Deployment = &agentv1alpha1.DeploymentSpec{}

	// if it exists, move PodTemplate from DaemonSet to Deployment
	if b.Agent.Spec.DaemonSet != nil {
		b.Agent.Spec.Deployment.PodTemplate = b.Agent.Spec.DaemonSet.PodTemplate
		b.Agent.Spec.DaemonSet = nil
	}
	b.PodTemplate = &b.Agent.Spec.Deployment.PodTemplate

	return b
}

func (b Builder) WithDeploymentStrategy(s appsv1.DeploymentStrategy) Builder {
	modifiedBuilder := b
	if b.Agent.Spec.Deployment == nil {
		modifiedBuilder = b.WithDeployment()
	}
	modifiedBuilder.Agent.Spec.Deployment.Strategy = s
	return modifiedBuilder
}

func (b Builder) WithDefaultESValidation(validation ValidationFunc) Builder {
	return b.WithESValidation(validation, "default")
}

func (b Builder) WithESValidation(validation ValidationFunc, outputName string) Builder {
	b.Validations = append(b.Validations, validation)
	b.ValidationsOutputs = append(b.ValidationsOutputs, outputName)

	return b
}

func (b Builder) WithElasticsearchRefs(refs ...agentv1alpha1.Output) Builder {
	b.Agent.Spec.ElasticsearchRefs = refs
	return b
}

func (b Builder) WithConfig(config *commonv1.Config) Builder {
	b.Agent.Spec.Config = config
	return b
}

func (b Builder) WithImage(image string) Builder {
	b.Agent.Spec.Image = image
	return b
}

func (b Builder) WithSuffix(suffix string) Builder {
	if suffix != "" {
		b.Agent.ObjectMeta.Name = b.Agent.ObjectMeta.Name + "-" + suffix
	}
	return b
}

func (b Builder) WithNamespace(namespace string) Builder {
	b.Agent.ObjectMeta.Namespace = namespace
	return b
}

func (b Builder) WithRestrictedSecurityContext() Builder {
	b.PodTemplate.Spec.SecurityContext = test.DefaultSecurityContext()

	return b
}

func (b Builder) WithContainerSecurityContext(securityContext corev1.SecurityContext) Builder {
	for i := range b.PodTemplate.Spec.Containers {
		b.PodTemplate.Spec.Containers[i].SecurityContext = &securityContext
	}

	return b
}

func (b Builder) WithLabel(key, value string) Builder {
	if b.Agent.Labels == nil {
		b.Agent.Labels = make(map[string]string)
	}
	b.Agent.Labels[key] = value

	return b
}

func (b Builder) WithPodLabel(key, value string) Builder {
	if b.PodTemplate.Labels == nil {
		b.PodTemplate.Labels = make(map[string]string)
	}
	b.PodTemplate.Labels[key] = value

	return b
}

func (b Builder) WithPodTemplateServiceAccount(name string) Builder {
	b.PodTemplate.Spec.ServiceAccountName = name

	return b
}

func (b Builder) WithRoles(clusterRoleNames ...string) Builder {
	resultBuilder := b
	for _, clusterRoleName := range clusterRoleNames {
		resultBuilder = bind(resultBuilder, clusterRoleName)
	}

	return resultBuilder
}

func bind(b Builder, clusterRoleName string) Builder {
	saName := b.PodTemplate.Spec.ServiceAccountName

	if saName == "" {
		saName = fmt.Sprintf("%s-sa", b.Agent.Name)
		b = b.WithPodTemplateServiceAccount(saName)
		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      saName,
				Namespace: b.Agent.Namespace,
			},
		}
		b.AdditionalObjects = append(b.AdditionalObjects, sa)
	}

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s-%s-binding", clusterRoleName, b.Agent.Namespace, b.Agent.Name),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: b.Agent.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     clusterRoleName,
		},
	}

	b.AdditionalObjects = append(b.AdditionalObjects, crb)

	return b
}

func (b Builder) WithSecureSettings(secretNames ...string) Builder {
	for _, secretName := range secretNames {
		b.Agent.Spec.SecureSettings = append(b.Agent.Spec.SecureSettings, commonv1.SecretSource{
			SecretName: secretName,
		})
	}

	return b
}

func (b Builder) WithConfigRef(secretName string) Builder {
	b.Agent.Spec.ConfigRef = &commonv1.ConfigSource{
		SecretRef: commonv1.SecretRef{
			SecretName: secretName,
		},
	}

	return b
}

func (b Builder) WithFleetMode() Builder {
	b.Agent.Spec.Mode = agentv1alpha1.AgentFleetMode

	return b
}

func (b Builder) WithFleetServer() Builder {
	b.Agent.Spec.FleetServerEnabled = true
	return b.WithFleetImage()
}

func (b Builder) WithFleetImage() Builder {
	def := test.Ctx().ImageDefinitionFor(FleetServerPseudoKind)
	b.Agent.Spec.Image = def.Image
	b.Agent.Spec.Version = def.Version
	return b
}

func (b Builder) WithKibanaRef(ref commonv1.ObjectSelector) Builder {
	b.Agent.Spec.KibanaRef = ref

	return b
}

func (b Builder) WithFleetServerRef(ref commonv1.ObjectSelector) Builder {
	b.Agent.Spec.FleetServerRef = ref

	return b
}

func (b Builder) WithObjects(objs ...k8sclient.Object) Builder {
	b.AdditionalObjects = append(b.AdditionalObjects, objs...)
	return b
}

func (b Builder) Ref() commonv1.ObjectSelector {
	return commonv1.ObjectSelector{
		Name:      b.Agent.Name,
		Namespace: b.Agent.Namespace,
	}
}

func (b Builder) RuntimeObjects() []k8sclient.Object {
	return append(b.AdditionalObjects, &b.Agent)
}

var _ test.Builder = Builder{}

func ApplyYamls(t *testing.T, b Builder, configYaml, podTemplateYaml string) Builder {
	t.Helper()
	if configYaml != "" {
		b.Agent.Spec.Config = &commonv1.Config{}
		err := settings.MustParseConfig([]byte(configYaml)).Unpack(&b.Agent.Spec.Config.Data)
		require.NoError(t, err)
	}

	if podTemplateYaml != "" {
		// use ghodss as settings package has issues with unpacking volumes part of the yamls
		err := ghodssyaml.Unmarshal([]byte(podTemplateYaml), b.PodTemplate)
		require.NoError(t, err)
	}

	return b
}

func ToOutput(selector commonv1.ObjectSelector, outputName string) agentv1alpha1.Output {
	return agentv1alpha1.Output{
		ObjectSelector: selector,
		OutputName:     outputName,
	}
}
