// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kb

import (
	"bufio"
	"bytes"
	"testing"
	"text/template"

	commonv1 "github.com/elastic/cloud-on-k8s/pkg/apis/common/v1"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/helper"
	"github.com/elastic/cloud-on-k8s/test/e2e/test/kibana"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/util/rand"
)

// TestKibanaStandalone tests running Kibana without an automatic association to Elasticsearch.
func TestKibanaStandalone(t *testing.T) {
	builders := mkKibanaStandaloneBuilders(t)
	test.Sequence(nil, test.EmptySteps, builders...).RunSequential(t)
}

func mkKibanaStandaloneBuilders(t *testing.T) []test.Builder {
	t.Helper()

	tmpl, err := template.ParseFiles("testdata/kibana_standalone.yaml")
	require.NoError(t, err, "Failed to parse template")

	buf := new(bytes.Buffer)
	rndSuffix := rand.String(4)
	esName := "test-kibana-standalone-es-" + rndSuffix
	require.NoError(t, tmpl.Execute(buf, map[string]string{
		"ESName": esName,
		"Suffix": rndSuffix,
	}))

	namespace := test.Ctx().ManagedNamespace(0)
	stackVersion := test.Ctx().ElasticStackVersion

	transform := func(builder test.Builder) test.Builder {
		switch b := builder.(type) {
		case elasticsearch.Builder:
			return b.WithNamespace(namespace).
				WithVersion(stackVersion).
				WithRestrictedSecurityContext()
		case kibana.Builder:
			return b.WithNamespace(namespace).
				WithVersion(stackVersion).
				WithExternalElasticsearchRef(commonv1.ObjectSelector{
					Namespace: namespace,
					Name:      esName,
				}).
				WithRestrictedSecurityContext()
		default:
			return b
		}
	}

	decoder := helper.NewYAMLDecoder()
	builders, err := decoder.ToBuilders(bufio.NewReader(buf), transform)
	require.NoError(t, err, "Failed to create builders")

	return builders
}
