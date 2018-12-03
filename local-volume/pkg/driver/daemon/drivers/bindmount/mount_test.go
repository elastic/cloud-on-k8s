package bindmount

import (
	"errors"
	"os"
	"testing"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/cmdutil"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/flex"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/protocol"
	"github.com/stretchr/testify/assert"
)

func TestDriver_Mount(t *testing.T) {
	type fields struct {
		factoryFunc cmdutil.FactoryFunc
		mountPath   string
	}
	type args struct {
		params protocol.MountRequest
		cb     func()
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
				factoryFunc: cmdutil.NewStubFactoryFunc(&cmdutil.StubExecutable{
					Bytes: []byte("some output"),
				}),
			},
			args: args{params: protocol.MountRequest{TargetDir: "/tmp"}},
			want: flex.Success("successfully created the volume"),
		},
		{
			name:   "failure due to failed permissions on mountPath",
			fields: fields{mountPath: "/"},
			args:   args{params: protocol.MountRequest{TargetDir: "etc/hosts"}},
			want:   flex.Failure("cannot ensure source directory: mkdir /hosts: permission denied"),
		},
		{
			name: "failure due to missing targetDir",
			args: args{params: protocol.MountRequest{TargetDir: "/some/path/here"}, cb: func() { os.RemoveAll("here") }},
			want: flex.Failure("cannot ensure target directory: mkdir /some/path/here: no such file or directory"),
		},
		{
			name: "failure due to command failure",
			fields: fields{
				mountPath: "/",
				factoryFunc: cmdutil.NewStubFactoryFunc(&cmdutil.StubExecutable{
					Bytes: []byte("some output"),
					Err:   errors.New("an error on the cmd layer"),
				}),
			},
			args: args{params: protocol.MountRequest{TargetDir: "/tmp"}},
			want: flex.Failure("cannot bind mount /tmp to /tmp: an error on the cmd layer. Output: some output"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Driver{
				factoryFunc: tt.fields.factoryFunc,
				mountPath:   tt.fields.mountPath,
			}
			got := d.Mount(tt.args.params)
			assert.Equal(t, tt.want, got)
			if tt.args.cb != nil {
				tt.args.cb()
			}
		})
	}
}
