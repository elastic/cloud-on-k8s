// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

//go:build beat || e2e

package beat

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/v2/test/e2e/test"
)

func TestWebhook(t *testing.T) {
	beat := beatv1beta1.Beat{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-webhook",
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		Spec: beatv1beta1.BeatSpec{
			Type:    "filebeat",
			Version: test.LatestReleasedVersion7x,
			// neither DaemonSet nor Deployment provided - this should result in an error like below
		},
	}

	err := test.NewK8sClientOrFatal().Client.Create(context.Background(), &beat)

	require.Error(t, err)
	require.Contains(
		t,
		err.Error(),
		`admission webhook "elastic-beat-validation-v1beta1.k8s.elastic.co" denied the request: Beat.beat.k8s.elastic.co "test-webhook" is invalid`,
	)
}
