// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package beat

import (
	"fmt"
	"strings"
	"testing"

	ghodssyaml "github.com/ghodss/yaml"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	"github.com/stretchr/testify/require"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	beatcommon "github.com/elastic/cloud-on-k8s/pkg/controller/beat/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/elastic/cloud-on-k8s/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
)

const (
	PSPClusterRoleName            = "elastic-beat-restricted"
	AuditbeatPSPClusterRoleName   = "elastic-auditbeat-restricted"
	PacketbeatPSPClusterRoleName  = "elastic-packetbeat-restricted"
	JournalbeatPSPClusterRoleName = "elastic-journalbeat-restricted"

	AutodiscoverClusterRoleName = "elastic-beat-autodiscover"
	MetricbeatClusterRoleName   = "elastic-beat-metricbeat"
)

// Builder to create a Beat
type Builder struct {
	Beat              beatv1beta1.Beat
	Validations       []ValidationFunc
	AdditionalObjects []k8sclient.Object

	// PodTemplate points to the PodTemplate in spec.DaemonSet or spec.Deployment
	PodTemplate *corev1.PodTemplateSpec

	// Suffix is the suffix that is added to e2e test resources
	Suffix string
}

func (b Builder) SkipTest() bool {
	// Beat on OpenShift3 requires specific securityContext due to hostPath volumes.
	// Skipping all Beat tests to reduce maintenance burden.
	if test.Ctx().Provider == "ocp3" {
		return true
	}

	ver := version.MustParse(b.Beat.Spec.Version)
	return version.SupportedBeatVersions.WithinRange(ver) != nil
}

// NewBuilderFromBeat creates a Beat builder from an existing Beat config. Sets all additional Builder fields
// appropriately.
func NewBuilderFromBeat(beat *beatv1beta1.Beat) Builder {
	var podTemplate *corev1.PodTemplateSpec
	if beat.Spec.DaemonSet != nil {
		podTemplate = &beat.Spec.DaemonSet.PodTemplate
	} else if beat.Spec.Deployment != nil {
		podTemplate = &beat.Spec.Deployment.PodTemplate
	}

	return Builder{
		Beat:        *beat,
		PodTemplate: podTemplate,
	}
}

func NewBuilder(name string) Builder {
	return newBuilder(name, rand.String(4))
}

func newBuilder(name string, suffix string) Builder {
	meta := metav1.ObjectMeta{
		Name:      name,
		Namespace: test.Ctx().ManagedNamespace(0),
		Labels:    map[string]string{run.TestNameLabel: name},
	}

	return Builder{
		Beat: beatv1beta1.Beat{
			ObjectMeta: meta,
			Spec: beatv1beta1.BeatSpec{
				Version: test.Ctx().ElasticStackVersion,
			},
		},
		Suffix: suffix,
	}.
		WithSuffix(suffix).
		WithLabel(run.TestNameLabel, name).
		WithDaemonSet()
}

type ValidationFunc func(client.Client) error

func (b Builder) WithType(typ beatcommon.Type) Builder {
	typeStr := string(typ)
	// for Beats we have to use the specific type as there are different Beats images within the one CRD kind.
	// capitalize the Beat name to be consistent in spelling with the other CRD kinds.
	def := test.Ctx().ImageDefinitionFor(strings.Title(typeStr))
	b.Beat.Spec.Type = typeStr
	b.Beat.Spec.Version = def.Version
	b.Beat.Spec.Image = def.Image
	return b
}

func (b Builder) WithVersion(version string) Builder {
	b.Beat.Spec.Version = version
	return b
}

func (b Builder) WithDaemonSet() Builder {
	b.Beat.Spec.DaemonSet = &beatv1beta1.DaemonSetSpec{}

	// if it exists, move PodTemplate from Deployment to DaemonSet
	if b.Beat.Spec.Deployment != nil {
		b.Beat.Spec.DaemonSet.PodTemplate = b.Beat.Spec.Deployment.PodTemplate
		b.Beat.Spec.Deployment = nil
	}

	b.PodTemplate = &b.Beat.Spec.DaemonSet.PodTemplate

	return b
}

func (b Builder) WithDeployment() Builder {
	b.Beat.Spec.Deployment = &beatv1beta1.DeploymentSpec{}

	// if it exists, move PodTemplate from DaemonSet to Deployment
	if b.Beat.Spec.DaemonSet != nil {
		b.Beat.Spec.Deployment.PodTemplate = b.Beat.Spec.DaemonSet.PodTemplate
		b.Beat.Spec.DaemonSet = nil
	}
	b.PodTemplate = &b.Beat.Spec.Deployment.PodTemplate

	return b
}

func (b Builder) WithDeploymentStrategy(s appsv1.DeploymentStrategy) Builder {
	modifiedBuilder := b
	if b.Beat.Spec.Deployment == nil {
		modifiedBuilder = b.WithDeployment()
	}
	modifiedBuilder.Beat.Spec.Deployment.Strategy = s
	return modifiedBuilder
}

func (b Builder) WithESValidations(validations ...ValidationFunc) Builder {
	b.Validations = append(b.Validations, validations...)

	return b
}

func (b Builder) WithElasticsearchRef(ref commonv1.ObjectSelector) Builder {
	b.Beat.Spec.ElasticsearchRef = ref
	return b
}

func (b Builder) WithKibanaRef(ref commonv1.ObjectSelector) Builder {
	b.Beat.Spec.KibanaRef = ref
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
	if b.Beat.Labels == nil {
		b.Beat.Labels = make(map[string]string)
	}
	b.Beat.Labels[key] = value

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
	for _, clusterRoleName := range clusterRoleNames {
		b = bind(b, clusterRoleName)
	}

	return b
}

func bind(b Builder, clusterRoleName string) Builder {
	saName := b.PodTemplate.Spec.ServiceAccountName

	if saName == "" {
		saName = fmt.Sprintf("%s-sa", b.Beat.Name)
		b = b.WithPodTemplateServiceAccount(saName)
		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      saName,
				Namespace: b.Beat.Namespace,
			},
		}
		b.AdditionalObjects = append(b.AdditionalObjects, sa)
	}

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s-%s-binding", clusterRoleName, b.Beat.Namespace, b.Beat.Name),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: b.Beat.Namespace,
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
		b.Beat.Spec.SecureSettings = append(b.Beat.Spec.SecureSettings, commonv1.SecretSource{
			SecretName: secretName,
		})
	}

	return b
}

func (b Builder) WithConfigRef(secretName string) Builder {
	b.Beat.Spec.ConfigRef = &commonv1.ConfigSource{
		SecretRef: commonv1.SecretRef{
			SecretName: secretName,
		},
	}

	return b
}

func (b Builder) WithObjects(objs ...k8sclient.Object) Builder {
	b.AdditionalObjects = append(b.AdditionalObjects, objs...)
	return b
}

func (b Builder) RuntimeObjects() []k8sclient.Object {
	return append(b.AdditionalObjects, &b.Beat)
}

var _ test.Builder = Builder{}

func ApplyYamls(t *testing.T, b Builder, configYaml, podTemplateYaml string) Builder {
	t.Helper()
	if configYaml != "" {
		b.Beat.Spec.Config = &commonv1.Config{}
		err := settings.MustParseConfig([]byte(configYaml)).Unpack(&b.Beat.Spec.Config.Data)
		require.NoError(t, err)
	}

	if podTemplateYaml != "" {
		// use ghodss as settings package has issues with unpacking volumes part of the yamls
		err := ghodssyaml.Unmarshal([]byte(podTemplateYaml), b.PodTemplate)
		require.NoError(t, err)
	}

	return b
}
