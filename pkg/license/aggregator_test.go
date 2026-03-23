// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package license

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apmv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/apm/v1"
	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	entv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/enterprisesearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	lsv1alpha1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestMemFromJavaOpts(t *testing.T) {
	tests := []struct {
		name     string
		actual   string
		expected resource.Quantity
		isErr    bool
	}{
		{
			name:     "in k",
			actual:   "-Xms1k -Xmx8388608k",
			expected: resource.MustParse("16777216Ki"),
		},
		{
			name:     "in K",
			actual:   "-Xmx1024K",
			expected: resource.MustParse("2048Ki"),
		},
		{
			name:     "in m",
			actual:   "-Xmx512m -Xms256m",
			expected: resource.MustParse("1024Mi"),
		},
		{
			name:     "in M",
			actual:   "-Xmx256M",
			expected: resource.MustParse("512Mi"),
		},
		{
			name:     "in g",
			actual:   "-Xmx64g",
			expected: resource.MustParse("128Gi"),
		},
		{
			name:     "in G",
			actual:   "-Xmx64G",
			expected: resource.MustParse("128Gi"),
		},
		{
			name:     "without unit",
			actual:   "-Xmx1048576",
			expected: resource.MustParse("2Mi"),
		},
		{
			name:     "with trailing spaces at the end",
			actual:   "-Xms1k -Xmx8388608k   ",
			expected: resource.MustParse("16777216Ki"),
		},
		{
			name:     "with trailing space at the beginning",
			actual:   "  -Xms1k -Xmx8388608k",
			expected: resource.MustParse("16777216Ki"),
		},
		{
			name:     "no memory setting detected",
			actual:   "-Dlog4j2.formatMsgNoLookups=true",
			expected: resource.MustParse("0"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := memFromJavaOpts(tt.actual)
			if tt.isErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if !got.Equal(tt.expected) {
					t.Errorf("memFromJavaOpts(%s) = %v, want %s", tt.actual, got.String(), tt.expected.String())
				}
			}
		})
	}
}

func TestMemFromNodeOpts(t *testing.T) {
	tests := []struct {
		name     string
		actual   string
		expected string
		isErr    bool
	}{
		{
			name:     "with max-old-space-size option",
			actual:   "--max-old-space-size=2048",
			expected: "2048M",
		},
		{
			name:   "empty options",
			actual: "",
			isErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := memFromNodeOptions(tt.actual)
			if tt.isErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				got := resource.MustParse(tt.expected)
				if !got.Equal(q) {
					t.Errorf("memFromNodeOptions(%s) = %v, want %s", tt.actual, got, tt.expected)
				}
			}
		})
	}
}

func TestAggregator(t *testing.T) {
	objects := readObjects(t, "testdata/stack.yaml")
	client := k8s.NewFakeClient(objects...)
	aggregator := aggregator{client: client}

	val, err := aggregator.aggregateMemory(context.Background())
	require.NoError(t, err)
	for k, v := range map[string]float64{
		elasticsearchKey: 294.0,
		kibanaKey:        5.9073486328125,
		apmKey:           2.0,
		entSearchKey:     24.0,
		logstashKey:      4.0,
	} {
		require.Equal(t, v, val.appUsage[k].inGiB(), k)
	}
	require.Equal(t, 329.9073486328125, val.totalMemory.inGiB(), "total")
}

func TestAggregator_BasicLicenseExcluded(t *testing.T) {
	// Create an enterprise ES cluster and a basic ES cluster, plus a Kibana referencing each.
	enterpriseES := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: "enterprise-es", Namespace: "default"},
		Spec: esv1.ElasticsearchSpec{
			Version: "8.0.0",
			NodeSets: []esv1.NodeSet{{
				Name:  "data",
				Count: 1,
				PodTemplate: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "elasticsearch",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
						},
					}},
				}},
			}},
		},
	}
	basicES := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: "basic-es", Namespace: "default"},
		Spec: esv1.ElasticsearchSpec{
			Version:     "8.0.0",
			LicenseType: "basic",
			NodeSets: []esv1.NodeSet{{
				Name:  "data",
				Count: 2,
				PodTemplate: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "elasticsearch",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("8Gi")},
						},
					}},
				}},
			}},
		},
	}
	// Kibana referencing enterprise cluster
	kbEnterprise := &kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{Name: "kb-enterprise", Namespace: "default"},
		Spec: kbv1.KibanaSpec{
			ElasticsearchRef: commonv1.ObjectSelector{Name: "enterprise-es", Namespace: "default"},
			Count:            1,
			PodTemplate: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name: "kibana",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
					},
				}},
			}},
		},
	}
	// Kibana referencing basic cluster
	kbBasic := &kbv1.Kibana{
		ObjectMeta: metav1.ObjectMeta{Name: "kb-basic", Namespace: "default"},
		Spec: kbv1.KibanaSpec{
			ElasticsearchRef: commonv1.ObjectSelector{Name: "basic-es", Namespace: "default"},
			Count:            1,
			PodTemplate: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name: "kibana",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")},
					},
				}},
			}},
		},
	}

	c := k8s.NewFakeClient(enterpriseES, basicES, kbEnterprise, kbBasic)
	a := aggregator{client: c}

	val, err := a.aggregateMemory(context.Background())
	require.NoError(t, err)

	// Only enterprise ES cluster (4Gi) should be counted, not basic (2 * 8Gi = 16Gi)
	require.Equal(t, 4.0, val.appUsage[elasticsearchKey].inGiB(), "elasticsearch")
	// Only Kibana referencing enterprise cluster (1Gi) should be counted, not basic (2Gi)
	require.Equal(t, 1.0, val.appUsage[kibanaKey].inGiB(), "kibana")
	// Total = 4 + 1 = 5
	require.Equal(t, 5.0, val.totalMemory.inGiB(), "total")
}

func TestAggregator_AllBasicExcluded(t *testing.T) {
	// All clusters basic: everything excluded
	basicES := &esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{Name: "basic-es", Namespace: "default"},
		Spec: esv1.ElasticsearchSpec{
			Version:     "8.0.0",
			LicenseType: "basic",
			NodeSets: []esv1.NodeSet{{
				Name:  "data",
				Count: 1,
				PodTemplate: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: "elasticsearch",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
						},
					}},
				}},
			}},
		},
	}

	c := k8s.NewFakeClient(basicES)
	a := aggregator{client: c}

	val, err := a.aggregateMemory(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0.0, val.totalMemory.inGiB(), "total should be zero when all clusters are basic")
}

func readObjects(t *testing.T, filePath string) []client.Object {
	t.Helper()

	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(esv1.GroupVersion, &esv1.Elasticsearch{}, &esv1.ElasticsearchList{})
	scheme.AddKnownTypes(kbv1.GroupVersion, &kbv1.Kibana{}, &kbv1.KibanaList{})
	scheme.AddKnownTypes(apmv1.GroupVersion, &apmv1.ApmServer{}, &apmv1.ApmServerList{})
	scheme.AddKnownTypes(entv1.GroupVersion, &entv1.EnterpriseSearch{}, &entv1.EnterpriseSearchList{})
	scheme.AddKnownTypes(lsv1alpha1.GroupVersion, &lsv1alpha1.Logstash{}, &lsv1alpha1.LogstashList{})
	decoder := serializer.NewCodecFactory(scheme).UniversalDeserializer()

	f, err := os.Open(filePath)
	require.NoError(t, err)

	defer f.Close()

	yamlReader := yaml.NewYAMLReader(bufio.NewReader(f))

	var objects []client.Object

	for {
		yamlBytes, err := yamlReader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			require.NoError(t, err)
		}
		runtimeObj, _, err := decoder.Decode(yamlBytes, nil, nil)
		require.NoError(t, err)

		obj, ok := runtimeObj.(client.Object)
		require.True(t, ok)

		objects = append(objects, obj)
	}

	return objects
}
