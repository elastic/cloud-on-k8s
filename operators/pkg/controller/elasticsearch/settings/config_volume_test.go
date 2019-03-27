// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package settings

import (
	"reflect"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/label"

	"k8s.io/client-go/kubernetes/scheme"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestConfigSecretName(t *testing.T) {
	require.Equal(t, "mypod-config", ConfigSecretName("mypod"))
}

func TestGetESConfigContent(t *testing.T) {
	pod := types.NamespacedName{
		Name:      "pod",
		Namespace: "namespace",
	}
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-config",
			Namespace: "namespace",
		},
		Data: map[string][]byte{
			ConfigFileName: []byte("a: b\nc: d\n"),
		},
	}
	secretInvalid := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-config",
			Namespace: "namespace",
		},
		Data: map[string][]byte{
			ConfigFileName: []byte("yolo"),
		},
	}
	tests := []struct {
		name    string
		client  k8s.Client
		esPod   types.NamespacedName
		want    FlatConfig
		wantErr bool
	}{
		{
			name:    "valid config exists",
			client:  k8s.WrapClient(fake.NewFakeClient(&secret)),
			esPod:   pod,
			want:    FlatConfig{"a": "b", "c": "d"},
			wantErr: false,
		},
		{
			name:    "config does not exist",
			client:  k8s.WrapClient(fake.NewFakeClient()),
			esPod:   pod,
			want:    FlatConfig{},
			wantErr: true,
		},
		{
			name:    "stored config is invalid",
			client:  k8s.WrapClient(fake.NewFakeClient(&secretInvalid)),
			esPod:   pod,
			want:    FlatConfig{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetESConfigContent(tt.client, tt.esPod)
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
	v1alpha1.AddToScheme(scheme.Scheme)
	cluster := v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace",
			Name:      "cluster",
		},
	}
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace",
			Name:      "pod",
		},
	}
	config := FlatConfig{"a": "b", "c": "d"}
	configSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: pod.Namespace,
			Name:      ConfigSecretName(pod.Name),
			Labels: map[string]string{
				label.ClusterNameLabelName: cluster.Name,
				label.PodNameLabelName:     pod.Name,
			},
		},
		Data: map[string][]byte{
			ConfigFileName: config.Render(),
		},
	}
	tests := []struct {
		name    string
		client  k8s.Client
		cluster v1alpha1.Elasticsearch
		pod     corev1.Pod
		config  FlatConfig
		wantErr bool
	}{
		{
			name:    "config does not exist",
			client:  k8s.WrapClient(fake.NewFakeClient()),
			cluster: cluster,
			pod:     pod,
			config:  config,
			wantErr: false,
		},
		{
			name:    "config already exists",
			client:  k8s.WrapClient(fake.NewFakeClient(&configSecret)),
			cluster: cluster,
			pod:     pod,
			config:  config,
			wantErr: false,
		},
		{
			name:    "config should be updated",
			client:  k8s.WrapClient(fake.NewFakeClient(&configSecret)),
			cluster: cluster,
			pod:     pod,
			config:  FlatConfig{"a": "b", "c": "different"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ReconcileConfig(tt.client, tt.cluster, tt.pod, tt.config); (err != nil) != tt.wantErr {
				t.Errorf("ReconcileConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			// config in the apiserver should be the expected one
			parsed, err := GetESConfigContent(tt.client, k8s.ExtractNamespacedName(&pod))
			require.NoError(t, err)
			require.Equal(t, tt.config, parsed)
		})
	}
}
