// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package initcontainer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/volume"
)

func TestNewInitContainers(t *testing.T) {
	type args struct {
		keystoreResources *keystore.Resources
	}
	tests := []struct {
		name                       string
		args                       args
		expectedNumberOfContainers int
	}{
		{
			name: "with keystore resources (init container)",
			args: args{
				keystoreResources: &keystore.Resources{
					InitContainer: corev1.Container{Name: "keystore-init"},
				},
			},
			expectedNumberOfContainers: 3,
		},
		{
			name: "with keystore resources (no init container - pre-built)",
			args: args{
				keystoreResources: &keystore.Resources{
					// Empty InitContainer means no init container needed
					InitContainer: corev1.Container{},
				},
			},
			expectedNumberOfContainers: 2,
		},
		{
			name: "without keystore resources",
			args: args{
				keystoreResources: nil,
			},
			expectedNumberOfContainers: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			containers, err := NewInitContainers(volume.SecretVolume{}, tt.args.keystoreResources, []string{})
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedNumberOfContainers, len(containers))
		})
	}
}
