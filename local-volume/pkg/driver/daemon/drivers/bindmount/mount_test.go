package bindmount

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/cmdutil"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/flex"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/protocol"
	"github.com/stretchr/testify/assert"
)

func createPathAndDelete(t *testing.T, p string) (string, func()) {
	createdPath, err := ioutil.TempDir("", p)
	if err != nil {
		t.Fatal(err)
	}
	return createdPath, func() {
		if err := os.RemoveAll(createdPath); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDriver_Mount(t *testing.T) {
	type fields struct {
		factoryFunc cmdutil.ExecutableFactory
		mountPath   string
		f           func(*testing.T, string) (string, func())
	}
	type args struct {
		f      func(*testing.T, string) (string, func())
		params protocol.MountRequest
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
				mountPath: os.TempDir(),
				factoryFunc: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					Bytes: []byte("some output"),
				}),
			},
			args: args{params: protocol.MountRequest{TargetDir: "/tmp"}, f: createPathAndDelete},
			want: flex.Success("successfully created the volume"),
		},
		{
			name: "failure due to command failure",
			fields: fields{
				mountPath: os.TempDir(),
				factoryFunc: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					Bytes: []byte("some output"),
					Err:   errors.New("an error on the cmd layer"),
				}),
			},
			args: args{params: protocol.MountRequest{TargetDir: "/tmp"}},
			want: flex.Failure(fmt.Sprint(
				"cannot bind mount ",
				path.Join(os.TempDir(), "tmp"),
				" to /tmp: an error on the cmd layer. Output: some output",
			)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.fields.f != nil {
				p, f := tt.fields.f(t, tt.fields.mountPath)
				defer f()
				tt.fields.mountPath = p

			}
			d := &Driver{
				factoryFunc: tt.fields.factoryFunc,
				mountPath:   tt.fields.mountPath,
			}
			if tt.args.f != nil {
				p, f := tt.args.f(t, tt.args.params.TargetDir)
				defer f()
				tt.args.params.TargetDir = p
			}
			got := d.Mount(tt.args.params)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDriver_Mount_MissingMountPath(t *testing.T) {
	d := &Driver{
		mountPath: path.Join(os.TempDir(), "somepath", "hosts"),
	}
	want := flex.Failure(fmt.Sprint("cannot ensure source directory: mkdir ",
		path.Join(os.TempDir(), "somepath", "hosts", "hosts"),
		": no such file or directory",
	))
	got := d.Mount(
		protocol.MountRequest{TargetDir: "some/hosts"},
	)
	assert.Equal(t, want, got)
}

func TestDriver_Mount_MissingTargetPath(t *testing.T) {
	mountPath, cleanup := createPathAndDelete(t, "somepath")
	defer cleanup()
	d := &Driver{mountPath: mountPath}
	want := flex.Failure(fmt.Sprint("cannot ensure target directory: mkdir ",
		path.Join(os.TempDir(), "some", "unexisting", "path"),
		": no such file or directory",
	))
	got := d.Mount(protocol.MountRequest{
		TargetDir: path.Join(os.TempDir(), "some", "unexisting", "path"),
	})
	assert.Equal(t, want, got)
}
