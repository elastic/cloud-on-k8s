// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package configmap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/initcontainer"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/nodespec"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestReconcileScriptsConfigMap(t *testing.T) {
	// Setup common test variables
	namespace := "ns1"
	esName := "test-es"
	configMapName := "test-es-es-scripts"
	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Name:      esName,
			Namespace: namespace,
		},
	}

	tests := []struct {
		name           string
		initialObjects []client.Object
		meta           metadata.Metadata
		wantErr        bool
		validate       func(t *testing.T, client k8s.Client)
	}{
		{
			name:           "creates a new config map when it doesn't exist",
			initialObjects: []client.Object{},
			meta: metadata.Metadata{
				Labels:      map[string]string{"label1": "value1"},
				Annotations: map[string]string{"annotation1": "value1"},
			},
			validate: func(t *testing.T, client k8s.Client) {
				t.Helper()
				var createdConfigMap corev1.ConfigMap
				err := client.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: configMapName}, &createdConfigMap)
				assert.NoError(t, err)
				assert.Equal(t, configMapName, createdConfigMap.Name)
				assert.Equal(t, namespace, createdConfigMap.Namespace)
				assert.Equal(t, map[string]string{"label1": "value1"}, createdConfigMap.Labels)
				assert.Equal(t, map[string]string{"annotation1": "value1"}, createdConfigMap.Annotations)

				// Verify content of the config map
				assert.Contains(t, createdConfigMap.Data, nodespec.LegacyReadinessProbeScriptConfigKey)
				assert.Contains(t, createdConfigMap.Data, nodespec.ReadinessPortProbeScriptConfigKey)
				assert.Contains(t, createdConfigMap.Data, nodespec.PreStopHookScriptConfigKey)
				assert.Contains(t, createdConfigMap.Data, initcontainer.PrepareFsScriptConfigKey)
				assert.Contains(t, createdConfigMap.Data, initcontainer.SuspendScriptConfigKey)
				assert.Contains(t, createdConfigMap.Data, initcontainer.SuspendedHostsFile)
			},
		},
		{
			name: "updates existing config map",
			initialObjects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:        configMapName,
						Namespace:   namespace,
						Labels:      map[string]string{"existing-label": "old-value"},
						Annotations: map[string]string{"existing-annotation": "old-value"},
					},
					Data: map[string]string{
						"existing-key": "existing-value",
					},
				},
			},
			meta: metadata.Metadata{
				Labels:      map[string]string{"label1": "value1"},
				Annotations: map[string]string{"annotation1": "value1"},
			},
			validate: func(t *testing.T, client k8s.Client) {
				t.Helper()
				var updatedConfigMap corev1.ConfigMap
				err := client.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: configMapName}, &updatedConfigMap)
				assert.NoError(t, err)

				// Labels should be updated
				assert.Equal(t, map[string]string{
					"existing-label": "old-value",
					"label1":         "value1",
				}, updatedConfigMap.Labels)
				// Annotations should be updated
				assert.Equal(t, map[string]string{
					"existing-annotation": "old-value",
					"annotation1":         "value1",
				}, updatedConfigMap.Annotations)

				// Data should be updated
				assert.NotContains(t, updatedConfigMap.Data, "old-key")
				assert.Contains(t, updatedConfigMap.Data, nodespec.PreStopHookScriptConfigKey)
				assert.Contains(t, updatedConfigMap.Data, initcontainer.PrepareFsScriptConfigKey)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock K8s client with initial objects
			mockClient := k8s.NewFakeClient(tt.initialObjects...)

			// Run the function (empty keystoreSecretMountPath for traditional init container approach)
			err := ReconcileScriptsConfigMap(context.Background(), mockClient, es, tt.meta, "")

			// Check error
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Run validation
				tt.validate(t, mockClient)
			}
		})
	}
}
