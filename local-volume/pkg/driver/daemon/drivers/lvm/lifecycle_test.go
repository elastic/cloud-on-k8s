// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package lvm

import (
	"errors"
	"testing"

	"github.com/elastic/cloud-on-k8s/local-volume/pkg/driver/daemon/cmdutil"
	"github.com/stretchr/testify/assert"
)

func TestDriver_ListVolumes(t *testing.T) {
	var vgLookup = `{"report":[{"vg":[{"vg_name":"vg","vg_uuid":"1231512521512","vg_size":"1234","vg_free":"1234","vg_extent_size":"1234","vg_extent_count":"1234","vg_free_count,string":"","vg_tags":"tag"}]}]}`
	var lvTp = `{"report":[{"lv":[{"lv_name":"tp","vg_name":"vg","lv_path":"cc","lv_size":"1234","lv_tags":"tt","lv_layout":"thin,pool","data_percent":"23"}]}]}`
	var (
		vgLookupFailure = &cmdutil.FakeExecutables{Stubs: []*cmdutil.FakeExecutable{
			{Err: errors.New("an error"), StdOutput: []byte("something bad")},
		}}
		vgListFailure = &cmdutil.FakeExecutables{Stubs: []*cmdutil.FakeExecutable{
			{StdOutput: []byte(vgLookup)},
			{Err: errors.New("an error"), StdOutput: []byte("something bad")},
		}}
		success = &cmdutil.FakeExecutables{Stubs: []*cmdutil.FakeExecutable{
			{StdOutput: []byte(vgLookup)},
			{StdOutput: []byte(lvTp)},
		}}
	)
	type fields struct {
		options  Options
		fakeExec *cmdutil.FakeExecutables
	}
	tests := []struct {
		name         string
		fields       fields
		want         []string
		wantCommands [][]string
		err          error
	}{
		{
			name: "failure VG Lookup",
			fields: fields{
				fakeExec: vgLookupFailure,
				options: Options{
					ExecutableFactory: vgLookupFailure.ExecutableFactory(),
				},
			},
			wantCommands: [][]string{
				[]string{"vgs", "--options=vg_name,vg_free", "--reportformat=json", "--units=b", "--nosuffix", ""},
			},
			err: errors.New("something bad"),
		},
		{
			name: "failure LV List",
			fields: fields{
				fakeExec: vgListFailure,
				options: Options{
					ExecutableFactory: vgListFailure.ExecutableFactory(),
				},
			},
			wantCommands: [][]string{
				[]string{"vgs", "--options=vg_name,vg_free", "--reportformat=json", "--units=b", "--nosuffix", ""},
				[]string{"lvs", "--options=lv_name,lv_size,vg_name,lv_layout,data_percent", "--reportformat=json", "--units=b", "--nosuffix", "vg"},
			},
			err: errors.New("something bad"),
		},
		{
			name: "Success",
			fields: fields{
				fakeExec: success,
				options: Options{
					ExecutableFactory: success.ExecutableFactory(),
				},
			},
			wantCommands: [][]string{
				[]string{"vgs", "--options=vg_name,vg_free", "--reportformat=json", "--units=b", "--nosuffix", ""},
				[]string{"lvs", "--options=lv_name,lv_size,vg_name,lv_layout,data_percent", "--reportformat=json", "--units=b", "--nosuffix", "vg"},
			},
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
		if tt.fields.fakeExec != nil {
			assert.Equal(
				t,
				tt.wantCommands,
				tt.fields.fakeExec.RecordedExecution(),
			)
		}
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
