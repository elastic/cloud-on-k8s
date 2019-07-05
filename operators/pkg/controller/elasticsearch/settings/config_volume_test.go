// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"reflect"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	common "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestConfigSecretName(t *testing.T) {
	require.Equal(t, "ssetName-es-config", ConfigSecretName("ssetName"))
}

func TestGetESConfigContent(t *testing.T) {
	namespace := "namespace"
	ssetName := "sset"
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigSecretName(ssetName),
			Namespace: namespace,
		},
		Data: map[string][]byte{
			ConfigFileName: []byte("a: b\nc: d\n"),
		},
	}
	secretInvalid := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigSecretName(ssetName),
			Namespace: namespace,
		},
		Data: map[string][]byte{
			ConfigFileName: []byte("yolo"),
		},
	}
	tests := []struct {
		name      string
		client    k8s.Client
		namespace string
		ssetName  string
		want      CanonicalConfig
		wantErr   bool
	}{
		{
			name:      "valid config exists",
			client:    k8s.WrapClient(fake.NewFakeClient(&secret)),
			namespace: namespace,
			ssetName:  ssetName,
			want:      CanonicalConfig{common.MustCanonicalConfig(map[string]string{"a": "b", "c": "d"})},
			wantErr:   false,
		},
		{
			name:      "config does not exist",
			client:    k8s.WrapClient(fake.NewFakeClient()),
			namespace: namespace,
			ssetName:  ssetName,
			want:      CanonicalConfig{},
			wantErr:   true,
		},
		{
			name:      "stored config is invalid",
			client:    k8s.WrapClient(fake.NewFakeClient(&secretInvalid)),
			namespace: namespace,
			ssetName:  ssetName,
			want:      CanonicalConfig{},
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetESConfigContent(tt.client, tt.namespace, tt.ssetName)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetESConfigContent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetESConfigContent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReconcileConfig(t *testing.T) {
	err := v1alpha1.AddToScheme(scheme.Scheme)
	assert.NoError(t, err)
	clusterName := "cluster"
	sset := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace",
			Name:      "sset",
		},
	}
	config := CanonicalConfig{common.MustCanonicalConfig(map[string]string{"a": "b", "c": "d"})}
	rendered, err := config.Render()
	require.NoError(t, err)
	configSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: sset.Namespace,
			Name:      ConfigSecretName(sset.Name),
			Labels: map[string]string{
				label.ClusterNameLabelName:     clusterName,
				label.StatefulSetNameLabelName: sset.Name,
			},
		},
		Data: map[string][]byte{
			ConfigFileName: rendered,
		},
	}
	tests := []struct {
		name        string
		client      k8s.Client
		clusterName string
		sset        appsv1.StatefulSet
		config      CanonicalConfig
		wantErr     bool
	}{
		{
			name:        "config does not exist",
			client:      k8s.WrapClient(fake.NewFakeClient()),
			clusterName: clusterName,
			sset:        sset,
			config:      config,
			wantErr:     false,
		},
		{
			name:        "config already exists",
			client:      k8s.WrapClient(fake.NewFakeClient(&configSecret)),
			clusterName: clusterName,
			sset:        sset,
			config:      config,
			wantErr:     false,
		},
		{
			name:        "config should be updated",
			client:      k8s.WrapClient(fake.NewFakeClient(&configSecret)),
			clusterName: clusterName,
			sset:        sset,
			config:      CanonicalConfig{common.MustCanonicalConfig(map[string]string{"a": "b", "c": "different"})},
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ReconcileConfig(tt.client, tt.clusterName, tt.sset, tt.config); (err != nil) != tt.wantErr {
				t.Errorf("ReconcileConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			// config in the apiserver should be the expected one
			parsed, err := GetESConfigContent(tt.client, sset.Namespace, sset.Name)
			require.NoError(t, err)
			require.Equal(t, tt.config, parsed)
		})
	}
}
