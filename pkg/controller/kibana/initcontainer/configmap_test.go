// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package initcontainer

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/common/v1"
	kbv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/kibana/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/metadata"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestReconcileScriptsConfigMap(t *testing.T) {
	kb := kbv1.Kibana{Name: "kb", Namespace: "ns"}
	meta := metadata.Metadata{Labels: map[string]string{"foo": "bar"}}
	c := k8s.NewFakeClient()

	require.NoError(t, ReconcileScriptsConfigMap(context.Background(), c, kb, meta))

	var cm corev1.ConfigMap
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: "ns", Name: kbv1.ScriptsConfigMap("kb")}, &cm))
	assert.Equal(t, "bar", cm.Labels["foo"])
	assert.Equal(t, commonv1.RestrictWatchedResourcesLabelValue, cm.Labels[commonv1.RestrictWatchedResourcesLabelName])
	assert.Contains(t, cm.Data, KibanaInitScriptConfigKey)
	// the caller's meta must not be mutated with the watched label (it is reused for the Kibana Deployment)
	assert.Equal(t, map[string]string{"foo": "bar"}, meta.Labels)
}

func TestNewScriptsConfigMapVolume(t *testing.T) {
	tests := []struct {
		name   string
		kbName string
		verify func(volume.ConfigMapVolume) error
	}{
		{
			name:   "returns expected volume name, mount path, and mode",
			kbName: "test-kb",
			verify: func(v volume.ConfigMapVolume) error {
				if v.Name() != "kibana-scripts" {
					return fmt.Errorf("unexpected name: %s", v.Name())
				}
				if v.VolumeMount().MountPath != "/mnt/elastic-internal/scripts" {
					return fmt.Errorf("unexpected mount path: %s", v.VolumeMount().MountPath)
				}
				if *v.Volume().ConfigMap.DefaultMode != 0o755 {
					return fmt.Errorf("unexpected default mode: %d", *v.Volume().ConfigMap.DefaultMode)
				}
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewScriptsConfigMapVolume(tt.kbName); tt.verify(got) != nil {
				t.Errorf("NewScriptsConfigMapVolume() = %s", tt.verify(got))
			}
		})
	}
}
