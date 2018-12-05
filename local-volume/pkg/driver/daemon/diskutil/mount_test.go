package diskutil

import (
	"errors"
	"path"
	"testing"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/cmdutil"
	"github.com/stretchr/testify/assert"
)

func TestBindMount(t *testing.T) {
	var success = cmdutil.FakeExecutable{}
	var failure = cmdutil.FakeExecutable{
		Bytes: []byte("some output"),
		Err:   errors.New("some error"),
	}
	type args struct {
		newCmd cmdutil.ExecutableFactory
		source string
		target string
		exec   cmdutil.Executable
	}
	tests := []struct {
		name        string
		args        args
		wantCommand []string
		err         error
	}{
		{
			name: "success",
			args: args{
				newCmd: cmdutil.NewFakeCmdBuilder(&success),
				source: path.Join("some", "source", "path"),
				target: path.Join("some", "target", "path"),
				exec:   &success,
			},
			wantCommand: []string{"mount", "--bind", "some/source/path", "some/target/path"},
		},
		{
			name: "failure",
			args: args{
				newCmd: cmdutil.NewFakeCmdBuilder(&failure),
				exec:   &success,
				source: path.Join("some", "source", "path"),
				target: path.Join("some", "target", "path"),
			},
			wantCommand: []string{"mount", "--bind", "some/source/path", "some/target/path"},
			err:         errors.New("some error. Output: some output"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := BindMount(tt.args.newCmd, tt.args.source, tt.args.target)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.wantCommand, tt.args.exec.Command())
		})
	}
}

func TestMountDevice(t *testing.T) {
	var success = cmdutil.FakeExecutable{}
	var failure = cmdutil.FakeExecutable{
		Bytes: []byte("some output"),
		Err:   errors.New("some error"),
	}
	type args struct {
		newCmd     cmdutil.ExecutableFactory
		devicePath string
		mountPath  string
		exec       cmdutil.Executable
	}
	tests := []struct {
		name        string
		args        args
		wantCommand []string
		err         error
	}{
		{
			name: "success",
			args: args{
				newCmd:     cmdutil.NewFakeCmdBuilder(&success),
				devicePath: path.Join("some", "source", "path"),
				mountPath:  path.Join("some", "target", "path"),
				exec:       &success,
			},
			wantCommand: []string{"mount", "some/source/path", "some/target/path"},
		},
		{
			name: "failure",
			args: args{
				newCmd:     cmdutil.NewFakeCmdBuilder(&failure),
				exec:       &success,
				devicePath: path.Join("some", "source", "path"),
				mountPath:  path.Join("some", "target", "path"),
			},
			wantCommand: []string{"mount", "some/source/path", "some/target/path"},
			err:         errors.New("some error. Output: some output"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MountDevice(tt.args.newCmd, tt.args.devicePath, tt.args.mountPath)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.wantCommand, tt.args.exec.Command())
		})
	}
}

func TestUnmount(t *testing.T) {
	var success = cmdutil.FakeExecutable{}
	var failure = cmdutil.FakeExecutable{
		Bytes: []byte("some output"),
		Err:   errors.New("some error"),
	}
	type args struct {
		newCmd    cmdutil.ExecutableFactory
		mountPath string
		exec      cmdutil.Executable
	}
	tests := []struct {
		name        string
		args        args
		wantCommand []string
		err         error
	}{
		{
			name: "success",
			args: args{
				newCmd:    cmdutil.NewFakeCmdBuilder(&success),
				mountPath: path.Join("some", "target", "path"),
				exec:      &success,
			},
			wantCommand: []string{"umount", "some/target/path"},
		},
		{
			name: "failure",
			args: args{
				newCmd:    cmdutil.NewFakeCmdBuilder(&failure),
				exec:      &success,
				mountPath: path.Join("some", "target", "path"),
			},
			wantCommand: []string{"umount", "some/target/path"},
			err:         errors.New("some error. Output: some output"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Unmount(tt.args.newCmd, tt.args.mountPath)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.wantCommand, tt.args.exec.Command())
		})
	}
}
