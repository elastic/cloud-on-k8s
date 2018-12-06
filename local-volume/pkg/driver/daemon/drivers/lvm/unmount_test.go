package lvm

import (
	"errors"
	"path"
	"testing"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/cmdutil"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/flex"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/protocol"
	"github.com/stretchr/testify/assert"
)

func TestDriver_Unmount(t *testing.T) {
	var (
		success = &cmdutil.FakeExecutables{Stubs: []*cmdutil.FakeExecutable{
			{},
		}}
		cmdErrorFailure = &cmdutil.FakeExecutables{Stubs: []*cmdutil.FakeExecutable{
			{Bytes: []byte("an output"), Err: errors.New("error")},
		}}
	)
	type fields struct {
		options  Options
		fakeExec *cmdutil.FakeExecutables
	}
	type args struct {
		params protocol.UnmountRequest
	}
	tests := []struct {
		name         string
		fields       fields
		args         args
		want         flex.Response
		wantCommands [][]string
	}{
		{
			name: "success",
			fields: fields{
				fakeExec: success,
				options: Options{
					ExecutableFactory: success.ExecutableFactory(),
				},
			},
			args:         args{params: protocol.UnmountRequest{TargetDir: path.Join("some", "volume", "path")}},
			wantCommands: [][]string{[]string{"umount", "some/volume/path"}},
			want:         flex.Success("successfully unmounted the volume"),
		},
		{
			name: "failure due to cmd error",
			fields: fields{
				fakeExec: cmdErrorFailure,
				options: Options{
					ExecutableFactory: cmdErrorFailure.ExecutableFactory(),
				},
			},
			args:         args{params: protocol.UnmountRequest{TargetDir: path.Join("some", "path")}},
			wantCommands: [][]string{[]string{"umount", "some/path"}},
			want:         flex.Failure("Cannot unmount volume some/path: error. Output: an output"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Driver{
				options: tt.fields.options,
			}
			got := d.Unmount(tt.args.params)
			assert.Equal(t, tt.want, got)
			if tt.fields.fakeExec != nil {
				assert.Equal(
					t,
					tt.wantCommands,
					tt.fields.fakeExec.RecordedExecution(),
				)
			}
		})
	}
}
