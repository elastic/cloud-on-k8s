// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"testing"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1"
	essettings "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/settings"
	kbconfig "github.com/elastic/cloud-on-k8s/pkg/controller/kibana/config"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func Test_Get(t *testing.T) {
	es := esv1.Elasticsearch{
		Spec: esv1.ElasticsearchSpec{
			NodeSets: []esv1.NodeSet{{
				Count: 10,
			}},
		},
	}
	licensingInfo, err := NewResourceReporter(k8s.FakeClient(&es)).Get()
	assert.NoError(t, err)
	assert.Equal(t, "42.95GB", licensingInfo.TotalManagedMemory)
	assert.Equal(t, "1", licensingInfo.EnterpriseResourceUnits)

	es = esv1.Elasticsearch{
		Spec: esv1.ElasticsearchSpec{
			NodeSets: []esv1.NodeSet{{
				Count: 100,
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: esv1.ElasticsearchContainerName,
								Resources: corev1.ResourceRequirements{
									Limits: map[corev1.ResourceName]resource.Quantity{
										corev1.ResourceMemory: resource.MustParse("6Gi"),
									},
								},
							},
						},
					},
				},
			}},
		},
	}
	licensingInfo, err = NewResourceReporter(k8s.FakeClient(&es)).Get()
	assert.NoError(t, err)
	assert.Equal(t, "644.25GB", licensingInfo.TotalManagedMemory)
	assert.Equal(t, "11", licensingInfo.EnterpriseResourceUnits)

	es = esv1.Elasticsearch{
		Spec: esv1.ElasticsearchSpec{
			NodeSets: []esv1.NodeSet{{
				Count: 10,
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: esv1.ElasticsearchContainerName,
								Env: []corev1.EnvVar{{
									Name: "ES_JAVA_OPTS", Value: "-Xms8G -Xmx8G",
								}},
							},
						},
					},
				},
			}},
		},
	}
	licensingInfo, err = NewResourceReporter(k8s.FakeClient(&es)).Get()
	assert.NoError(t, err)
	assert.Equal(t, "171.80GB", licensingInfo.TotalManagedMemory)
	assert.Equal(t, "3", licensingInfo.EnterpriseResourceUnits)

	es = esv1.Elasticsearch{
		Spec: esv1.ElasticsearchSpec{
			NodeSets: []esv1.NodeSet{{
				Count: 10,
				PodTemplate: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: esv1.ElasticsearchContainerName,
								Env: []corev1.EnvVar{{
									Name: essettings.EnvEsJavaOpts, Value: "-Xms8G -Xmx8G",
								}},
							},
						},
					},
				},
			}},
		},
	}
	licensingInfo, err = NewResourceReporter(k8s.FakeClient(&es)).Get()
	assert.NoError(t, err)
	assert.Equal(t, "171.80GB", licensingInfo.TotalManagedMemory)
	assert.Equal(t, "3", licensingInfo.EnterpriseResourceUnits)

	kb := kbv1.Kibana{
		Spec: kbv1.KibanaSpec{
			Count: 100,
		},
	}
	licensingInfo, err = NewResourceReporter(k8s.FakeClient(&kb)).Get()
	assert.NoError(t, err)
	assert.Equal(t, "107.37GB", licensingInfo.TotalManagedMemory)
	assert.Equal(t, "2", licensingInfo.EnterpriseResourceUnits)

	kb = kbv1.Kibana{
		Spec: kbv1.KibanaSpec{
			Count: 100,
			PodTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: kbv1.KibanaContainerName,
							Resources: corev1.ResourceRequirements{
								Limits: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
							},
						},
					},
				},
			},
		},
	}
	licensingInfo, err = NewResourceReporter(k8s.FakeClient(&kb)).Get()
	assert.NoError(t, err)
	assert.Equal(t, "214.75GB", licensingInfo.TotalManagedMemory)
	assert.Equal(t, "4", licensingInfo.EnterpriseResourceUnits)

	kb = kbv1.Kibana{
		Spec: kbv1.KibanaSpec{
			Count: 100,
			PodTemplate: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: kbv1.KibanaContainerName,
							Env: []corev1.EnvVar{{
								Name: kbconfig.EnvNodeOpts, Value: "--max-old-space-size=2048",
							}},
						},
					},
				},
			},
		},
	}
	licensingInfo, err = NewResourceReporter(k8s.FakeClient(&kb)).Get()
	assert.NoError(t, err)
	assert.Equal(t, "204.80GB", licensingInfo.TotalManagedMemory)
	assert.Equal(t, "4", licensingInfo.EnterpriseResourceUnits)
}
