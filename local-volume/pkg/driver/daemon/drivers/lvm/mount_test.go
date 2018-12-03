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
					FactoryFunc: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
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
					FactoryFunc: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
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
					FactoryFunc: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
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
					FactoryFunc: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
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
					FactoryFunc: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
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
					FactoryFunc: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
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
					FactoryFunc: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
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
	type fields struct {
		options Options
	}
	type args struct {
		params protocol.MountRequest
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   flex.Response
	}{
		{
			name: "fails looking up VG",
			fields: fields{options: Options{
				FactoryFunc: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
					{Err: errors.New("something bad happened")},
				}),
				VolumeGroupName: "vg",
				UseThinVolumes:  true,
				ThinPoolName:    "tp",
			}},
			args: args{
				params: protocol.MountRequest{
					TargetDir: path.Join("some", "path"),
					Options:   protocol.MountOptions{},
				},
			},
			want: flex.Failure("volume group vg does not seem to exist"),
		},
		{
			name: "fails creating LV",
			fields: fields{options: Options{
				FactoryFunc: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
					{StdOutput: []byte(vgLookup)},
					{Err: errors.New("an error"), StdOutput: []byte("some out")},
				}),
				VolumeGroupName: "vg",
				UseThinVolumes:  true,
				ThinPoolName:    "tp",
			}},
			args: args{
				params: protocol.MountRequest{
					TargetDir: path.Join("some", "path"),
					Options:   protocol.MountOptions{},
				},
			},
			want: flex.Failure("cannot get or create thin pool tp: some out"),
		},
		{
			name: "fails obtaining the LV path",
			fields: fields{options: Options{
				FactoryFunc: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
					{StdOutput: []byte(vgLookup)},
					{StdOutput: []byte(lvTp)},
					{},
					{Err: errors.New("an error"), StdOutput: []byte("some out")},
				}),
				VolumeGroupName: "vg",
				UseThinVolumes:  true,
				ThinPoolName:    "tp",
			}},
			args: args{
				params: protocol.MountRequest{
					TargetDir: path.Join("some", "path"),
					Options:   protocol.MountOptions{},
				},
			},
			want: flex.Failure("cannot retrieve logical volume device path: some out"),
		},
		{
			name: "fails obtaining the LV path",
			fields: fields{options: Options{
				FactoryFunc: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
					{StdOutput: []byte(vgLookup)},
					{StdOutput: []byte(lvTp)},
					{},
					{Err: errors.New("an error"), StdOutput: []byte("some out")},
				}),
				VolumeGroupName: "vg",
				UseThinVolumes:  true,
				ThinPoolName:    "tp",
			}},
			args: args{
				params: protocol.MountRequest{
					TargetDir: path.Join("some", "path"),
					Options:   protocol.MountOptions{},
				},
			},
			want: flex.Failure("cannot retrieve logical volume device path: some out"),
		},
		{
			name: "fails formatting the LV path device",
			fields: fields{options: Options{
				FactoryFunc: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
					{StdOutput: []byte(vgLookup)},
					{StdOutput: []byte(lvTp)},
					{},
					{StdOutput: []byte(lvTp)},
					{Err: errors.New("an error"), StdOutput: []byte("some out")},
				}),
				VolumeGroupName: "vg",
				UseThinVolumes:  true,
				ThinPoolName:    "tp",
			}},
			args: args{
				params: protocol.MountRequest{
					TargetDir: path.Join("some", "path"),
					Options:   protocol.MountOptions{},
				},
			},
			want: flex.Failure("cannot format logical volume path as ext4: an error. Output: "),
		},
		{
			name: "fails due to mount device erroring",
			fields: fields{options: Options{
				FactoryFunc: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
					{StdOutput: []byte(vgLookup)},
					{StdOutput: []byte(lvTp)},
					{},
					{StdOutput: []byte(lvTp)},
					{},
					{Err: errors.New("an error"), StdOutput: []byte("some out")},
				}),
				VolumeGroupName: "vg",
				UseThinVolumes:  true,
				ThinPoolName:    "tp",
			}},
			args: args{
				params: protocol.MountRequest{
					TargetDir: path.Join("some", "path"),
					Options:   protocol.MountOptions{},
				},
			},
			want: flex.Failure("cannot mount device cc to some/path: an error. Output: "),
		},
		{
			name: "succeeds",
			fields: fields{options: Options{
				FactoryFunc: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
					{StdOutput: []byte(vgLookup)},
					{StdOutput: []byte(lvTp)},
					{},
					{StdOutput: []byte(lvTp)},
					{},
					{},
				}),
				VolumeGroupName: "vg",
				UseThinVolumes:  true,
				ThinPoolName:    "tp",
			}},
			args: args{
				params: protocol.MountRequest{
					TargetDir: path.Join("some", "path"),
					Options:   protocol.MountOptions{},
				},
			},
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
		})
	}
}
