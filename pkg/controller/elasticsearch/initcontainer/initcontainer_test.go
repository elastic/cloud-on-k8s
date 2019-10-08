// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package initcontainer

import (
	"testing"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/keystore"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	"github.com/stretchr/testify/assert"
)

func TestNewInitContainers(t *testing.T) {
	type args struct {
		elasticsearchImage string
		operatorImage      string
		keystoreResources  *keystore.Resources
	}
	tests := []struct {
		name                       string
		args                       args
		expectedNumberOfContainers int
	}{
		{
			name: "with keystore resources",
			args: args{
				elasticsearchImage: "es-image",
				operatorImage:      "op-image",
				keystoreResources:  &keystore.Resources{},
			},
			expectedNumberOfContainers: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			containers, err := NewInitContainers(
				tt.args.elasticsearchImage,
				volume.SecretVolume{},
				"clustername",
				tt.args.keystoreResources,
			)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedNumberOfContainers, len(containers))
		})
	}
}
