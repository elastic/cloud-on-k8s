// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

// +build agent e2e

package agent

import (
	"context"
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/apis/agent/v1alpha1"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWebhook(t *testing.T) {
	agent := v1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-webhook",
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		Spec: v1alpha1.AgentSpec{
			Version: "7.10.0",
		},
	}

	err := test.NewK8sClientOrFatal().Client.Create(context.Background(), &agent)

	require.Error(t, err)
	require.Contains(
		t,
		err.Error(),
		`either daemonset or deployment must be specified`,
	)
}
