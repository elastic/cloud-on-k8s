package diskutil

import (
	"errors"
	"path"
	"testing"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/cmdutil"
	"github.com/stretchr/testify/assert"
)

func TestFormatDevice(t *testing.T) {
	var success = cmdutil.StubExecutable{}
	var failure = cmdutil.StubExecutable{
		Bytes: []byte("some output"),
		Err:   errors.New("some error"),
	}
	type args struct {
		newCmd     cmdutil.FactoryFunc
		devicePath string
		fstype     string
		exec       cmdutil.Executable
	}
	tests := []struct {
		name        string
		args        args
		wantCommand []string
		err         error
	}{
		{
			name: "succeeds",
			args: args{
				newCmd:     cmdutil.NewStubFactoryFunc(&success),
				exec:       &success,
				devicePath: path.Join("some", "path"),
				fstype:     "xfs",
			},
			wantCommand: []string{"mkfs", "-t", "xfs", "some/path"},
		},
		{
			name: "failure",
			args: args{
				newCmd:     cmdutil.NewStubFactoryFunc(&failure),
				exec:       &failure,
				devicePath: path.Join("some", "path"),
				fstype:     "xfs",
			},
			wantCommand: []string{"mkfs", "-t", "xfs", "some/path"},
			err:         errors.New("some error. Output: some output"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := FormatDevice(tt.args.newCmd, tt.args.devicePath, tt.args.fstype)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.wantCommand, tt.args.exec.Command())
		})
	}
}
