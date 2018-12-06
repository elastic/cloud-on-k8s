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
