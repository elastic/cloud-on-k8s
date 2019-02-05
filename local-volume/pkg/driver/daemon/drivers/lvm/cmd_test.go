// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package lvm

import (
	"errors"
	"testing"

	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/cmdutil"
	"github.com/stretchr/testify/assert"
)

func TestRunLVMCmd(t *testing.T) {
	var randomStdOutput = `{"report":[{"lv":[{"lv_name":"aa","vg_name":"bb","lv_path":"cc","lv_size":"1234","lv_tags":"tt","lv_layout":"ll","data_percent":"23"}]}]}`
	type args struct {
		cmd    cmdutil.Executable
		result interface{}
	}
	tests := []struct {
		name string
		args args
		err  error
	}{
		{
			name: "Succeeds on JSON output",
			args: args{
				result: &lvsOutput{},
				cmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					StdOutput: []byte(randomStdOutput),
				})("command", "argument"),
			},
		},
		{
			name: "Fails on non JSON output",
			args: args{
				result: &lvsOutput{},
				cmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					StdOutput: []byte(`eeeeekkkk`),
				})("command", "argument"),
			},
			err: errors.New("cannot parse cmd output: invalid character 'e' looking for beginning of value eeeeekkkk"),
		},
		{
			name: "Fails on insufficient free space",
			args: args{
				cmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					Err: errors.New("insufficient free space"),
				})("command", "argument"),
			},
			err: ErrNoSpace,
		},
		{
			name: "Fails on logical volume not found",
			args: args{
				cmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					Err: errors.New("Failed to find logical volume"),
				})("command", "argument"),
			},
			err: ErrLogicalVolumeNotFound,
		},
		{
			name: "Fails on volume group not found",
			args: args{
				cmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					Err: errors.New("Volume group something not found"),
				})("command", "argument"),
			},
			err: ErrVolumeGroupNotFound,
		},
		{
			name: "Fails on insuficient devices",
			args: args{
				cmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					Err: errors.New("Insufficient suitable allocatable extents for logical volume"),
				})("command", "argument"),
			},
			err: ErrTooFewDisks,
		},
		{
			name: "Fails unkown error",
			args: args{
				cmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					StdOutput: []byte("Something unexpected happened"),
					Err:       errors.New("Unknown error mate"),
				})("command", "argument"),
			},
			err: errors.New("Something unexpected happened"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RunLVMCmd(tt.args.cmd, tt.args.result)
			assert.Equal(t, tt.err, err)
		})
	}
}
