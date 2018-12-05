package lvm

import (
	"errors"
	"testing"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/cmdutil"
	"github.com/stretchr/testify/assert"
)

func TestDriver_ListVolumes(t *testing.T) {
	var vgLookup = `{"report":[{"vg":[{"vg_name":"vg","vg_uuid":"1231512521512","vg_size":"1234","vg_free":"1234","vg_extent_size":"1234","vg_extent_count":"1234","vg_free_count,string":"","vg_tags":"tag"}]}]}`
	var lvTp = `{"report":[{"lv":[{"lv_name":"tp","vg_name":"vg","lv_path":"cc","lv_size":"1234","lv_tags":"tt","lv_layout":"thin,pool","data_percent":"23"}]}]}`
	type fields struct {
		options Options
	}
	tests := []struct {
		name   string
		fields fields
		want   []string
		err    error
	}{
		{
			name: "failure VG Lookup",
			fields: fields{options: Options{
				ExecutableFactory: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
					{Err: errors.New("an error"), StdOutput: []byte("something bad")},
				}),
			}},
			err: errors.New("something bad"),
		},
		{
			name: "failure LV List",
			fields: fields{options: Options{
				ExecutableFactory: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
					{StdOutput: []byte(vgLookup)},
					{Err: errors.New("an error"), StdOutput: []byte("something bad")},
				}),
			}},
			err: errors.New("something bad"),
		},
		{
			name: "Success",
			fields: fields{options: Options{
				ExecutableFactory: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
					{StdOutput: []byte(vgLookup)},
					{StdOutput: []byte(lvTp)},
				}),
			}},
			want: []string{"tp"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Driver{
				options: tt.fields.options,
			}
			got, err := d.ListVolumes()
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.err, err)
		})
	}
}

func TestDriver_PurgeVolume(t *testing.T) {
	var vgLookup = `{"report":[{"vg":[{"vg_name":"vg","vg_uuid":"1231512521512","vg_size":"1234","vg_free":"1234","vg_extent_size":"1234","vg_extent_count":"1234","vg_free_count,string":"","vg_tags":"tag"}]}]}`
	var lvLookup = `{"report":[{"lv":[{"lv_name":"lv","vg_name":"vg","lv_path":"cc","lv_size":"1234","lv_tags":"tt","lv_layout":"thin,pool","data_percent":"23"}]}]}`
	type fields struct {
		options Options
	}
	type args struct {
		volumeName string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		err    error
	}{
		{
			name: "failure VG Lookup",
			fields: fields{options: Options{
				ExecutableFactory: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
					{Err: errors.New("an error"), StdOutput: []byte("something bad")},
				}),
			}},
			args: args{volumeName: "lv"},
			err:  errors.New("something bad"),
		},
		{
			name: "failure VG Lookup VG not found returns nil",
			fields: fields{options: Options{
				ExecutableFactory: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
					{StdOutput: []byte(`{"report":null}`)},
				}),
			}},
			args: args{volumeName: "lv"},
			err:  nil,
		},
		{
			name: "failure LV Lookup",
			fields: fields{options: Options{
				ExecutableFactory: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
					{StdOutput: []byte(vgLookup)},
					{Err: errors.New("an error"), StdOutput: []byte("something bad")},
				}),
			}},
			args: args{volumeName: "lv"},
			err:  errors.New("something bad"),
		},
		{
			name: "failure LV Lookup LV not found returns nil",
			fields: fields{options: Options{
				ExecutableFactory: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
					{StdOutput: []byte(vgLookup)},
					{StdOutput: []byte(`{"report":null}`)},
				}),
			}},
			args: args{volumeName: "lv"},
			err:  nil,
		},
		{
			name: "failure LV remove",
			fields: fields{options: Options{
				ExecutableFactory: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
					{StdOutput: []byte(vgLookup)},
					{StdOutput: []byte(lvLookup)},
					{Err: errors.New("an error"), StdOutput: []byte("something bad")},
				}),
			}},
			args: args{volumeName: "lv"},
			err:  errors.New("something bad"),
		},
		{
			name: "Success",
			fields: fields{options: Options{
				ExecutableFactory: cmdutil.NewFakeCmdsBuilder([]*cmdutil.FakeExecutable{
					{StdOutput: []byte(vgLookup)},
					{StdOutput: []byte(lvLookup)},
					{},
				}),
			}},
			args: args{volumeName: "lv"},
			err:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Driver{
				options: tt.fields.options,
			}
			err := d.PurgeVolume(tt.args.volumeName)
			assert.Equal(t, tt.err, err)
		})
	}
}
