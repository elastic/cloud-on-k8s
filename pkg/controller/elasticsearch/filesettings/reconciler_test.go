// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package filesettings

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/utils/k8s"
)

func Test_ReconcileEmptyFileSettingsSecret(t *testing.T) {
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{
		Namespace: "esNs",
		Name:      "esName",
	}}

	fakeClient := k8s.NewFakeClient()

	err := ReconcileEmptyFileSettingsSecret(context.Background(), fakeClient, es, true)
	assert.NoError(t, err)

	var secret corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "esNs", Name: "esName-es-file-settings"}, &secret)
	assert.NoError(t, err)

	err = ReconcileEmptyFileSettingsSecret(context.Background(), fakeClient, es, true)
	assert.NoError(t, err)

	var secret2 corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "esNs", Name: "esName-es-file-settings"}, &secret2)
	assert.NoError(t, err)
	assert.Equal(t, secret2.ResourceVersion, secret.ResourceVersion)

	err = ReconcileEmptyFileSettingsSecret(context.Background(), fakeClient, es, false)
	assert.NoError(t, err)

	var secret3 corev1.Secret
	err = fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "esNs", Name: "esName-es-file-settings"}, &secret3)
	assert.NoError(t, err)
	assert.Greater(t, secret3.ResourceVersion, secret.ResourceVersion)
}
