// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package bindmount

import (
	"testing"

	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/cmdutil"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/flex"
	"github.com/stretchr/testify/assert"
)

func TestDriver_Init(t *testing.T) {
	type fields struct {
		factoryFunc cmdutil.ExecutableFactory
		mountPath   string
	}
	tests := []struct {
		name   string
		fields fields
		want   flex.Response
	}{
		{
			name: "init",
			want: flex.Response{
				Status:  flex.StatusSuccess,
				Message: "driver is available",
				Capabilities: flex.Capabilities{
					Attach: false, // only implement mount and unmount
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Driver{
				factoryFunc: tt.fields.factoryFunc,
				mountPath:   tt.fields.mountPath,
			}
			got := d.Init()
			assert.Equal(t, tt.want, got)
		})
	}
}
