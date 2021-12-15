// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	"sort"
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/defaults"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/elastic/cloud-on-k8s/pkg/utils/pointer"
	"github.com/go-test/deep"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type esSampleBuilder struct {
	userConfig              map[string]interface{}
	esAdditionalAnnotations map[string]string
}

func newEsSampleBuilder() *esSampleBuilder {
	return &esSampleBuilder{}
}

func (esb *esSampleBuilder) build() esv1.Elasticsearch {
	es := sampleES.DeepCopy()
	for k, v := range esb.esAdditionalAnnotations {
		es.Annotations[k] = v
	}
	if esb.userConfig != nil {
		es.Spec.NodeSets[0].Config = &commonv1.Config{Data: esb.userConfig}
	}
	return *es
}

var sampleES = esv1.Elasticsearch{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: "namespace",
		Name:      "name",
		Labels: map[string]string{
			"cluster-label-name": "cluster-label-value",
		},
		Annotations: map[string]string{
			"cluster-annotation-name": "cluster-annotation-value",
		},
	},
	Spec: esv1.ElasticsearchSpec{
		Version: "7.2.0",
		NodeSets: []esv1.NodeSet{
			{
				Name:  "nodeset-1",
				Count: 2,
				Config: &commonv1.Config{
					Data: map[string]interface{}{
						"node.attr.foo": "bar",
						"node.master":   "true",
						"node.data":     "false",
					},
				},
				PodTemplate: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"pod-template-label-name": "pod-template-label-value",
						},
						Annotations: map[string]string{
							"pod-template-annotation-name": "pod-template-annotation-value",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "additional-container",
							},
							{
								Name: "elasticsearch",
								Env: []corev1.EnvVar{
									{
										Name:  "my-env",
										Value: "my-value",
									},
								},
							},
						},
						InitContainers: []corev1.Container{
							{
								Name: "additional-init-container",
							},
						},
					},
				},
				VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
			},
			{
				Name:  "nodeset-1",
				Count: 2,
			},
		},
	},
}

func TestBuildPodTemplateSpecWithDefaultSecurityContext(t *testing.T) {
	for _, tt := range []struct {
		name                string
		version             version.Version
		setDefaultFSGroup   bool
		userSecurityContext *corev1.PodSecurityContext
		wantSecurityContext *corev1.PodSecurityContext
	}{
		{
			name:                "pre-8.0, setting off, no user context",
			version:             version.MustParse("7.8.0"),
			setDefaultFSGroup:   false,
			userSecurityContext: nil,
			wantSecurityContext: nil,
		},
		{
			name:                "pre-8.0, setting off, user context",
			version:             version.MustParse("7.8.0"),
			setDefaultFSGroup:   false,
			userSecurityContext: &corev1.PodSecurityContext{FSGroup: pointer.Int64(123)},
			wantSecurityContext: &corev1.PodSecurityContext{FSGroup: pointer.Int64(123)},
		},
		{
			name:                "pre-8.0, setting on, no user context",
			version:             version.MustParse("7.8.0"),
			setDefaultFSGroup:   true,
			userSecurityContext: nil,
			wantSecurityContext: nil,
		},
		{
			name:                "pre-8.0, setting on, user context",
			version:             version.MustParse("7.8.0"),
			setDefaultFSGroup:   true,
			userSecurityContext: &corev1.PodSecurityContext{FSGroup: pointer.Int64(123)},
			wantSecurityContext: &corev1.PodSecurityContext{FSGroup: pointer.Int64(123)},
		},
		{
			name:                "8.0+, setting off, no user context",
			version:             version.MustParse("8.0.0"),
			setDefaultFSGroup:   false,
			userSecurityContext: nil,
			wantSecurityContext: nil,
		},
		{
			name:                "8.0+, setting off, user context",
			version:             version.MustParse("8.0.0"),
			setDefaultFSGroup:   false,
			userSecurityContext: &corev1.PodSecurityContext{FSGroup: pointer.Int64(123)},
			wantSecurityContext: &corev1.PodSecurityContext{FSGroup: pointer.Int64(123)},
		},
		{
			name:                "8.0+, setting on, no user context",
			version:             version.MustParse("8.0.0"),
			setDefaultFSGroup:   true,
			userSecurityContext: nil,
			wantSecurityContext: &corev1.PodSecurityContext{FSGroup: pointer.Int64(1000)},
		},
		{
			name:                "8.0+, setting on, user context",
			version:             version.MustParse("8.0.0"),
			setDefaultFSGroup:   true,
			userSecurityContext: &corev1.PodSecurityContext{FSGroup: pointer.Int64(123)},
			wantSecurityContext: &corev1.PodSecurityContext{FSGroup: pointer.Int64(123)},
		},
		{
			name:                "8.0+, setting on, empty user context",
			version:             version.MustParse("8.0.0"),
			setDefaultFSGroup:   true,
			userSecurityContext: &corev1.PodSecurityContext{},
			wantSecurityContext: &corev1.PodSecurityContext{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			es := *sampleES.DeepCopy()
			es.Spec.Version = tt.version.String()
			es.Spec.NodeSets[0].PodTemplate.Spec.SecurityContext = tt.userSecurityContext

			cfg, err := settings.NewMergedESConfig(es.Name, tt.version, corev1.IPv4Protocol, es.Spec.HTTP, *es.Spec.NodeSets[0].Config)
			require.NoError(t, err)

			actual, err := BuildPodTemplateSpec(k8s.NewFakeClient(), es, es.Spec.NodeSets[0], cfg, nil, tt.setDefaultFSGroup)
			require.NoError(t, err)
			require.Equal(t, tt.wantSecurityContext, actual.Spec.SecurityContext)
		})
	}
}

func TestBuildPodTemplateSpec(t *testing.T) {
	nodeSet := sampleES.Spec.NodeSets[0]
	ver, err := version.Parse(sampleES.Spec.Version)
	require.NoError(t, err)
	cfg, err := settings.NewMergedESConfig(sampleES.Name, ver, corev1.IPv4Protocol, sampleES.Spec.HTTP, *nodeSet.Config)
	require.NoError(t, err)

	actual, err := BuildPodTemplateSpec(k8s.NewFakeClient(), sampleES, sampleES.Spec.NodeSets[0], cfg, nil, false)
	require.NoError(t, err)

	// build expected PodTemplateSpec

	terminationGracePeriodSeconds := DefaultTerminationGracePeriodSeconds
	varFalse := false

	volumes, volumeMounts := buildVolumes(sampleES.Name, nodeSet, nil)
	// should be sorted
	sort.Slice(volumes, func(i, j int) bool { return volumes[i].Name < volumes[j].Name })
	sort.Slice(volumeMounts, func(i, j int) bool { return volumeMounts[i].Name < volumeMounts[j].Name })

	initContainers, err := initcontainer.NewInitContainers(transportCertificatesVolume(sampleES.Name), nil)
	require.NoError(t, err)
	// init containers should be patched with volume and inherited env vars and image
	headlessSvcEnvVar := corev1.EnvVar{Name: "HEADLESS_SERVICE_NAME", Value: "name-es-nodeset-1"}
	esDockerImage := "docker.elastic.co/elasticsearch/elasticsearch:7.2.0"
	for i := range initContainers {
		initContainers[i].Image = esDockerImage
		initContainers[i].Env = append(initContainers[i].Env, headlessSvcEnvVar)
		initContainers[i].VolumeMounts = append(initContainers[i].VolumeMounts, volumeMounts...)
		initContainers[i].Resources = DefaultResources
	}

	// remove the prepare-fs init-container from comparison, it has its own volume mount logic
	// that is harder to test
	for i, c := range initContainers {
		if c.Name == initcontainer.PrepareFilesystemContainerName {
			initContainers = append(initContainers[:i], initContainers[i+1:]...)
		}
	}
	for i, c := range actual.Spec.InitContainers {
		if c.Name == initcontainer.PrepareFilesystemContainerName {
			actual.Spec.InitContainers = append(actual.Spec.InitContainers[:i], actual.Spec.InitContainers[i+1:]...)
		}
	}

	expected := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"common.k8s.elastic.co/type":                    "elasticsearch",
				"elasticsearch.k8s.elastic.co/cluster-name":     "name",
				"elasticsearch.k8s.elastic.co/config-hash":      "3415561705",
				"elasticsearch.k8s.elastic.co/http-scheme":      "https",
				"elasticsearch.k8s.elastic.co/node-data":        "false",
				"elasticsearch.k8s.elastic.co/node-ingest":      "true",
				"elasticsearch.k8s.elastic.co/node-master":      "true",
				"elasticsearch.k8s.elastic.co/node-ml":          "true",
				"elasticsearch.k8s.elastic.co/statefulset-name": "name-es-nodeset-1",
				"elasticsearch.k8s.elastic.co/version":          "7.2.0",
				"pod-template-label-name":                       "pod-template-label-value",
			},
			Annotations: map[string]string{
				"pod-template-annotation-name": "pod-template-annotation-value",
				"co.elastic.logs/module":       "elasticsearch",
			},
		},
		Spec: corev1.PodSpec{
			Volumes: volumes,
			InitContainers: append(initContainers, corev1.Container{
				Name:         "additional-init-container",
				Image:        esDockerImage,
				Env:          defaults.ExtendPodDownwardEnvVars(headlessSvcEnvVar),
				VolumeMounts: volumeMounts,
				Resources:    DefaultResources, // inherited from main container
			}),
			Containers: []corev1.Container{
				{
					Name: "additional-container",
				},
				{
					Name:  "elasticsearch",
					Image: esDockerImage,
					Ports: []corev1.ContainerPort{
						{Name: "https", HostPort: 0, ContainerPort: 9200, Protocol: "TCP", HostIP: ""},
						{Name: "transport", HostPort: 0, ContainerPort: 9300, Protocol: "TCP", HostIP: ""},
					},
					Env: append(
						[]corev1.EnvVar{{Name: "my-env", Value: "my-value"}},
						DefaultEnvVars(sampleES.Spec.HTTP, HeadlessServiceName(esv1.StatefulSet(sampleES.Name, nodeSet.Name)))...),
					Resources:      DefaultResources,
					VolumeMounts:   volumeMounts,
					ReadinessProbe: NewReadinessProbe(),
					Lifecycle: &corev1.Lifecycle{
						PreStop: NewPreStopHook(),
					},
				},
			},
			TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
			AutomountServiceAccountToken:  &varFalse,
			Affinity:                      DefaultAffinity(sampleES.Name),
		},
	}

	deep.MaxDepth = 25
	require.Nil(t, deep.Equal(expected, actual))
}

func Test_getDefaultContainerPorts(t *testing.T) {
	tt := []struct {
		name string
		es   esv1.Elasticsearch
		want []corev1.ContainerPort
	}{
		{
			name: "https",
			es:   sampleES,
			want: []corev1.ContainerPort{
				{Name: "https", HostPort: 0, ContainerPort: 9200, Protocol: "TCP", HostIP: ""},
				{Name: "transport", HostPort: 0, ContainerPort: 9300, Protocol: "TCP", HostIP: ""},
			},
		},
		{
			name: "http",
			es: esv1.Elasticsearch{
				Spec: esv1.ElasticsearchSpec{
					HTTP: commonv1.HTTPConfig{
						TLS: commonv1.TLSOptions{
							SelfSignedCertificate: &commonv1.SelfSignedCertificate{
								Disabled: true,
							},
						},
					},
				},
			},
			want: []corev1.ContainerPort{
				{Name: "http", HostPort: 0, ContainerPort: 9200, Protocol: "TCP", HostIP: ""},
				{Name: "transport", HostPort: 0, ContainerPort: 9300, Protocol: "TCP", HostIP: ""},
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, getDefaultContainerPorts(tc.es), tc.want)
		})
	}
}

func Test_enableLog4JFormatMsgNoLookups(t *testing.T) {
	tt := []struct {
		name                       string
		version                    string
		userEnv                    []corev1.EnvVar
		expectedEsJavaOptsEnvValue string
	}{
		{
			name:                       "before 7.2.0, JVM log4j2.formatMsgNoLookups parameter is set by default",
			version:                    "7.0.0",
			userEnv:                    []corev1.EnvVar{{Name: "YO", Value: "LO"}},
			expectedEsJavaOptsEnvValue: "-Dlog4j2.formatMsgNoLookups=true",
		},
		{
			name:                       "before 7.2.0, JVM log4j2.formatMsgNoLookups parameter is merged with user-provided JVM parameters",
			version:                    "7.1.0",
			userEnv:                    []corev1.EnvVar{{Name: "ES_JAVA_OPTS", Value: "-Xms=42000 -Xmx=42000"}},
			expectedEsJavaOptsEnvValue: "-Dlog4j2.formatMsgNoLookups=true -Xms=42000 -Xmx=42000",
		},
		{
			name:                       "before 7.2.0, JVM log4j2.formatMsgNoLookups user-provided parameter is not overridden by us",
			version:                    "7.1.0",
			userEnv:                    []corev1.EnvVar{{Name: "ES_JAVA_OPTS", Value: "-Xms=42000 -Dlog4j2.formatMsgNoLookups=false -Xmx=42000"}},
			expectedEsJavaOptsEnvValue: "-Xms=42000 -Dlog4j2.formatMsgNoLookups=false -Xmx=42000",
		},
		{
			name:                       "since 7.2.0, JVM log4j2.formatMsgNoLookups parameter is not set by default",
			version:                    "7.2.0",
			userEnv:                    []corev1.EnvVar{{Name: "ES_JAVA_OPTS", Value: "-Xms=42000 -Xmx=42000"}},
			expectedEsJavaOptsEnvValue: "-Xms=42000 -Xmx=42000",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			sampleES := newEsSampleBuilder().build()
			// set version
			sampleES.Spec.Version = tc.version
			// set user env
			sampleES.Spec.NodeSets[0].PodTemplate.Spec.Containers[1].Env = tc.userEnv

			ver, err := version.Parse(sampleES.Spec.Version)
			require.NoError(t, err)
			cfg, err := settings.NewMergedESConfig(sampleES.Name, ver, corev1.IPv4Protocol, sampleES.Spec.HTTP, *sampleES.Spec.NodeSets[0].Config)
			require.NoError(t, err)
			actual, err := BuildPodTemplateSpec(k8s.NewFakeClient(), sampleES, sampleES.Spec.NodeSets[0], cfg, nil, false)
			require.NoError(t, err)

			env := actual.Spec.Containers[1].Env
			envMap := make(map[string]string)
			for _, e := range env {
				envMap[e.Name] = e.Value
			}
			assert.Equal(t, len(env), len(envMap))
			assert.Equal(t, tc.expectedEsJavaOptsEnvValue, envMap[settings.EnvEsJavaOpts])
		})
	}
}
