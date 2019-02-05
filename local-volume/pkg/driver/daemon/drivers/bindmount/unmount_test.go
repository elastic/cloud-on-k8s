// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package bindmount

import (
	"errors"
	"reflect"
	"testing"

	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/cmdutil"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/flex"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/protocol"
)

func TestDriver_Unmount(t *testing.T) {
	type fields struct {
		factoryFunc cmdutil.ExecutableFactory
		mountPath   string
	}
	type args struct {
		params protocol.UnmountRequest
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   flex.Response
	}{
		{
			name: "Success",
			fields: fields{
				mountPath: "/",
				factoryFunc: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					Bytes: []byte("some output"),
				}),
			},
			args: args{params: protocol.UnmountRequest{TargetDir: "/tmp"}},
			want: flex.Success("successfully removed the volume"),
		},
		{
			name: "Success",
			fields: fields{
				mountPath: "/",
				factoryFunc: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					Bytes: []byte("some output"),
					Err:   errors.New("some error"),
				}),
			},
			args: args{params: protocol.UnmountRequest{TargetDir: "/tmp"}},
			want: flex.Failure("Cannot unmount volume /tmp: some error. Output: some output"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Driver{
				factoryFunc: tt.fields.factoryFunc,
				mountPath:   tt.fields.mountPath,
			}
			if got := d.Unmount(tt.args.params); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Driver.Unmount() = %v, want %v", got, tt.want)
			}
		})
	}
}
