// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apmtype "github.com/elastic/cloud-on-k8s/pkg/apis/apm/v1beta1"
	commonv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1beta1"
	estype "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1beta1"
	kbtype "github.com/elastic/cloud-on-k8s/pkg/apis/kibana/v1beta1"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/apmserver"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/kibana"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestSamples(t *testing.T) {
	sampleFiles, err := filepath.Glob("../../config/samples/*/*.yaml")
	require.NoError(t, err, "Failed to find samples")

	decoder := createDecoder(t)
	for _, sample := range sampleFiles {
		builders := createBuilders(t, decoder, sample)
		testName := mkTestName(t, sample)
		t.Run(testName, func(t *testing.T) {
			test.Sequence(nil, test.EmptySteps, builders...).RunSequential(t)
		})
	}
}

func createDecoder(t *testing.T) runtime.Decoder {
	t.Helper()

	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(estype.GroupVersion, &estype.Elasticsearch{}, &estype.ElasticsearchList{})
	scheme.AddKnownTypes(kbtype.GroupVersion, &kbtype.Kibana{}, &kbtype.KibanaList{})
	scheme.AddKnownTypes(apmtype.GroupVersion, &apmtype.ApmServer{}, &apmtype.ApmServerList{})
	return serializer.NewCodecFactory(scheme).UniversalDeserializer()
}

func mkTestName(t *testing.T, path string) string {
	t.Helper()

	baseName := filepath.Base(path)
	baseName = strings.TrimSuffix(baseName, ".yaml")
	parentDir := filepath.Base(filepath.Dir(path))
	return filepath.Join(parentDir, baseName)
}

func createBuilders(t *testing.T, decoder runtime.Decoder, sampleFile string) []test.Builder {
	t.Helper()

	f, err := os.Open(sampleFile)
	require.NoError(t, err, "Failed to open file %s", sampleFile)
	defer f.Close()

	var builders []test.Builder
	namespace := test.Ctx().ManagedNamespace(0)
	suffix := rand.String(4)

	yamlReader := yaml.NewYAMLReader(bufio.NewReader(f))
	for {
		yamlBytes, err := yamlReader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			require.NoError(t, err, "Failed to read YAML from %s", sampleFile)
		}
		obj, _, err := decoder.Decode(yamlBytes, nil, nil)
		require.NoError(t, err, "Failed to decode YAML from %s", sampleFile)

		var builder test.Builder

		switch decodedObj := obj.(type) {
		case *estype.Elasticsearch:
			b := elasticsearch.NewBuilderWithoutSuffix(decodedObj.Name)
			b.Elasticsearch = *decodedObj
			builder = b.WithNamespace(namespace).
				WithSuffix(suffix).
				WithRestrictedSecurityContext()
		case *kbtype.Kibana:
			b := kibana.NewBuilderWithoutSuffix(decodedObj.Name)
			b.Kibana = *decodedObj
			builder = b.WithNamespace(namespace).
				WithSuffix(suffix).
				WithElasticsearchRef(tweakElasticsearchRef(b.Kibana.Spec.ElasticsearchRef, namespace, suffix)).
				WithRestrictedSecurityContext()
		case *apmtype.ApmServer:
			b := apmserver.NewBuilderWithoutSuffix(decodedObj.Name)
			b.ApmServer = *decodedObj
			builder = b.WithNamespace(namespace).
				WithSuffix(suffix).
				WithElasticsearchRef(tweakElasticsearchRef(b.ApmServer.Spec.ElasticsearchRef, namespace, suffix)).
				WithConfig(map[string]interface{}{"apm-server.ilm.enabled": false}).
				WithRestrictedSecurityContext()
		}

		builders = append(builders, builder)
	}

	return builders
}

func tweakElasticsearchRef(ref commonv1beta1.ObjectSelector, namespace, suffix string) commonv1beta1.ObjectSelector {
	if ref.Name != "" {
		ref.Name = ref.Name + "-" + suffix
		if ref.Namespace == "" {
			ref.Namespace = namespace
		}
	}

	return ref
}
