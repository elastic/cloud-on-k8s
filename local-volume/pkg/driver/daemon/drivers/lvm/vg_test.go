// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package lvm

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/elastic/cloud-on-k8s/local-volume/pkg/driver/daemon/cmdutil"
	"github.com/stretchr/testify/assert"
)

func TestLookupVolumeGroup(t *testing.T) {
	var success = `{"report":[{"vg":[{"vg_name":"vg","vg_uuid":"1231512521512","vg_size":"1234","vg_free":"1234","vg_extent_size":"1234","vg_extent_count":"1234","vg_free_count,string":"","vg_tags":"tag"}]}]}`
	type args struct {
		newCmd cmdutil.ExecutableFactory
		name   string
	}
	tests := []struct {
		name string
		args args
		want VolumeGroup
		err  error
	}{
		{
			name: "success",
			args: args{
				newCmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					StdOutput: []byte(success),
				}),
			},
			want: VolumeGroup{
				name:      "vg",
				bytesFree: 1234,
			},
		},
		{
			name: "failure due to cmd err",
			args: args{
				newCmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					StdOutput: []byte("something wrong happened"),
					Err:       errors.New("an error"),
				}),
			},
			want: VolumeGroup{},
			err:  errors.New("something wrong happened"),
		},
		{
			name: "failure due to vg not found",
			args: args{
				newCmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					StdOutput: []byte(`{"report":null}`),
				}),
			},
			want: VolumeGroup{},
			err:  ErrVolumeGroupNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LookupVolumeGroup(tt.args.newCmd, tt.args.name)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestVolumeGroup_CreateLogicalVolume(t *testing.T) {
	type fields struct {
		name      string
		bytesFree uint64
	}
	type args struct {
		newCmd      cmdutil.ExecutableFactory
		name        string
		sizeInBytes uint64
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
				name:      "avg",
				bytesFree: 4096,
			},
			args: args{
				newCmd:      cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{}),
				name:        "anlv",
				sizeInBytes: 2048,
			},
			want: LogicalVolume{
				name:        "anlv",
				sizeInBytes: 2048,
				vg:          VolumeGroup{name: "avg", bytesFree: 4096},
			},
		},
		{
			name: "failure due to lv name",
			fields: fields{
				name:      "avg",
				bytesFree: 4096,
			},
			args: args{
				newCmd:      cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{}),
				name:        "anlv??",
				sizeInBytes: 2048,
			},
			err: ErrInvalidLVName,
		},
		{
			name: "failure due to cmd error",
			fields: fields{
				name:      "avg",
				bytesFree: 4096,
			},
			args: args{
				newCmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					StdOutput: []byte("something happened"),
					Err:       errors.New("an error"),
				}),
				name:        "anlv",
				sizeInBytes: 2048,
			},
			err: errors.New("something happened"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vg := VolumeGroup{
				name:      tt.fields.name,
				bytesFree: tt.fields.bytesFree,
			}
			got, err := vg.CreateLogicalVolume(tt.args.newCmd, tt.args.name, tt.args.sizeInBytes)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestVolumeGroup_CreateThinPool(t *testing.T) {
	var lookupRes = `{"report":[{"lv":[{"lv_name":"lv","vg_name":"vg","lv_path":"cc","lv_size":"1234","lv_tags":"tt","lv_layout":"thin,pool","data_percent":"23.0"}]}]}`
	type fields struct {
		name      string
		bytesFree uint64
	}
	type args struct {
		newCmd cmdutil.ExecutableFactory
		name   string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   ThinPool
		err    error
	}{
		{
			name: "success",
			fields: fields{
				name:      "vg",
				bytesFree: 4096,
			},
			args: args{
				newCmd: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
					{}, {StdOutput: []byte(lookupRes)},
				}),
				name: "lv",
			},
			want: ThinPool{LogicalVolume: LogicalVolume{name: "lv", sizeInBytes: 1234, vg: VolumeGroup{name: "vg", bytesFree: 4096}}, dataPercent: 23},
		},
		{
			name: "failure on invalid LV name",
			err:  ErrInvalidLVName,
		},
		{
			name: "failure on cmd error",
			fields: fields{
				name:      "vg",
				bytesFree: 4096,
			},
			args: args{
				newCmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					StdOutput: []byte("something bad happened"),
					Err:       errors.New("an error"),
				}),
				name: "lv",
			},
			err: errors.New("something bad happened"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vg := VolumeGroup{
				name:      tt.fields.name,
				bytesFree: tt.fields.bytesFree,
			}
			got, err := vg.CreateThinPool(tt.args.newCmd, tt.args.name)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestVolumeGroup_lookupLV(t *testing.T) {
	var randomStdOutput = `{"report":[{"lv":[{"lv_name":"lv","vg_name":"vg","lv_path":"cc","lv_size":"1234","lv_tags":"tt","lv_layout":"ll","data_percent":"23"}]}]}`
	var success lvsOutput
	json.Unmarshal([]byte(randomStdOutput), &success)
	type fields struct {
		name      string
		bytesFree uint64
	}
	type args struct {
		newCmd cmdutil.ExecutableFactory
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   lvsOutput
		err    error
	}{
		{
			name: "success",
			fields: fields{
				name:      "vg",
				bytesFree: 4096,
			},
			args: args{
				newCmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					StdOutput: []byte(randomStdOutput),
				}),
			},
			want: success,
		},
		{
			name: "failure due to cmd error ",
			fields: fields{
				name:      "vg",
				bytesFree: 4096,
			},
			args: args{
				newCmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					StdOutput: []byte("something bad happened here"),
					Err:       errors.New("an error"),
				}),
			},
			want: lvsOutput{},
			err:  errors.New("something bad happened here"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vg := VolumeGroup{
				name:      tt.fields.name,
				bytesFree: tt.fields.bytesFree,
			}
			got, err := vg.lookupLV(tt.args.newCmd)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestVolumeGroup_LookupLogicalVolume(t *testing.T) {
	var randomStdOutput = `{"report":[{"lv":[{"lv_name":"lv","vg_name":"vg","lv_path":"cc","lv_size":"1234","lv_tags":"tt","lv_layout":"ll","data_percent":"23"}]}]}`
	var multipleLV = `{"report":[{"lv":[{"lv_name":"lv2","vg_name":"vg2","lv_path":"cc","lv_size":"1234","lv_tags":"tt","lv_layout":"ll","data_percent":"23"},{"lv_name":"lv","vg_name":"vg","lv_path":"cc","lv_size":"1234","lv_tags":"tt","lv_layout":"ll","data_percent":"23"}]}]}`
	type fields struct {
		name      string
		bytesFree uint64
	}
	type args struct {
		newCmd cmdutil.ExecutableFactory
		name   string
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
				name:      "vg",
				bytesFree: 4096,
			},
			args: args{
				newCmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					StdOutput: []byte(randomStdOutput),
				}),
				name: "lv",
			},
			want: LogicalVolume{
				name:        "lv",
				sizeInBytes: 1234,
				vg:          VolumeGroup{name: "vg", bytesFree: 4096},
			},
		},
		{
			name: "success multiple lv",
			fields: fields{
				name:      "vg",
				bytesFree: 4096,
			},
			args: args{
				newCmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					StdOutput: []byte(multipleLV),
				}),
				name: "lv",
			},
			want: LogicalVolume{
				name:        "lv",
				sizeInBytes: 1234,
				vg:          VolumeGroup{name: "vg", bytesFree: 4096},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vg := VolumeGroup{
				name:      tt.fields.name,
				bytesFree: tt.fields.bytesFree,
			}
			got, err := vg.LookupLogicalVolume(tt.args.newCmd, tt.args.name)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestVolumeGroup_LookupThinPool(t *testing.T) {
	var randomStdOutput = `{"report":[{"lv":[{"lv_name":"lvn","vg_name":"vg","lv_path":"cc","lv_size":"1234","lv_tags":"tt","lv_layout":"thin,pool","data_percent":"23.0"}]}]}`
	type fields struct {
		name      string
		bytesFree uint64
	}
	type args struct {
		newCmd cmdutil.ExecutableFactory
		name   string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   ThinPool
		err    error
	}{
		{
			name: "success",
			fields: fields{
				name:      "vg",
				bytesFree: 4096,
			},
			args: args{
				newCmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					StdOutput: []byte(randomStdOutput),
				}),
				name: "lvn",
			},
			want: ThinPool{LogicalVolume: LogicalVolume{name: "lvn", sizeInBytes: 1234, vg: VolumeGroup{name: "vg", bytesFree: 4096}}, dataPercent: 23},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vg := VolumeGroup{
				name:      tt.fields.name,
				bytesFree: tt.fields.bytesFree,
			}
			got, err := vg.LookupThinPool(tt.args.newCmd, tt.args.name)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestVolumeGroup_ListLogicalVolumes(t *testing.T) {
	var multipleLV = `{"report":[{"lv":[{"lv_name":"lv2","vg_name":"vg2","lv_path":"cc","lv_size":"1234","lv_tags":"tt","lv_layout":"ll","data_percent":"23"},{"lv_name":"lv","vg_name":"vg","lv_path":"cc","lv_size":"1234","lv_tags":"tt","lv_layout":"ll","data_percent":"23"}]}]}`
	type fields struct {
		name      string
		bytesFree uint64
	}
	type args struct {
		newCmd cmdutil.ExecutableFactory
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   []LogicalVolume
		err    error
	}{
		{
			name: "success",
			fields: fields{
				name:      "vg",
				bytesFree: 4096,
			},
			args: args{
				newCmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					StdOutput: []byte(multipleLV),
				}),
			},
			want: []LogicalVolume{
				LogicalVolume{name: "lv2", sizeInBytes: 1234, vg: VolumeGroup{name: "vg", bytesFree: 4096}},
				LogicalVolume{name: "lv", sizeInBytes: 1234, vg: VolumeGroup{name: "vg", bytesFree: 4096}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vg := VolumeGroup{
				name:      tt.fields.name,
				bytesFree: tt.fields.bytesFree,
			}
			got, err := vg.ListLogicalVolumes(tt.args.newCmd)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestVolumeGroup_GetOrCreateThinPool(t *testing.T) {
	var lvTp = `{"report":[{"lv":[{"lv_name":"lv","vg_name":"vg","lv_path":"cc","lv_size":"1234","lv_tags":"tt","lv_layout":"thin,pool","data_percent":"23"}]}]}`
	var noLvFound = `{"report":[{"lv":[{"lv_name":"lvn","vg_name":"vg","lv_path":"cc","lv_size":"1234","lv_tags":"tt","lv_layout":"thin,pool","data_percent":"23"}]}]}`
	type fields struct {
		name      string
		bytesFree uint64
	}
	type args struct {
		newCmd cmdutil.ExecutableFactory
		name   string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   ThinPool
		err    error
	}{
		{
			name: "volume exists",
			fields: fields{
				name:      "vg",
				bytesFree: 4096,
			},
			args: args{
				newCmd: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					StdOutput: []byte(lvTp),
				}),
				name: "lv",
			},
			want: ThinPool{LogicalVolume: LogicalVolume{
				name: "lv", sizeInBytes: 1234, vg: VolumeGroup{name: "vg", bytesFree: 4096},
			}, dataPercent: 23,
			},
		},
		{
			name: "volume doesn't exist and thus creates it",
			fields: fields{
				name:      "vg",
				bytesFree: 4096,
			},
			args: args{
				newCmd: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
					{StdOutput: []byte(noLvFound)}, // lvs
					{},                             // create
					{StdOutput: []byte(lvTp)},      // lvs
				}),
				name: "lv",
			},
			want: ThinPool{LogicalVolume: LogicalVolume{
				name: "lv", sizeInBytes: 1234, vg: VolumeGroup{name: "vg", bytesFree: 4096},
			}, dataPercent: 23,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vg := VolumeGroup{
				name:      tt.fields.name,
				bytesFree: tt.fields.bytesFree,
			}
			got, err := vg.GetOrCreateThinPool(tt.args.newCmd, tt.args.name)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
