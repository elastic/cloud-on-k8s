// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build mixed || e2e

package e2e

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/rand"

	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/elasticsearch"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test/helper"
)

const recipesFile = "../../config/recipes/autopilot/fleet-kubernetes-integration.yaml"

// TestAutopilot runs a test suite only if running within a GKE Autopilot
// cluster with a daemonset to set vm.max_map_count and an Elasticsearch
// and Kibana instance.
func TestAutopilot(t *testing.T) {
	if !test.Ctx().AutopilotCluster {
		return
	}
	randSuffix := rand.String(4)
	ns := test.Ctx().ManagedNamespace(0)

	transform := func(builder test.Builder) test.Builder {
		switch b := builder.(type) {
		case elasticsearch.Builder:
			b = b.WithoutAllowMMAP()
			return b

		default:
			return b
		}
	}

	helper.RunFile(t, recipesFile, ns, randSuffix, nil, transform)
}