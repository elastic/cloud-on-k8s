// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package initcontainer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewInitContainers(t *testing.T) {
	type args struct {
		imageName        string
		linkedFiles      LinkedFilesArray
		SetVMMaxMapCount bool
	}
	tests := []struct {
		name                       string
		args                       args
		expectedNumberOfContainers int
	}{
		{
			name: "with SetVMMaxMapCount enabled",
			args: args{
				imageName:        "image",
				linkedFiles:      LinkedFilesArray{},
				SetVMMaxMapCount: true,
			},
			expectedNumberOfContainers: 2,
		},
		{
			name: "with SetVMMaxMapCount disabled",
			args: args{
				imageName:        "image",
				linkedFiles:      LinkedFilesArray{},
				SetVMMaxMapCount: false,
			},
			expectedNumberOfContainers: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			containers, err := NewInitContainers(tt.args.imageName, tt.args.linkedFiles, tt.args.SetVMMaxMapCount)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedNumberOfContainers, len(containers))
		})
	}
}
