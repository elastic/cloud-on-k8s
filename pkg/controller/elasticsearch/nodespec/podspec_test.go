// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

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
	"github.com/go-test/deep"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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

func TestBuildPodTemplateSpec(t *testing.T) {
	nodeSet := sampleES.Spec.NodeSets[0]
	ver, err := version.Parse(sampleES.Spec.Version)
	require.NoError(t, err)
	cfg, err := settings.NewMergedESConfig(sampleES.Name, *ver, sampleES.Spec.HTTP, *nodeSet.Config)
	require.NoError(t, err)

	actual, err := BuildPodTemplateSpec(sampleES, sampleES.Spec.NodeSets[0], cfg, nil)
	require.NoError(t, err)

	// build expected PodTemplateSpec

	terminationGracePeriodSeconds := DefaultTerminationGracePeriodSeconds
	varFalse := false

	volumes, volumeMounts := buildVolumes(sampleES.Name, nodeSet, nil)
	// should be sorted
	sort.Slice(volumes, func(i, j int) bool { return volumes[i].Name < volumes[j].Name })
	sort.Slice(volumeMounts, func(i, j int) bool { return volumeMounts[i].Name < volumeMounts[j].Name })

	initContainers, err := initcontainer.NewInitContainers(
		"docker.elastic.co/elasticsearch/elasticsearch:7.2.0",
		transportCertificatesVolume(sampleES.Name),
		sampleES.Name,
		nil,
	)
	require.NoError(t, err)
	// should be patched with volume and env
	for i := range initContainers {
		initContainers[i].Env = append(initContainers[i].Env, defaults.PodDownwardEnvVars()...)
		initContainers[i].VolumeMounts = append(initContainers[i].VolumeMounts, volumeMounts...)
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
				"elasticsearch.k8s.elastic.co/config-hash":      "2535617526",
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
				Image:        "docker.elastic.co/elasticsearch/elasticsearch:7.2.0",
				Env:          defaults.PodDownwardEnvVars(),
				VolumeMounts: volumeMounts,
			}),
			Containers: []corev1.Container{
				{
					Name: "additional-container",
				},
				{
					Name:  "elasticsearch",
					Image: "docker.elastic.co/elasticsearch/elasticsearch:7.2.0",
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
