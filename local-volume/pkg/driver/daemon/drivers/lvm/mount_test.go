// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package lvm

import (
	"errors"
	"path"
	"testing"

	"github.com/elastic/k8s-operators/local-volume/pkg/driver/daemon/cmdutil"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/flex"
	"github.com/elastic/k8s-operators/local-volume/pkg/driver/protocol"
	"github.com/stretchr/testify/assert"
)

func TestDriver_createThinLV(t *testing.T) {
	var lvTp = `{"report":[{"lv":[{"lv_name":"tp","vg_name":"vg","lv_path":"cc","lv_size":"1234","lv_tags":"tt","lv_layout":"thin,pool","data_percent":"23"}]}]}`
	type fields struct {
		options Options
	}
	type args struct {
		vg          VolumeGroup
		name        string
		virtualSize uint64
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
				options: Options{
					ExecutableFactory: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
						{StdOutput: []byte(lvTp)},
						{},
					}),
					VolumeGroupName: "vg",
					UseThinVolumes:  true,
					ThinPoolName:    "tp",
				},
			},
			args: args{
				vg: VolumeGroup{
					name:      "vg",
					bytesFree: 4096,
				},
				name:        "lv",
				virtualSize: 2048,
			},
			want: LogicalVolume{
				name: "lv", sizeInBytes: 2560,
				vg: VolumeGroup{name: "vg", bytesFree: 4096},
			},
		},
		{
			name: "failure due thin pool CMD execution",
			fields: fields{
				options: Options{
					ExecutableFactory: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
						{
							StdOutput: []byte("some output"),
							Err:       errors.New("some error"),
						},
					}),
					VolumeGroupName: "vg",
					UseThinVolumes:  true,
					ThinPoolName:    "tp",
				},
			},
			args: args{
				vg: VolumeGroup{
					name:      "vg",
					bytesFree: 4096,
				},
				name:        "lv",
				virtualSize: 2048,
			},
			err: errors.New("cannot get or create thin pool tp: some output"),
		},
		{
			name: "failure due CreateThinVolume CMD execution",
			fields: fields{
				options: Options{
					ExecutableFactory: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
						{StdOutput: []byte(lvTp)},
						{
							StdOutput: []byte("some output"),
							Err:       errors.New("some error"),
						},
					}),
					VolumeGroupName: "vg",
					UseThinVolumes:  true,
					ThinPoolName:    "tp",
				},
			},
			args: args{
				vg: VolumeGroup{
					name:      "vg",
					bytesFree: 4096,
				},
				name:        "lv",
				virtualSize: 2048,
			},
			err: errors.New("cannot create thin volume: some output"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Driver{
				options: tt.fields.options,
			}
			got, err := d.createThinLV(tt.args.vg, tt.args.name, tt.args.virtualSize)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDriver_createStandardLV(t *testing.T) {
	type fields struct {
		options Options
	}
	type args struct {
		vg   VolumeGroup
		name string
		size uint64
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
				options: Options{
					ExecutableFactory: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
						{},
					}),
					VolumeGroupName: "vg",
					UseThinVolumes:  true,
					ThinPoolName:    "tp",
				},
			},
			args: args{
				vg: VolumeGroup{
					name:      "vg",
					bytesFree: 4096,
				},
				name: "lv",
				size: 2048,
			},
			want: LogicalVolume{
				name: "lv", sizeInBytes: 2048,
				vg: VolumeGroup{name: "vg", bytesFree: 4096},
			},
		},
		{
			name: "fails due to bigger volume than volume group free size",
			fields: fields{
				options: Options{
					VolumeGroupName: "vg",
					UseThinVolumes:  true,
					ThinPoolName:    "tp",
				},
			},
			args: args{
				vg: VolumeGroup{
					name:      "vg",
					bytesFree: 4096,
				},
				name: "lv",
				size: 5120,
			},
			err: errors.New("not enough space left on volume group: available 4096 bytes, requested: 5120 bytes"),
		},
		{
			name: "fails due to cmd execution failure",
			fields: fields{
				options: Options{
					ExecutableFactory: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
						{
							StdOutput: []byte("some output"),
							Err:       errors.New("some error"),
						},
					}),
					VolumeGroupName: "vg",
					UseThinVolumes:  true,
					ThinPoolName:    "tp",
				},
			},
			args: args{
				vg: VolumeGroup{
					name:      "vg",
					bytesFree: 4096,
				},
				name: "lv",
				size: 512,
			},
			err: errors.New("cannot create logical volume: some output"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Driver{
				options: tt.fields.options,
			}
			got, err := d.createStandardLV(tt.args.vg, tt.args.name, tt.args.size)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDriver_CreateLV(t *testing.T) {
	var lvTp = `{"report":[{"lv":[{"lv_name":"tp","vg_name":"vg","lv_path":"cc","lv_size":"1234","lv_tags":"tt","lv_layout":"thin,pool","data_percent":"23"}]}]}`
	var lvNTp = `{"report":[{"lv":[{"lv_name":"tp","vg_name":"vg","lv_path":"cc","lv_size":"1234","lv_tags":"tt","lv_layout":"","data_percent":"23"}]}]}`
	type fields struct {
		options Options
	}
	type args struct {
		vg   VolumeGroup
		name string
		size uint64
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   LogicalVolume
		err    error
	}{
		{
			name: "success on Thin Volume",
			fields: fields{
				options: Options{
					ExecutableFactory: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
						{StdOutput: []byte(lvTp)},
						{},
					}),
					VolumeGroupName: "vg",
					UseThinVolumes:  true,
					ThinPoolName:    "tp",
				},
			},
			args: args{
				vg:   VolumeGroup{name: "vg", bytesFree: 4096},
				name: "lv",
				size: 2048,
			},
			want: LogicalVolume{
				name: "lv", sizeInBytes: 2560,
				vg: VolumeGroup{name: "vg", bytesFree: 4096},
			},
		},
		{
			name: "success on standard Volume",
			fields: fields{
				options: Options{
					ExecutableFactory: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
						{StdOutput: []byte(lvNTp)},
						{},
					}),
					VolumeGroupName: "vg",
					UseThinVolumes:  false,
					ThinPoolName:    "tp",
				},
			},
			args: args{
				vg:   VolumeGroup{name: "vg", bytesFree: 4096},
				name: "lv",
				size: 2048,
			},
			want: LogicalVolume{
				name: "lv", sizeInBytes: 2048,
				vg: VolumeGroup{name: "vg", bytesFree: 4096},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Driver{options: tt.fields.options}
			got, err := d.CreateLV(tt.args.vg, tt.args.name, tt.args.size)
			assert.Equal(t, tt.err, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDriver_Mount(t *testing.T) {
	var vgLookup = `{"report":[{"vg":[{"vg_name":"vg","vg_uuid":"1231512521512","vg_size":"1234","vg_free":"1234","vg_extent_size":"1234","vg_extent_count":"1234","vg_free_count,string":"","vg_tags":"tag"}]}]}`
	var lvTp = `{"report":[{"lv":[{"lv_name":"tp","vg_name":"vg","lv_path":"cc","lv_size":"1234","lv_tags":"tt","lv_layout":"thin,pool","data_percent":"23"}]}]}`
	var (
		vgLookupFailure = &cmdutil.FakeExecutables{Stubs: []*cmdutil.FakeExecutable{
			{Err: errors.New("something bad happened")},
		}}
		lvCreateFailure = &cmdutil.FakeExecutables{Stubs: []*cmdutil.FakeExecutable{
			{StdOutput: []byte(vgLookup)},
			{Err: errors.New("an error"), StdOutput: []byte("some out")},
		}}
		lvGetPathFailure = &cmdutil.FakeExecutables{Stubs: []*cmdutil.FakeExecutable{
			{StdOutput: []byte(vgLookup)},
			{StdOutput: []byte(lvTp)},
			{},
			{Err: errors.New("an error"), StdOutput: []byte("some out")},
		}}
		lvFailure = &cmdutil.FakeExecutables{Stubs: []*cmdutil.FakeExecutable{
			{StdOutput: []byte(vgLookup)},
			{StdOutput: []byte(lvTp)},
			{},
			{StdOutput: []byte(lvTp)},
			{Err: errors.New("an error"), StdOutput: []byte("some out")},
		}}
		mountFailure = &cmdutil.FakeExecutables{Stubs: []*cmdutil.FakeExecutable{
			{StdOutput: []byte(vgLookup)},
			{StdOutput: []byte(lvTp)},
			{},
			{StdOutput: []byte(lvTp)},
			{},
			{Err: errors.New("an error"), StdOutput: []byte("some out")},
		}}
		success = &cmdutil.FakeExecutables{Stubs: []*cmdutil.FakeExecutable{
			{StdOutput: []byte(vgLookup)},
			{StdOutput: []byte(lvTp)},
			{},
			{StdOutput: []byte(lvTp)},
			{},
			{},
		}}
	)
	type fields struct {
		options  Options
		fakeExec *cmdutil.FakeExecutables
	}
	type args struct {
		params protocol.MountRequest
	}
	tests := []struct {
		name         string
		fields       fields
		args         args
		want         flex.Response
		wantCommands [][]string
	}{
		{
			name: "fails looking up VG",
			fields: fields{
				fakeExec: vgLookupFailure,
				options: Options{
					ExecutableFactory: vgLookupFailure.ExecutableFactory(),
					VolumeGroupName:   "vg",
					UseThinVolumes:    true,
					ThinPoolName:      "tp",
				},
			},
			args: args{
				params: protocol.MountRequest{
					TargetDir: path.Join("some", "path"),
					Options:   protocol.MountOptions{},
				},
			},
			wantCommands: [][]string{
				[]string{"vgs", "--options=vg_name,vg_free", "--reportformat=json", "--units=b", "--nosuffix", "vg"},
			},
			want: flex.Failure("volume group vg does not seem to exist"),
		},
		{
			name: "fails creating LV",
			fields: fields{
				fakeExec: lvCreateFailure,
				options: Options{
					ExecutableFactory: lvCreateFailure.ExecutableFactory(),
					VolumeGroupName:   "vg",
					UseThinVolumes:    true,
					ThinPoolName:      "tp",
				},
			},
			args: args{
				params: protocol.MountRequest{
					TargetDir: path.Join("some", "path"),
					Options:   protocol.MountOptions{},
				},
			},
			wantCommands: [][]string{
				[]string{"vgs", "--options=vg_name,vg_free", "--reportformat=json", "--units=b", "--nosuffix", "vg"},
				[]string{"lvs", "--options=lv_name,lv_size,vg_name,lv_layout,data_percent", "--reportformat=json", "--units=b", "--nosuffix", "vg"},
			},
			want: flex.Failure("cannot get or create thin pool tp: some out"),
		},
		{
			name: "fails obtaining the LV path",
			fields: fields{
				fakeExec: lvGetPathFailure,
				options: Options{
					ExecutableFactory: lvGetPathFailure.ExecutableFactory(),
					VolumeGroupName:   "vg",
					UseThinVolumes:    true,
					ThinPoolName:      "tp",
				},
			},
			args: args{
				params: protocol.MountRequest{
					TargetDir: path.Join("some", "path"),
					Options:   protocol.MountOptions{},
				},
			},
			wantCommands: [][]string{
				[]string{"vgs", "--options=vg_name,vg_free", "--reportformat=json", "--units=b", "--nosuffix", "vg"},
				[]string{"lvs", "--options=lv_name,lv_size,vg_name,lv_layout,data_percent", "--reportformat=json", "--units=b", "--nosuffix", "vg"},
				[]string{"lvcreate", "--virtualsize", "1000000512b", "--name", "path", "--thin", "--thinpool", "tp", "vg"},
				[]string{"lvs", "--options=lv_path", "--reportformat=json", "--units=b", "--nosuffix", "vg/path"},
			},
			want: flex.Failure("cannot retrieve logical volume device path: some out"),
		},
		{
			name: "fails formatting the LV path device",
			fields: fields{
				fakeExec: lvFailure,
				options: Options{
					ExecutableFactory: lvFailure.ExecutableFactory(),
					VolumeGroupName:   "vg",
					UseThinVolumes:    true,
					ThinPoolName:      "tp",
				},
			},
			args: args{
				params: protocol.MountRequest{
					TargetDir: path.Join("some", "path"),
					Options:   protocol.MountOptions{},
				},
			},
			wantCommands: [][]string{
				[]string{"vgs", "--options=vg_name,vg_free", "--reportformat=json", "--units=b", "--nosuffix", "vg"},
				[]string{"lvs", "--options=lv_name,lv_size,vg_name,lv_layout,data_percent", "--reportformat=json", "--units=b", "--nosuffix", "vg"},
				[]string{"lvcreate", "--virtualsize", "1000000512b", "--name", "path", "--thin", "--thinpool", "tp", "vg"},
				[]string{"lvs", "--options=lv_path", "--reportformat=json", "--units=b", "--nosuffix", "vg/path"},
				[]string{"mkfs", "-t", "ext4", "cc"},
			},
			want: flex.Failure("cannot format logical volume path as ext4: an error. Output: "),
		},
		{
			name: "fails due to mount device erroring",
			fields: fields{
				fakeExec: mountFailure,
				options: Options{
					ExecutableFactory: mountFailure.ExecutableFactory(),
					VolumeGroupName:   "vg",
					UseThinVolumes:    true,
					ThinPoolName:      "tp",
				},
			},
			args: args{
				params: protocol.MountRequest{
					TargetDir: path.Join("some", "path"),
					Options:   protocol.MountOptions{},
				},
			},
			wantCommands: [][]string{
				[]string{"vgs", "--options=vg_name,vg_free", "--reportformat=json", "--units=b", "--nosuffix", "vg"},
				[]string{"lvs", "--options=lv_name,lv_size,vg_name,lv_layout,data_percent", "--reportformat=json", "--units=b", "--nosuffix", "vg"},
				[]string{"lvcreate", "--virtualsize", "1000000512b", "--name", "path", "--thin", "--thinpool", "tp", "vg"},
				[]string{"lvs", "--options=lv_path", "--reportformat=json", "--units=b", "--nosuffix", "vg/path"},
				[]string{"mkfs", "-t", "ext4", "cc"},
				[]string{"mount", "cc", "some/path"},
			},
			want: flex.Failure("cannot mount device cc to some/path: an error. Output: "),
		},
		{
			name: "succeeds",
			fields: fields{
				fakeExec: success,
				options: Options{
					ExecutableFactory: success.ExecutableFactory(),
					VolumeGroupName:   "vg",
					UseThinVolumes:    true,
					ThinPoolName:      "tp",
				}},
			args: args{
				params: protocol.MountRequest{
					TargetDir: path.Join("some", "path"),
					Options:   protocol.MountOptions{},
				},
			},
			wantCommands: [][]string{
				[]string{"vgs", "--options=vg_name,vg_free", "--reportformat=json", "--units=b", "--nosuffix", "vg"},
				[]string{"lvs", "--options=lv_name,lv_size,vg_name,lv_layout,data_percent", "--reportformat=json", "--units=b", "--nosuffix", "vg"},
				[]string{"lvcreate", "--virtualsize", "1000000512b", "--name", "path", "--thin", "--thinpool", "tp", "vg"},
				[]string{"lvs", "--options=lv_path", "--reportformat=json", "--units=b", "--nosuffix", "vg/path"},
				[]string{"mkfs", "-t", "ext4", "cc"},
				[]string{"mount", "cc", "some/path"}},
			want: flex.Success("successfully created the volume"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Driver{
				options: tt.fields.options,
			}
			got := d.Mount(tt.args.params)
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
