package lvm

import (
	"errors"
	"testing"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/cmdutil"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/flex"
	"github.com/elastic/stack-operators/local-volume/pkg/driver/protocol"
	"github.com/stretchr/testify/assert"
)

func TestDriver_Unmount(t *testing.T) {
	type fields struct {
		options Options
	}
	type args struct {
		params protocol.UnmountRequest
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   flex.Response
	}{
		{
			name: "success",
			fields: fields{options: Options{
				FactoryFunc: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{}),
			}},
			want: flex.Success("successfully unmounted the volume"),
		},
		{
			name: "failure due to cmd error",
			fields: fields{options: Options{
				FactoryFunc: cmdutil.NewFakeCmdBuilder(&cmdutil.FakeExecutable{
					Bytes: []byte("an output"),
					Err:   errors.New("error"),
				}),
			}},
			want: flex.Failure("Cannot unmount volume : error. Output: an output"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Driver{
				options: tt.fields.options,
			}
			got := d.Unmount(tt.args.params)
			assert.Equal(t, tt.want, got)
		})
	}
}
