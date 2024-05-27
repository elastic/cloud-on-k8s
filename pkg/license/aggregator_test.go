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
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apmv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/apm/v1"
	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	entv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/enterprisesearch/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/kibana/v1"
	lsv1alpha1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/logstash/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
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
	aggregator := Aggregator{client: client}

	val, err := aggregator.AggregateMemory(context.Background())
	require.NoError(t, err)
	require.Equal(t, 329.9073486328125, inGiB(val))
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
