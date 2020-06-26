// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package beat

import (
	"testing"

	beatv1beta1 "github.com/elastic/cloud-on-k8s/pkg/apis/beat/v1beta1"
	"github.com/elastic/cloud-on-k8s/test/e2e/test"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWebhook(t *testing.T) {
	beat := beatv1beta1.Beat{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-webhook",
			Namespace: test.Ctx().ManagedNamespace(0),
		},
		Spec: beatv1beta1.BeatSpec{
			Type:    "filebeat",
			Version: "7.8.0",
		},
	}

	err := test.NewK8sClientOrFatal().Client.Create(&beat)

	require.Error(t, err)
	require.Contains(
		t,
		err.Error(),
		`admission webhook "elastic-beat-validation-v1beta1.k8s.elastic.co" denied the request: Beat.beat.k8s.elastic.co "test-webhook" is invalid`,
	)
}
