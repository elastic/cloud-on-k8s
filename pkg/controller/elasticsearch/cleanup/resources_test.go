// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package cleanup

import (
	"context"
	"testing"
	"time"

	esv1 "github.com/elastic/cloud-on-k8s/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/label"
	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsTooYoungForGC(t *testing.T) {
	tests := []struct {
		name               string
		objectCreationTime time.Time
		want               bool
	}{
		{
			name:               "object that was just created",
			objectCreationTime: time.Now().Add(-1 * time.Minute),
			want:               true,
		},
		{
			name:               "object created in the future (edge case)",
			objectCreationTime: time.Now().Add(1 * time.Hour),
			want:               true,
		},
		{
			name:               "object created a while ago",
			objectCreationTime: time.Now().Add(-DeleteAfter).Add(-1 * time.Minute),
			want:               false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.NewTime(tt.objectCreationTime),
				},
			}
			if got := IsTooYoungForGC(&obj); got != tt.want {
				t.Errorf("IsTooYoungForGC() = %v, want %v", got, tt.want)
			}
		})
	}
}

func secret(name string, clusterName string, podRef string, creationTime time.Time) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns1",
			Name:      name,
			Labels: map[string]string{
				label.ClusterNameLabelName: clusterName,
				label.PodNameLabelName:     podRef,
			},
			CreationTimestamp: metav1.NewTime(creationTime),
		},
	}
}

func TestDeleteOrphanedSecrets(t *testing.T) {
	now := time.Now()
	whileAgo := time.Now().Add(-DeleteAfter).Add(-1 * time.Minute)

	es := esv1.Elasticsearch{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns1",
			Name:      "es1",
		},
	}
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns1",
			Name:      "pod1",
			Labels: map[string]string{
				label.ClusterNameLabelName: es.Name,
			},
		},
	}

	tests := []struct {
		name                string
		client              k8s.Client
		es                  esv1.Elasticsearch
		secretsAfterCleanup []*corev1.Secret
	}{
		{
			name:                "nothing in the cache",
			client:              k8s.NewFakeClient(),
			es:                  es,
			secretsAfterCleanup: nil,
		},
		{
			name: "nothing to delete, pod exists",
			client: k8s.NewFakeClient(
				&pod,
				secret("s", es.Name, pod.Name, whileAgo),
			),
			es: es,
			secretsAfterCleanup: []*corev1.Secret{
				secret("s", es.Name, pod.Name, whileAgo),
			},
		},
		{
			name: "2 secrets to cleanup but not old enough",
			client: k8s.NewFakeClient(
				secret("s1", es.Name, pod.Name, now),
				secret("s2", es.Name, pod.Name, now),
			),
			es: es,
			secretsAfterCleanup: []*corev1.Secret{
				secret("s1", es.Name, pod.Name, now),
				secret("s2", es.Name, pod.Name, now),
			},
		},
		{
			name: "2 secrets to cleanup for the same pod",
			client: k8s.NewFakeClient(
				secret("s1", es.Name, pod.Name, whileAgo),
				secret("s2", es.Name, pod.Name, whileAgo),
			),
			es:                  es,
			secretsAfterCleanup: nil,
		},
		{
			name: "2 secrets to cleanup for different pods",
			client: k8s.NewFakeClient(
				secret("s1", es.Name, pod.Name, whileAgo),
				secret("s2", es.Name, "pod2", whileAgo),
			),
			es:                  es,
			secretsAfterCleanup: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := DeleteOrphanedSecrets(context.Background(), tt.client, tt.es)
			require.NoError(t, err)
			// the correct number of secrets should remain in the cache
			var secrets corev1.SecretList
			err = tt.client.List(context.Background(), &secrets)
			require.NoError(t, err)
			require.Equal(t, len(tt.secretsAfterCleanup), len(secrets.Items))
			// remaining secret should be the expected ones
			for _, expected := range tt.secretsAfterCleanup {
				var actual corev1.Secret
				err = tt.client.Get(context.Background(), k8s.ExtractNamespacedName(expected), &actual)
				require.NoError(t, err)
			}
		})
	}
}
