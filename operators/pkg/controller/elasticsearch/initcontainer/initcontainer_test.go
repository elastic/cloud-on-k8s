// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package initcontainer

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/volume"
	"github.com/stretchr/testify/assert"
)

func TestNewInitContainers(t *testing.T) {
	varTrue := true
	varFalse := false
	type args struct {
		elasticsearchImage string
		operatorImage      string
		SetVMMaxMapCount   *bool
		keystoreResources  *keystore.Resources
	}
	tests := []struct {
		name                       string
		args                       args
		expectedNumberOfContainers int
	}{
		{
			name: "with SetVMMaxMapCount enabled",
			args: args{
				elasticsearchImage: "es-image",
				operatorImage:      "op-image",
				SetVMMaxMapCount:   &varTrue,
			},
			expectedNumberOfContainers: 2,
		},
		{
			name: "with SetVMMaxMapCount unspecified",
			args: args{
				elasticsearchImage: "es-image",
				operatorImage:      "op-image",
				SetVMMaxMapCount:   nil,
			},
			expectedNumberOfContainers: 2,
		},
		{
			name: "with SetVMMaxMapCount disabled",
			args: args{
				elasticsearchImage: "es-image",
				operatorImage:      "op-image",
				SetVMMaxMapCount:   &varFalse,
			},
			expectedNumberOfContainers: 1,
		},
		{
			name: "with keystore resources",
			args: args{
				elasticsearchImage: "es-image",
				operatorImage:      "op-image",
				SetVMMaxMapCount:   nil,
				keystoreResources:  &keystore.Resources{},
			},
			expectedNumberOfContainers: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			containers, err := NewInitContainers(
				tt.args.elasticsearchImage,
				tt.args.SetVMMaxMapCount,
				volume.SecretVolume{},
				"clusterName",
				tt.args.keystoreResources,
			)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedNumberOfContainers, len(containers))
		})
	}
}
