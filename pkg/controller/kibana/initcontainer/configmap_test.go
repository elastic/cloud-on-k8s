// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package initcontainer

import (
	"fmt"
	"testing"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
)

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
				if *v.Volume().ConfigMap.DefaultMode != 0755 {
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
