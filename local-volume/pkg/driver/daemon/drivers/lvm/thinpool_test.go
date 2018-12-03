package lvm

import (
	"errors"
	"testing"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/cmdutil"
	"github.com/stretchr/testify/assert"
)

func TestThinPool_CreateThinVolume(t *testing.T) {
	type fields struct {
		LogicalVolume LogicalVolume
		dataPercent   float64
	}
	type args struct {
		newCmd             cmdutil.FactoryFunc
		name               string
		virtualSizeInBytes uint64
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   LogicalVolume
		err    error
	}{
		{
			name: "success",
			fields: fields{
				LogicalVolume: LogicalVolume{
					name:        `LV`,
					sizeInBytes: 2048,
					vg:          VolumeGroup{name: "VG", bytesFree: 4096},
				},
			},
			args: args{
				newCmd:             cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{}),
				name:               "data",
				virtualSizeInBytes: 1024,
			},
			want: LogicalVolume{name: "data", sizeInBytes: 0x600, vg: VolumeGroup{name: "VG", bytesFree: 4096}},
		},
		{
			name: "failure due invalid name",
			fields: fields{
				LogicalVolume: LogicalVolume{
					name:        `LV`,
					sizeInBytes: 2048,
					vg:          VolumeGroup{name: "VG", bytesFree: 4096},
				},
			},
			args: args{
				newCmd:             cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{}),
				name:               "data??",
				virtualSizeInBytes: 1024,
			},
			err: errors.New("lvm: name contains invalid character, valid set includes: [A-Za-z0-9_+.-]"),
		},
		{
			name: "failure due cmd error",
			fields: fields{
				LogicalVolume: LogicalVolume{
					name:        `LV`,
					sizeInBytes: 2048,
					vg:          VolumeGroup{name: "VG", bytesFree: 4096},
				},
			},
			args: args{
				newCmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					StdOutput: []byte("something went wrong"),
					Err:       errors.New("SOME ERROR"),
				}),
				name:               "data",
				virtualSizeInBytes: 1024,
			},
			err: errors.New("something went wrong"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tp := ThinPool{
				LogicalVolume: tt.fields.LogicalVolume,
				dataPercent:   tt.fields.dataPercent,
			}
			got, err := tp.CreateThinVolume(tt.args.newCmd, tt.args.name, tt.args.virtualSizeInBytes)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
