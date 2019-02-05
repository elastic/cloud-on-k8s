package lvm

import (
	"errors"
	"testing"

	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/cmdutil"
	"github.com/stretchr/testify/assert"
)

func TestLogicalVolume_Path(t *testing.T) {
	var lvsOutput = `{"report":[{"lv":[{"lv_name":"anlv","vg_name":"avg","lv_path":"/dev/anlv","lv_size":"2048","lv_tags":"tt","lv_layout":"ll","data_percent":"23"}]}]}`
	type fields struct {
		name        string
		sizeInBytes uint64
		vg          VolumeGroup
	}
	type args struct {
		newCmd cmdutil.ExecutableFactory
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   string
		err    error
	}{
		{
			name: "Succeeds",
			fields: fields{
				name:        "anlv",
				sizeInBytes: 2048,
				vg:          VolumeGroup{name: "avg", bytesFree: 4096},
			},
			args: args{newCmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
				StdOutput: []byte(lvsOutput),
			})},
			want: "/dev/anlv",
		},
		{
			name: "fails due to empty result leads to error",
			fields: fields{
				name:        "anlv",
				sizeInBytes: 2048,
				vg:          VolumeGroup{name: "avg", bytesFree: 4096},
			},
			args: args{newCmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
				StdOutput: []byte(`{}`),
			})},
			want: "",
			err:  ErrLogicalVolumeNotFound,
		},
		{
			name: "fails due to cmd error",
			fields: fields{
				name:        "anlv",
				sizeInBytes: 2048,
				vg:          VolumeGroup{name: "avg", bytesFree: 4096},
			},
			args: args{newCmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
				StdOutput: []byte(`Something went wrong`),
				Err:       errors.New("some error"),
			})},
			want: "",
			err:  errors.New("Something went wrong"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lv := LogicalVolume{
				name:        tt.fields.name,
				sizeInBytes: tt.fields.sizeInBytes,
				vg:          tt.fields.vg,
			}
			got, err := lv.Path(tt.args.newCmd)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLogicalVolume_Remove(t *testing.T) {
	type fields struct {
		name        string
		sizeInBytes uint64
		vg          VolumeGroup
	}
	type args struct {
		newCmd cmdutil.ExecutableFactory
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		err    error
	}{
		{
			name:   "succeeds",
			fields: fields{},
			args: args{newCmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
				StdOutput: []byte(`{}`),
			})},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lv := LogicalVolume{
				name:        tt.fields.name,
				sizeInBytes: tt.fields.sizeInBytes,
				vg:          tt.fields.vg,
			}
			err := lv.Remove(tt.args.newCmd)
			assert.Equal(t, tt.err, err)
		})
	}
}

func TestValidateLogicalVolumeName(t *testing.T) {
	type args struct {
		name string
	}
	tests := []struct {
		name string
		args args
		err  error
	}{
		{
			name: "success",
			args: args{name: "aaa"},
		},
		{
			name: "failure",
			args: args{name: "aaa??"},
			err:  ErrInvalidLVName,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLogicalVolumeName(tt.args.name)
			assert.Equal(t, tt.err, err)
		})
	}
}
