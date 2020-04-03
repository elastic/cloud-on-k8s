// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package e2e

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/test/e2e/cmd/run"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/apmserver"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/helper"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/kibana"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/rand"
)

func TestSamples(t *testing.T) {
	sampleFiles, err := filepath.Glob("../../config/samples/*/*.yaml")
	require.NoError(t, err, "Failed to find samples")

	decoder := helper.NewYAMLDecoder()
	for _, sample := range sampleFiles {
		testName := mkTestName(t, sample)
		builders := createBuilders(t, decoder, sample, testName)
		t.Run(testName, func(t *testing.T) {
			test.Sequence(nil, test.EmptySteps, builders...).RunSequential(t)
		})
	}
}

func mkTestName(t *testing.T, path string) string {
	t.Helper()

	baseName := filepath.Base(path)
	baseName = strings.TrimSuffix(baseName, ".yaml")
	parentDir := filepath.Base(filepath.Dir(path))
	testName := filepath.Join(parentDir, baseName)

	// testName will be used as label, so avoid using illegal chars
	return strings.ReplaceAll(testName, "/", "-")
}

func createBuilders(t *testing.T, decoder *helper.YAMLDecoder, sampleFile, testName string) []test.Builder {
	t.Helper()

	f, err := os.Open(sampleFile)
	require.NoError(t, err, "Failed to open file %s", sampleFile)
	defer f.Close()

	namespace := test.Ctx().ManagedNamespace(0)
	suffix := rand.String(4)
	transform := func(builder test.Builder) test.Builder {
		fullTestName := "TestSamples-" + testName
		switch b := builder.(type) {
		case elasticsearch.Builder:
			return b.WithNamespace(namespace).
				WithSuffix(suffix).
				WithRestrictedSecurityContext().
				WithLabel(run.TestNameLabel, fullTestName).
				WithPodLabel(run.TestNameLabel, fullTestName)
		case kibana.Builder:
			return b.WithNamespace(namespace).
				WithSuffix(suffix).
				WithElasticsearchRef(tweakElasticsearchRef(b.Kibana.Spec.ElasticsearchRef, suffix)).
				WithRestrictedSecurityContext().
				WithLabel(run.TestNameLabel, fullTestName).
				WithPodLabel(run.TestNameLabel, fullTestName)
		case apmserver.Builder:
			return b.WithNamespace(namespace).
				WithSuffix(suffix).
				WithElasticsearchRef(tweakElasticsearchRef(b.ApmServer.Spec.ElasticsearchRef, suffix)).
				WithConfig(map[string]interface{}{"apm-server.ilm.enabled": false}).
				WithRestrictedSecurityContext().
				WithLabel(run.TestNameLabel, fullTestName).
				WithPodLabel(run.TestNameLabel, fullTestName)
		default:
			return b
		}
	}

	builders, err := decoder.ToBuilders(bufio.NewReader(f), transform)
	require.NoError(t, err, "Failed to create builders")
	return builders
}

func tweakElasticsearchRef(ref commonv1.ObjectSelector, suffix string) commonv1.ObjectSelector {
	// All the objects defined in the YAML file will have a random test suffix added to prevent clashes with previous runs.
	// This necessitates changing the Elasticsearch reference to match the suffixed name.
	if ref.Name != "" {
		ref.Name = ref.Name + "-" + suffix
	}

	return ref
}
