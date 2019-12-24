// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kb

import (
	"bufio"
	"bytes"
	"testing"
	"text/template"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
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

	namespace := test.Ctx().ManagedNamespace(0)
	stackVersion := test.Ctx().ElasticStackVersion

	templateFile := "testdata/kibana_standalone.yaml"
	v := version.MustParse(stackVersion)
	if v.Major == 6 {
		templateFile = "testdata/kibana_standalone_6x.yaml"
	}

	tmpl, err := template.ParseFiles(templateFile)
	require.NoError(t, err, "Failed to parse template")

	buf := new(bytes.Buffer)
	require.NoError(t, tmpl.Execute(buf, map[string]string{"Suffix": rand.String(4)}))

	transform := func(builder test.Builder) test.Builder {
		switch b := builder.(type) {
		case elasticsearch.Builder:
			return b.WithNamespace(namespace).
				WithVersion(stackVersion).
				WithRestrictedSecurityContext()
		case kibana.Builder:
			return b.WithNamespace(namespace).
				WithVersion(stackVersion).
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
