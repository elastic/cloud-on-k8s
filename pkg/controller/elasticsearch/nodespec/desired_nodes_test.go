// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nodespec

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	commonsettings "github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/settings"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/settings"
	esvolume "github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func bytesString(q string) string {
	qty := resource.MustParse(q)
	return fmt.Sprintf("%db", qty.Value())
}

// storageResources builds the minimal ResourcesList entry needed by ToDesiredNodes:
// - ES container with CPU limit+request and memory limit+request (equal, as required)
// - VolumeMount pointing path.data to the elasticsearch-data volume
// - path.data set in config
// - VolumeClaimTemplate for elasticsearch-data with the given storage size
func storageResources(storageSizeInVCT string) Resources {
	cpu := resource.MustParse("1")
	mem := resource.MustParse("2Gi")
	storage := resource.MustParse(storageSizeInVCT)

	replicas := int32(1)
	sts := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "es-data"},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "es"}},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name: esv1.ElasticsearchContainerName,
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    cpu,
								corev1.ResourceMemory: mem,
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    cpu,
								corev1.ResourceMemory: mem,
							},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      esvolume.ElasticsearchDataVolumeName,
							MountPath: esvolume.ElasticsearchDataMountPath,
						}},
					}},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{Name: esvolume.ElasticsearchDataVolumeName},
				Spec: corev1.PersistentVolumeClaimSpec{
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceStorage: storage},
					},
				},
			}},
		},
	}

	cfg := settings.CanonicalConfig{
		CanonicalConfig: commonsettings.MustCanonicalConfig(map[string]any{
			"path.data": esvolume.ElasticsearchDataMountPath,
		}),
	}

	return Resources{
		NodeSet:         "data",
		StatefulSet:     sts,
		Config:          cfg,
		HeadlessService: corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "es-data"}},
	}
}

// TestStorageShorthandFlowsToDesiredNodes verifies the resources.storage → StatefulSet VCT →
// desired nodes API chain. The autoscaling contract writes resources.storage without touching
// VolumeClaimTemplates; BuildStatefulSet calls ApplyStorageOverride which stamps the size into
// the VCT; ToDesiredNodes then reads it back via claimedStorage (PVCs don't exist yet in a
// brand-new cluster, so the code falls back to the VCT spec).
func TestStorageShorthandFlowsToDesiredNodes(t *testing.T) {
	tests := []struct {
		name        string
		vctStorage  string // storage size stamped into the StatefulSet VCT (as ApplyStorageOverride would produce)
		wantStorage string // expected Storage field in the DesiredNode
		wantRequeue bool
	}{
		{
			// The autoscaler writes resources.storage = 10Gi; ApplyStorageOverride stamps
			// that into the VCT; ToDesiredNodes must read 10Gi back.
			name:        "storage shorthand stamped by ApplyStorageOverride flows to desired nodes",
			vctStorage:  "10Gi",
			wantStorage: bytesString("10Gi"),
			wantRequeue: true, // PVC does not exist yet → requeue
		},
		{
			// Control: user declares storage directly in the VCT; same fallback path.
			name:        "direct VCT storage also reaches desired nodes",
			vctStorage:  "50Gi",
			wantStorage: bytesString("50Gi"),
			wantRequeue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := ResourcesList{storageResources(tt.vctStorage)}
			// Fake client returns NotFound for any PVC → withStorage falls back to claimedStorage.
			k8sClient := k8s.NewFakeClient()

			desiredNodes, requeue, err := rl.ToDesiredNodes(context.Background(), k8sClient, "8.16.0")
			require.NoError(t, err)
			assert.Equal(t, tt.wantRequeue, requeue)
			require.Len(t, desiredNodes, 1)
			assert.Equal(t, tt.wantStorage, desiredNodes[0].Storage)
		})
	}
}
