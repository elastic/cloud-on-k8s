// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build agent || e2e

package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/v2/pkg/apis/agent/v1alpha1"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

func TestWebhook(t *testing.T) {
	agent := v1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-webhook",
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		Spec: v1alpha1.AgentSpec{
			Version: test.LatestReleasedVersion7x,
		},
	}

	err := test.NewK8sClientOrFatal().Client.Create(context.Background(), &agent)

	require.Error(t, err)
	require.Contains(
		t,
		err.Error(),
		`either daemonSet or deployment or statefulSet must be specified`,
	)
}
