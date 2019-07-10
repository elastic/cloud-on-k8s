// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package reconcile

import (
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	common "github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/cleanup"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/pod"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/services"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/settings"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNewResourcesStateFromAPI_MissingPodConfiguration(t *testing.T) {
	// This test focuses on the edge case where
	// no configuration secret is found for a given pod.
	v1alpha1.AddToScheme(scheme.Scheme)
	ssetName := "sset"
	cluster := v1alpha1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "es",
		},
	}
	externalService := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      services.ExternalServiceName(cluster.Name),
		},
	}
	recentPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "ns",
			Name:              "pod",
			CreationTimestamp: metav1.NewTime(time.Now()),
		},
	}
	oldPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "pod",
			Labels: map[string]string{
				label.StatefulSetNameLabelName: ssetName,
			},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-cleanup.DeleteAfter).Add(-1 * time.Minute)),
		},
	}
	deletionTimestamp := metav1.NewTime(time.Now().Add(1 * time.Hour))
	deletingPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "pod",
			Labels: map[string]string{
				label.StatefulSetNameLabelName: ssetName,
			},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-cleanup.DeleteAfter).Add(-1 * time.Minute)),
			DeletionTimestamp: &deletionTimestamp,
		},
	}
	config := settings.CanonicalConfig{CanonicalConfig: common.MustNewSingleValue("a", "b")}
	rendered, err := config.Render()
	require.NoError(t, err)
	configSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      settings.ConfigSecretName(ssetName),
			Labels: map[string]string{
				label.ClusterNameLabelName:     cluster.Name,
				label.StatefulSetNameLabelName: oldPod.Name,
			},
		},
		Data: map[string][]byte{
			settings.ConfigFileName: rendered,
		},
	}

	tests := []struct {
		name             string
		c                k8s.Client
		es               v1alpha1.Elasticsearch
		wantCurrentPods  pod.PodsWithConfig
		wantDeletingPods pod.PodsWithConfig
		wantErr          string
	}{
		{
			name:            "configuration found",
			c:               k8s.WrapClient(fake.NewFakeClient(&cluster, &externalService, &oldPod, &configSecret)),
			es:              cluster,
			wantCurrentPods: pod.PodsWithConfig{{Pod: oldPod, Config: config}},
			wantErr:         "",
		},
		{
			name: "no configuration found, pod is terminating: continue with a dummy config",
			c:    k8s.WrapClient(fake.NewFakeClient(&cluster, &externalService, &deletingPod)),
			es:   cluster,
			wantDeletingPods: pod.PodsWithConfig{{Pod: deletingPod, Config: settings.CanonicalConfig{CanonicalConfig: common.MustNewSingleValue(
				"pod.deletion", "in.progress",
			)}}},
			wantErr: "",
		},
		{
			name:            "no configuration found, pod is recent: requeue",
			c:               k8s.WrapClient(fake.NewFakeClient(&cluster, &externalService, &recentPod)),
			es:              cluster,
			wantCurrentPods: nil,
			wantErr:         "configuration secret for pod pod not yet in the cache, re-queueing",
		},
		{
			name: "no configuration found, pod is old: should be associated a dummy config for replacement",
			c:    k8s.WrapClient(fake.NewFakeClient(&cluster, &externalService, &oldPod)),
			es:   cluster,
			wantCurrentPods: pod.PodsWithConfig{{Pod: oldPod, Config: settings.CanonicalConfig{CanonicalConfig: common.MustNewSingleValue(
				"error.pod.to.replace", "no configuration secret volume found for that pod, scheduling it for deletion",
			)}}},
			wantErr: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewResourcesStateFromAPI(tt.c, tt.es)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				require.Equal(t, len(tt.wantCurrentPods), len(got.CurrentPods))
				if len(tt.wantCurrentPods) > 0 {
					require.Equal(t, tt.wantCurrentPods[0].Config, got.CurrentPods[0].Config)
				}
				require.Equal(t, len(tt.wantDeletingPods), len(got.DeletingPods))
				if len(tt.wantDeletingPods) > 0 {
					require.Equal(t, tt.wantDeletingPods[0].Config, got.DeletingPods[0].Config)
				}
			}
		})
	}
}
