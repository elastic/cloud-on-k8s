package cmdutil

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunCmd(t *testing.T) {
	type args struct {
		cmd Executable
	}
	tests := []struct {
		name string
		args args
		err  error
	}{
		{
			name: "Runs a command succesfully",
			args: args{
				cmd: &FakeExecutable{
					name: "ls",
					args: []string{"-l"},
					Bytes: []byte(`total 16
					-rw-r--r--  1 some  root  919 Nov 28 12:19 cmd.go
					-rw-r--r--  1 some  root  760 Nov 28 12:21 cmd_test.go`),
				},
			},
		},
		{
			name: "Runs a command and fails due permissions",
			args: args{
				cmd: &FakeExecutable{
					name: "ls",
					args: []string{"-l"},
					Err:  errors.New(`ls: .: Operation not permitted`),
				},
			},
			err: errors.New(`ls: .: Operation not permitted. Output: `),
		},
		{
			name: "Runs a command and fails due permissions",
			args: args{
				cmd: &FakeExecutable{
					name:  "ls",
					args:  []string{"-l"},
					Bytes: []byte(`total 16`),
					Err:   errors.New(`ls: cmd_some.go: Operation not permitted`),
				},
			},
			err: errors.New(`ls: cmd_some.go: Operation not permitted. Output: total 16`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RunCmd(tt.args.cmd)
			assert.Equal(t, tt.err, err)
		})
	}
}

func TestNewExecutableFactory_CominedOutput(t *testing.T) {
	type args struct {
		name string
		args []string
	}
	tests := []struct {
		name string
		args args
		want []byte
		err  string
	}{
		{
			name: "dummy command",
			args: args{
				"echo",
				[]string{"boom"},
			},
			want: []byte("boom\n"),
		},
		{
			name: "dummy command failure",
			args: args{
				"asdad",
				[]string{"some"},
			},
			err: `exec: "asdad": executable file not found in $PATH`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewExecutableFactory()(tt.args.name, tt.args.args...).CombinedOutput()
			assert.Equal(t, tt.want, got)
			if err != nil {
				assert.Equal(t, tt.err, err.Error())
			}
		})
	}
}

func TestNewExecutableFactory_Run(t *testing.T) {
	type args struct {
		name string
		args []string
	}
	tests := []struct {
		name    string
		args    args
		wantOut []byte
		wantErr []byte
		err     string
	}{
		{
			name: "dummy command",
			args: args{
				"echo",
				[]string{"boom"},
			},
			wantOut: []byte("boom\n"),
			wantErr: []byte(""),
		},
		{
			name: "dummy command failure",
			args: args{
				"asdad",
				[]string{"boom"},
			},
			err: `exec: "asdad": executable file not found in $PATH`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewExecutableFactory()(tt.args.name, tt.args.args...)
			if err := c.Run(); err != nil {
				assert.Equal(t, tt.err, err.Error(), "Run")
			}
			assert.Equal(t, tt.wantOut, c.StdOut(), "StdOut")
			assert.Equal(t, tt.wantErr, c.StdErr(), "StdErr")
		})
	}
}
